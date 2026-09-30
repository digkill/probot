package telegram

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/session"
	gotd "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/telegram/message"
	msgpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/updates"
	updhook "github.com/gotd/td/telegram/updates/hook"
	"github.com/gotd/td/tg"
	"golang.org/x/time/rate"
)

type gotdClient struct {
	account         domain.TelegramAccount
	cfg             Config
	storage         *encryptedStorage
	events          EventSink
	logger          *log.Logger
	client          *gotd.Client
	auth            authRPC
	qrAuth          qrAuthFunc
	peers           *peers.Manager
	resolver        *msgpeer.LRUResolver
	sender          *message.Sender
	gaps            *updates.Manager
	waiter          *floodwait.Waiter
	loginToken      qrlogin.LoggedIn
	selfID          atomic.Int64
	authMu          sync.Mutex
	login           domain.TelegramLogin
	phone, codeHash string
	authDone        chan struct{}
	authorized      bool
	life            context.Context
	cancel          context.CancelFunc
	qrCancel        context.CancelFunc
	authWG          sync.WaitGroup
	fatal           chan error
	onFloodWait     func(time.Duration)
}

func NewFactory(cfg Config, kv KV, events EventSink, logger *log.Logger) (Factory, error) {
	if cfg.AppID <= 0 || cfg.AppHash == "" || len(cfg.EncryptionKey) != 32 || !(cfg.RPCRate > 0 && cfg.RPCRate <= 10) || cfg.AuthTTL < time.Minute || kv == nil || events == nil || logger == nil {
		return nil, ErrInvalidInput
	}
	return func(a domain.TelegramAccount) (Client, error) {
		c := &gotdClient{account: a, cfg: cfg, storage: &encryptedStorage{kv: kv, id: a.ID, key: []byte(cfg.EncryptionKey)}, events: events, logger: logger, authDone: make(chan struct{}), fatal: make(chan error, 1), login: domain.TelegramLogin{State: "new"}}
		c.storage.onFailure = func(err error) {
			select {
			case c.fatal <- err:
			default:
			}
			if c.cancel != nil {
				c.cancel()
			}
		}
		d := tg.NewUpdateDispatcher()
		c.loginToken = qrlogin.OnLoginToken(d)
		d.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
			return c.receive(ctx, u.Message)
		})
		d.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
			return c.receive(ctx, u.Message)
		})
		// Bind peers once the API exists. The closure is called only inside Run.
		handler := gotd.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
			err := c.peers.UpdateHook(d).Handle(ctx, u)
			if err != nil {
				c.storage.failed.Store(true)
				c.storage.onFailure(safe(ErrStorage, err))
			}
			return err
		})
		c.gaps = updates.New(updates.Config{Handler: handler, Storage: c.storage, AccessHasher: c.storage, UserAccessHasher: c.storage, MaxChannelDifferenceConcurrency: 2,
			OnTooLong: func() {
				logger.Printf("component=telegram account_id=%s operation=recovery status=history_resync_required", a.ID)
			},
			OnChannelTooLong: func(id int64) {
				logger.Printf("component=telegram account_id=%s peer_id=%d operation=recovery status=history_resync_required", a.ID, id)
			},
		})
		c.waiter = floodwait.NewWaiter().WithMaxWait(time.Minute).WithMaxRetries(3).WithCallback(func(ctx context.Context, w floodwait.FloodWait) {
			logger.Printf("component=telegram account_id=%s operation=rpc wait_duration=%s", a.ID, w.Duration)
			if c.onFloodWait != nil {
				c.onFloodWait(w.Duration)
			}
		})
		c.client = gotd.NewClient(cfg.AppID, cfg.AppHash, gotd.Options{OnSelfError: func(ctx context.Context, err error) error {
			if c.selfID.Load() != 0 && errors.Is(rpcError(err), ErrSessionInvalid) {
				return rpcError(err)
			}
			return nil
		}, OnSelfSuccess: func(self *tg.User) { c.selfID.Store(self.ID) }, SessionStorage: c.storage, UpdateHandler: c.gaps, Middlewares: []gotd.Middleware{
			updhook.UpdateHook(c.gaps.Handle), updhook.AffectedHook(c.gaps), c.waiter, ratelimit.New(rate.Limit(cfg.RPCRate), 1),
		}})
		c.auth = c.client.Auth()
		c.qrAuth = func(ctx context.Context, signal qrlogin.LoggedIn, show func(context.Context, qrlogin.Token) error) (*tg.AuthAuthorization, error) {
			return c.client.QR().Auth(ctx, signal, show)
		}
		c.peers = peers.Options{Storage: c.storage, Cache: peerCache{c.storage}}.Build(c.client.API())
		c.resolver = msgpeer.NewLRUResolver(peerResolver{c}, 256).WithExpiration(5 * time.Minute)
		c.sender = message.NewSender(c.client.API()).WithResolver(c.resolver)
		return c, nil
	}, nil
}

func (c *gotdClient) Run(parent context.Context, cb Callbacks) error {
	ctx, cancel := context.WithCancel(parent)
	c.authMu.Lock()
	c.life = ctx
	c.onFloodWait = cb.FloodWait
	c.cancel = cancel
	c.authMu.Unlock()
	defer func() {
		c.authMu.Lock()
		cancel()
		c.authMu.Unlock()
		c.authWG.Wait()
		c.authMu.Lock()
		c.phone = ""
		c.codeHash = ""
		c.login.URL = ""
		c.authMu.Unlock()
	}()
	c.authWG.Add(1)
	go func() {
		defer c.authWG.Done()
		timer := time.NewTicker(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.authMu.Lock()
				c.expireLogin()
				c.authMu.Unlock()
			}
		}
	}()
	sessionData, sessionErr := c.storage.LoadSession(ctx)
	if sessionErr != nil && !errors.Is(sessionErr, session.ErrNotFound) {
		return sessionErr
	}
	hadSession := len(sessionData) > 0
	clear(sessionData)
	err := c.waiter.Run(ctx, func(ctx context.Context) error {
		return c.client.Run(ctx, func(ctx context.Context) error {
			status, err := c.client.Auth().Status(ctx)
			if err != nil {
				return err
			}
			if status.Authorized {
				c.authMu.Lock()
				c.markAuthorized()
				c.authMu.Unlock()
			} else {
				if hadSession && c.account.UserID != 0 {
					return safe(ErrSessionInvalid, ErrAuthRequired)
				}
				cb.Status(domain.TelegramAuthRequired)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.authDone:
			}
			self, err := c.client.Self(ctx)
			if err != nil {
				return err
			}
			c.selfID.Store(self.ID)
			if err := cb.Identity(ctx, domain.TelegramAccount{UserID: self.ID, Phone: self.Phone, Username: self.Username, FirstName: self.FirstName, LastName: self.LastName}); err != nil {
				return err
			}
			if _, err := c.peers.Self(ctx); err != nil {
				return err
			}
			return c.gaps.Run(ctx, c.client.API(), self.ID, updates.AuthOptions{OnStart: func(context.Context) { cb.Status(domain.TelegramConnected) }})
		})
	})
	select {
	case failure := <-c.fatal:
		return failure
	default:
	}
	mapped := rpcError(err)
	if errors.Is(mapped, ErrSessionInvalid) {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer stop()
		if err := c.storage.kv.TelegramDeleteState(cleanup, c.account.ID); err != nil {
			return safe(ErrStorage, err)
		}
	}
	return mapped
}

func (c *gotdClient) receive(ctx context.Context, raw tg.MessageClass) error {
	m, ok := mapMessage(c.account.ID, raw)
	if !ok {
		return nil
	}
	kind := domain.TelegramMessageReceived
	if m.Outgoing {
		kind = domain.TelegramMessageSent
	}
	// Never advance persistent pts after an event persistence failure. gotd logs
	// handler errors and otherwise advances state; freeze writes before returning.
	c.storage.mu.Lock()
	defer c.storage.mu.Unlock()
	if c.storage.failed.Load() {
		return ErrStorage
	}
	if err := c.events.PublishTelegramEvent(ctx, domain.TelegramEvent{AccountID: c.account.ID, Type: kind, Message: m}); err != nil {
		c.storage.failed.Store(true)
		failure := safe(ErrStorage, err)
		select {
		case c.fatal <- failure:
		default:
		}
		c.cancel()
		return failure
	}
	return nil
}

func (c *gotdClient) Logout(ctx context.Context) error {
	if _, err := c.client.API().AuthLogOut(ctx); err != nil {
		return rpcError(err)
	}
	c.authMu.Lock()
	c.authorized = false
	c.login = domain.TelegramLogin{State: "new"}
	if c.cancel != nil {
		c.cancel()
	}
	c.authMu.Unlock()
	return nil
}
