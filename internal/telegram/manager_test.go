package telegram

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

type memoryRepo struct {
	mu             sync.Mutex
	accounts       map[uuid.UUID]domain.TelegramAccount
	values         map[string][]byte
	leases, clears int
	writeErr       error
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{accounts: map[uuid.UUID]domain.TelegramAccount{}, values: map[string][]byte{}}
}
func (r *memoryRepo) TelegramAccount(ctx context.Context, id uuid.UUID) (domain.TelegramAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return a, ErrAccountNotFound
	}
	return a, nil
}
func (r *memoryRepo) TelegramEnabledAccounts(context.Context) ([]domain.TelegramAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.TelegramAccount
	for _, a := range r.accounts {
		if a.Enabled {
			out = append(out, a)
		}
	}
	return out, nil
}
func (r *memoryRepo) TelegramSetEnabled(ctx context.Context, id uuid.UUID, v bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	a.Enabled = v
	r.accounts[id] = a
	return nil
}
func (r *memoryRepo) TelegramSetStatus(ctx context.Context, id uuid.UUID, v string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	a.Status = v
	r.accounts[id] = a
	return nil
}
func (r *memoryRepo) TelegramSaveIdentity(ctx context.Context, a domain.TelegramAccount) error {
	return nil
}
func (r *memoryRepo) TelegramAcquire(ctx context.Context, id uuid.UUID) (context.Context, func(), error) {
	r.mu.Lock()
	r.leases++
	r.mu.Unlock()
	return ctx, func() { r.mu.Lock(); r.leases--; r.mu.Unlock() }, nil
}
func (r *memoryRepo) TelegramLoad(ctx context.Context, id uuid.UUID, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.values[id.String()+key]...), nil
}
func (r *memoryRepo) TelegramSave(ctx context.Context, id uuid.UUID, key string, v []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr != nil {
		return r.writeErr
	}
	r.values[id.String()+key] = append([]byte(nil), v...)
	return nil
}
func (r *memoryRepo) TelegramDeleteState(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clears++
	for k := range r.values {
		if len(k) >= 36 && k[:36] == id.String() {
			delete(r.values, k)
		}
	}
	return nil
}

type fakeClient struct {
	Client
	run  func(context.Context, Callbacks) error
	send func(context.Context) (*domain.TelegramMessage, error)
}

func (c *fakeClient) Run(ctx context.Context, cb Callbacks) error {
	if c.run != nil {
		return c.run(ctx, cb)
	}
	cb.Status(domain.TelegramConnected)
	<-ctx.Done()
	return ctx.Err()
}
func (c *fakeClient) SendMessage(ctx context.Context, _ domain.TelegramSendRequest) (*domain.TelegramMessage, error) {
	if c.send != nil {
		return c.send(ctx)
	}
	return &domain.TelegramMessage{ID: 1}, nil
}
func (c *fakeClient) Logout(context.Context) error { return nil }
func managerForTest(t *testing.T, r *memoryRepo, f Factory) (*AccountManager, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	m, err := NewAccountManager(ctx, r, f, log.New(io.Discard, "", 0), 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		ctx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if err := m.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	return m, cancel
}
func waitStatus(t *testing.T, m *AccountManager, id uuid.UUID, status string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a, ok := m.GetAccount(id); ok && a.Status == status {
			return
		}
		time.Sleep(time.Millisecond)
	}
	a, _ := m.GetAccount(id)
	t.Fatalf("status=%s want=%s", a.Status, status)
}
func addAccount(r *memoryRepo) uuid.UUID {
	id := uuid.New()
	r.accounts[id] = domain.TelegramAccount{ID: id, WorkspaceID: uuid.New(), Enabled: true}
	return id
}

func TestManagerConcurrentLifecycle(t *testing.T) {
	r := newMemoryRepo()
	id := addAccount(r)
	var created atomic.Int32
	m, _ := managerForTest(t, r, func(domain.TelegramAccount) (Client, error) { created.Add(1); return &fakeClient{}, nil })
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.StartAccount(context.Background(), id); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	waitStatus(t, m, id, domain.TelegramConnected)
	if created.Load() != 1 {
		t.Fatalf("created %d", created.Load())
	}
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.SendMessage(context.Background(), id, domain.TelegramSendRequest{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.StopAccount(context.Background(), id); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	waitStatus(t, m, id, domain.TelegramStopped)
	if _, err := m.SendMessage(context.Background(), id, domain.TelegramSendRequest{}); !errors.Is(err, ErrAccountNotRunning) {
		t.Fatal(err)
	}
	if err := m.StartAccount(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, domain.TelegramConnected)
	if created.Load() != 2 {
		t.Fatalf("restart count %d", created.Load())
	}
}
func TestManagerUnknownAndFailureIsolation(t *testing.T) {
	r := newMemoryRepo()
	bad, good := addAccount(r), addAccount(r)
	m, _ := managerForTest(t, r, func(a domain.TelegramAccount) (Client, error) {
		if a.ID == bad {
			return &fakeClient{run: func(context.Context, Callbacks) error { return ErrSessionInvalid }}, nil
		}
		return &fakeClient{}, nil
	})
	if err := m.StartAccount(context.Background(), uuid.New()); !errors.Is(err, ErrAccountNotFound) {
		t.Fatal(err)
	}
	if err := m.StopAccount(context.Background(), uuid.New()); !errors.Is(err, ErrAccountNotFound) {
		t.Fatal(err)
	}
	if err := m.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, bad, domain.TelegramAuthRequired)
	waitStatus(t, m, good, domain.TelegramConnected)
	if _, err := m.SendMessage(context.Background(), good, domain.TelegramSendRequest{}); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	clears := r.clears
	r.mu.Unlock()
	if clears != 1 {
		t.Fatalf("invalid session not removed: %d", clears)
	}
}
func TestManagerRootCancelStopsCallsAndReleasesLease(t *testing.T) {
	r := newMemoryRepo()
	id := addAccount(r)
	sending := make(chan struct{})
	result := make(chan error, 1)
	m, cancel := managerForTest(t, r, func(domain.TelegramAccount) (Client, error) {
		return &fakeClient{send: func(ctx context.Context) (*domain.TelegramMessage, error) {
			close(sending)
			<-ctx.Done()
			return nil, ctx.Err()
		}}, nil
	})
	if err := m.StartAccount(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, domain.TelegramConnected)
	go func() { _, err := m.SendMessage(context.Background(), id, domain.TelegramSendRequest{}); result <- err }()
	<-sending
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("send goroutine did not exit")
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	leases := r.leases
	r.mu.Unlock()
	if leases != 0 {
		t.Fatalf("leases=%d", leases)
	}
	if err := m.StartAccount(context.Background(), id); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestManagerInvalidSessionFromOperation(t *testing.T) {
	r := newMemoryRepo()
	id := addAccount(r)
	m, _ := managerForTest(t, r, func(domain.TelegramAccount) (Client, error) {
		return &fakeClient{send: func(context.Context) (*domain.TelegramMessage, error) { return nil, ErrSessionInvalid }}, nil
	})
	if err := m.StartAccount(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, domain.TelegramConnected)
	if _, err := m.SendMessage(context.Background(), id, domain.TelegramSendRequest{}); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal(err)
	}
	waitStatus(t, m, id, domain.TelegramAuthRequired)
}
func TestManagerLogoutClearsState(t *testing.T) {
	r := newMemoryRepo()
	id := addAccount(r)
	m, _ := managerForTest(t, r, func(domain.TelegramAccount) (Client, error) { return &fakeClient{}, nil })
	if err := m.StartAccount(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, domain.TelegramConnected)
	if err := m.LogoutAccount(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	a, _ := r.TelegramAccount(context.Background(), id)
	if a.Enabled || a.Status != domain.TelegramAuthRequired {
		t.Fatalf("%+v", a)
	}
	r.mu.Lock()
	clears := r.clears
	r.mu.Unlock()
	if clears != 1 {
		t.Fatalf("clears=%d", clears)
	}
}

func TestFloodWaitStatusDoesNotCrashOrDisableAccount(t *testing.T) {
	r := newMemoryRepo()
	id := addAccount(r)
	m, _ := managerForTest(t, r, func(domain.TelegramAccount) (Client, error) {
		return &fakeClient{run: func(ctx context.Context, cb Callbacks) error {
			cb.Status(domain.TelegramConnected)
			cb.FloodWait(time.Minute)
			<-ctx.Done()
			return ctx.Err()
		}}, nil
	})
	if err := m.StartAccount(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, id, domain.TelegramFloodWait)
	if _, err := m.SendMessage(context.Background(), id, domain.TelegramSendRequest{}); err != nil {
		t.Fatal(err)
	}
}
