package telegram

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
)

func (c *gotdClient) markAuthorized() {
	if !c.authorized {
		c.authorized = true
		close(c.authDone)
	}
	c.phone = ""
	c.codeHash = ""
	c.login.State = "authorized"
	c.login.URL = ""
	c.login.Error = ""
}
func (c *gotdClient) expireLogin() {
	if c.authorized || c.login.ExpiresAt.IsZero() || time.Now().Before(c.login.ExpiresAt) {
		return
	}
	if c.qrCancel != nil {
		c.qrCancel()
		c.qrCancel = nil
	}
	c.phone = ""
	c.codeHash = ""
	c.login.URL = ""
	c.login.State = "expired"
}
func (c *gotdClient) LoginStatus() domain.TelegramLogin {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	c.expireLogin()
	return c.login
}

// Login advances an in-memory state machine. No HTTP request waits for a human.
// Code/password submissions must carry the login_id of the current operation.
func (c *gotdClient) Login(ctx context.Context, action, value, loginID string) (domain.TelegramLogin, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.life == nil || c.life.Err() != nil {
		return c.login, ErrAccountNotRunning
	}
	c.expireLogin()
	if c.authorized {
		return c.login, ErrConflict
	}
	switch action {
	case "qr", "phone":
		if c.login.State == "waiting_qr" || c.login.State == "waiting_code" || c.login.State == "waiting_password" {
			return c.login, ErrConflict
		}
		c.login = domain.TelegramLogin{ID: uuid.NewString(), State: "new", ExpiresAt: time.Now().Add(c.cfg.AuthTTL)}
		if action == "qr" {
			life, stop := context.WithDeadline(c.life, c.login.ExpiresAt)
			c.qrCancel = stop
			id := c.login.ID
			c.login.State = "waiting_qr"
			c.authWG.Add(1)
			ready := make(chan struct{})
			go c.runQR(life, id, stop, ready)
			// Wait for the initial token, never for the user to scan it.
			c.authMu.Unlock()
			select {
			case <-ctx.Done():
				c.authMu.Lock()
				return c.login, ctx.Err()
			case <-ready:
			}
			c.authMu.Lock()
			if c.login.State == "failed" {
				return c.login, ErrUnavailable
			}
			return c.login, nil
		}
		phone := strings.TrimSpace(value)
		if !validPhone(phone) {
			c.login.State = "failed"
			return c.login, ErrInvalidInput
		}
		sent, err := c.auth.SendCode(ctx, phone, auth.SendCodeOptions{})
		if err != nil {
			c.login.State = "failed"
			return c.login, rpcError(err)
		}
		switch s := sent.(type) {
		case *tg.AuthSentCode:
			c.phone = phone
			c.codeHash = s.PhoneCodeHash
			c.login.State = "waiting_code"
		case *tg.AuthSentCodeSuccess:
			c.markAuthorized()
		default:
			c.login.State = "failed"
			return c.login, ErrUnavailable
		}
		return c.login, nil
	case "code":
		if c.login.State != "waiting_code" || loginID != c.login.ID {
			return c.login, ErrConflict
		}
		if value == "" || len(value) > 32 {
			return c.login, ErrInvalidInput
		}
		_, err := c.auth.SignIn(ctx, c.phone, value, c.codeHash)
		if errors.Is(rpcError(err), ErrPasswordRequired) {
			c.phone = ""
			c.codeHash = ""
			c.login.State = "waiting_password"
			return c.login, nil
		}
		if err != nil {
			return c.login, rpcError(err)
		}
		c.markAuthorized()
		return c.login, nil
	case "password":
		if c.login.State != "waiting_password" || loginID != c.login.ID {
			return c.login, ErrConflict
		}
		if value == "" || len(value) > 1024 {
			return c.login, ErrInvalidInput
		}
		// Only transient bytes reach the SRP helper; never add the password to state.
		secret := []byte(value)
		defer clear(secret)
		_, err := c.auth.PasswordWith(ctx, auth.PasswordHashFor(secret))
		if err != nil {
			return c.login, rpcError(err)
		}
		c.markAuthorized()
		return c.login, nil
	default:
		return c.login, ErrInvalidInput
	}
}
func validPhone(s string) bool {
	if len(s) < 7 || len(s) > 16 || s[0] != '+' {
		return false
	}
	for _, v := range s[1:] {
		if v < '0' || v > '9' {
			return false
		}
	}
	return true
}

func (c *gotdClient) runQR(ctx context.Context, id string, stop context.CancelFunc, ready chan struct{}) {
	var once sync.Once
	notify := func() { once.Do(func() { close(ready) }) }
	defer notify()
	defer c.authWG.Done()
	defer stop()
	_, err := c.qrAuth(ctx, c.loginToken, func(ctx context.Context, token qrlogin.Token) error {
		c.authMu.Lock()
		defer c.authMu.Unlock()
		if c.login.ID != id || ctx.Err() != nil {
			return context.Canceled
		}
		c.login.URL = token.URL()
		c.login.TokenExpiresAt = token.Expires()
		notify()
		return nil
	})
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.login.ID != id {
		return
	}
	c.login.URL = ""
	switch {
	case err == nil:
		c.markAuthorized()
	case errors.Is(rpcError(err), ErrPasswordRequired):
		c.login.State = "waiting_password"
	case ctx.Err() != nil:
		c.login.State = "expired"
	default:
		c.login.State = "failed"
		c.login.Error = rpcError(err).Error()
	}
}

// Small RPC boundary keeps the auth state machine testable without a connection.
type authRPC interface {
	SendCode(context.Context, string, auth.SendCodeOptions) (tg.AuthSentCodeClass, error)
	SignIn(context.Context, string, string, string) (*tg.AuthAuthorization, error)
	PasswordWith(context.Context, auth.PasswordHashFunc) (*tg.AuthAuthorization, error)
}
type qrAuthFunc func(context.Context, qrlogin.LoggedIn, func(context.Context, qrlogin.Token) error) (*tg.AuthAuthorization, error)
