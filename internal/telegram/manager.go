package telegram

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

type accountRuntime struct {
	mu         sync.RWMutex
	account    domain.TelegramAccount
	client     Client
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	ready      bool
	terminal   error
	floodUntil time.Time
}

type AccountManager struct {
	mu          sync.RWMutex
	root        context.Context
	cancel      context.CancelFunc
	repo        Repository
	factory     Factory
	logger      *log.Logger
	accounts    map[uuid.UUID]*accountRuntime
	maxAccounts int
	closing     bool
	wg          sync.WaitGroup
}

func NewAccountManager(ctx context.Context, repo Repository, factory Factory, logger *log.Logger, maxAccounts int) (*AccountManager, error) {
	if ctx == nil || repo == nil || factory == nil || logger == nil || maxAccounts < 1 {
		return nil, ErrInvalidInput
	}
	root, cancel := context.WithCancel(ctx)
	return &AccountManager{root: root, cancel: cancel, repo: repo, factory: factory, logger: logger, accounts: make(map[uuid.UUID]*accountRuntime), maxAccounts: maxAccounts}, nil
}

func (m *AccountManager) Restore(ctx context.Context) error {
	accounts, err := m.repo.TelegramEnabledAccounts(ctx)
	if err != nil {
		return safe(ErrStorage, err)
	}
	for _, a := range accounts {
		if err := m.StartAccount(ctx, a.ID); err != nil {
			m.logger.Printf("component=telegram account_id=%s operation=restore status=failed", a.ID)
		}
	}
	return nil
}

func (m *AccountManager) StartAccount(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || m.root.Err() != nil {
		return ErrUnavailable
	}
	if r := m.accounts[id]; r != nil {
		select {
		case <-r.done:
		default:
			if r.ctx.Err() != nil {
				return ErrConflict
			}
			return nil
		}
	}
	active := 0
	for _, r := range m.accounts {
		select {
		case <-r.done:
		default:
			active++
		}
	}
	if active >= m.maxAccounts {
		return ErrConflict
	}
	a, err := m.repo.TelegramAccount(ctx, id)
	if err != nil {
		return err
	}
	client, err := m.factory(a)
	if err != nil {
		return err
	}
	if err := m.repo.TelegramSetEnabled(ctx, id, true); err != nil {
		return safe(ErrStorage, err)
	}
	life, cancel := context.WithCancel(m.root)
	a.Status = domain.TelegramConnecting
	a.Enabled = true
	r := &accountRuntime{account: a, client: client, ctx: life, cancel: cancel, done: make(chan struct{})}
	m.accounts[id] = r
	m.wg.Add(1)
	go m.run(r)
	return nil
}

func (m *AccountManager) run(r *accountRuntime) {
	defer m.wg.Done()
	defer close(r.done)
	defer r.cancel()
	id := r.account.ID
	ctx, release, err := m.repo.TelegramAcquire(r.ctx, id)
	if err != nil {
		r.mu.Lock()
		r.account.Status = domain.TelegramFailed
		r.mu.Unlock()
		m.logger.Printf("component=telegram account_id=%s operation=lease status=failed", id)
		return
	}
	defer release()
	status := func(s string) {
		r.mu.Lock()
		r.account.Status = s
		r.ready = s == domain.TelegramConnected || s == domain.TelegramAuthRequired
		r.mu.Unlock()
		if err := m.repo.TelegramSetStatus(ctx, id, s); err != nil {
			m.logger.Printf("component=telegram account_id=%s operation=persist_status status=failed", id)
		}
	}
	status(domain.TelegramConnecting)
	err = r.client.Run(ctx, Callbacks{Status: status, FloodWait: func(d time.Duration) { r.mu.Lock(); r.floodUntil = time.Now().Add(d); r.mu.Unlock() }, Identity: func(ctx context.Context, a domain.TelegramAccount) error {
		a.ID = id
		if err := m.repo.TelegramSaveIdentity(ctx, a); err != nil {
			return safe(ErrStorage, err)
		}
		r.mu.Lock()
		r.account.UserID = a.UserID
		r.account.Phone = a.Phone
		r.account.Username = a.Username
		r.account.FirstName = a.FirstName
		r.account.LastName = a.LastName
		r.mu.Unlock()
		return nil
	}})
	r.mu.RLock()
	terminal := r.terminal
	r.mu.RUnlock()
	if terminal != nil {
		err = terminal
	}
	if errors.Is(err, ErrSessionInvalid) {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(r.ctx), 5*time.Second)
		if clearErr := m.repo.TelegramDeleteState(cleanup, id); clearErr != nil {
			err = safe(ErrStorage, clearErr)
		}
		stop()
	}
	final := domain.TelegramStopped
	if errors.Is(err, ErrAuthRequired) || errors.Is(err, ErrSessionInvalid) {
		final = domain.TelegramAuthRequired
	} else if r.ctx.Err() == nil && err != nil {
		final = domain.TelegramFailed
	}
	r.mu.Lock()
	r.ready = false
	r.account.Status = final
	r.mu.Unlock()
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(r.ctx), 5*time.Second)
	defer cancel()
	_ = m.repo.TelegramSetStatus(cleanup, id, final)
	m.logger.Printf("component=telegram account_id=%s operation=runtime status=%s", id, final)
}

func (m *AccountManager) StopAccount(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	if _, err := m.repo.TelegramAccount(ctx, id); err != nil {
		m.mu.Unlock()
		return err
	}
	if err := m.repo.TelegramSetEnabled(ctx, id, false); err != nil {
		m.mu.Unlock()
		return safe(ErrStorage, err)
	}
	r := m.accounts[id]
	if r != nil {
		r.mu.Lock()
		r.account.Enabled = false
		r.mu.Unlock()
		r.cancel()
	}
	m.mu.Unlock()
	if r == nil {
		return m.repo.TelegramSetStatus(ctx, id, domain.TelegramStopped)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		return nil
	}
}

func (m *AccountManager) GetAccount(id uuid.UUID) (domain.TelegramAccount, bool) {
	m.mu.RLock()
	r, ok := m.accounts[id]
	m.mu.RUnlock()
	if !ok {
		return domain.TelegramAccount{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.account
	if a.Status == domain.TelegramConnected && time.Now().Before(r.floodUntil) {
		a.Status = domain.TelegramFloodWait
	}
	return a, true
}

// WithClient scopes every operation to both the request and account lifetimes.
func (m *AccountManager) WithClient(ctx context.Context, id uuid.UUID, authRequired bool, fn func(context.Context, Client) error) error {
	m.mu.RLock()
	r := m.accounts[id]
	m.mu.RUnlock()
	if r == nil {
		return ErrAccountNotRunning
	}
	r.mu.RLock()
	ready, status := r.ready, r.account.Status
	r.mu.RUnlock()
	if !ready || r.ctx.Err() != nil {
		return ErrAccountNotRunning
	}
	if authRequired && status != domain.TelegramConnected {
		return ErrAuthRequired
	}
	op, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	defer cancel()
	defer stop()
	err := fn(op, r.client)
	if errors.Is(err, ErrSessionInvalid) {
		r.mu.Lock()
		r.terminal = err
		r.mu.Unlock()
		r.cancel()
	}
	return err
}
func (m *AccountManager) SendMessage(ctx context.Context, id uuid.UUID, req domain.TelegramSendRequest) (out *domain.TelegramMessage, err error) {
	err = m.WithClient(ctx, id, true, func(ctx context.Context, c Client) error { var e error; out, e = c.SendMessage(ctx, req); return e })
	return
}
func (m *AccountManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	m.closing = true
	m.cancel()
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (m *AccountManager) LogoutAccount(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.accounts[id]
	if r == nil {
		return ErrAccountNotRunning
	}
	r.mu.RLock()
	ready := r.ready
	r.mu.RUnlock()
	if !ready {
		return ErrAccountNotRunning
	}
	op, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	defer cancel()
	defer stop()
	if err := r.client.Logout(op); err != nil {
		return err
	}
	r.cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
	}
	if err := m.repo.TelegramDeleteState(ctx, id); err != nil {
		return safe(ErrStorage, err)
	}
	if err := m.repo.TelegramSetEnabled(ctx, id, false); err != nil {
		return safe(ErrStorage, err)
	}
	r.mu.Lock()
	r.account.Enabled = false
	r.account.Status = domain.TelegramAuthRequired
	r.mu.Unlock()
	return m.repo.TelegramSetStatus(ctx, id, domain.TelegramAuthRequired)
}
