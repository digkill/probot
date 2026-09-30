package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

type fakeAuth struct {
	signErr, passwordErr     error
	codeCalls, passwordCalls int
}

func (f *fakeAuth) SendCode(ctx context.Context, phone string, _ auth.SendCodeOptions) (tg.AuthSentCodeClass, error) {
	f.codeCalls++
	return &tg.AuthSentCode{PhoneCodeHash: "private-code-hash"}, nil
}
func (f *fakeAuth) SignIn(ctx context.Context, phone, code, hash string) (*tg.AuthAuthorization, error) {
	return &tg.AuthAuthorization{}, f.signErr
}
func (f *fakeAuth) PasswordWith(ctx context.Context, _ auth.PasswordHashFunc) (*tg.AuthAuthorization, error) {
	f.passwordCalls++
	return &tg.AuthAuthorization{}, f.passwordErr
}
func authForTest(t *testing.T) (*gotdClient, *fakeAuth, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeAuth{}
	c := &gotdClient{auth: f, cfg: Config{AuthTTL: 5 * time.Minute}, life: ctx, cancel: cancel, authDone: make(chan struct{}), login: domain.TelegramLogin{State: "new"}}
	t.Cleanup(func() { cancel(); c.authWG.Wait() })
	return c, f, cancel
}
func TestPhoneCodePasswordTransitions(t *testing.T) {
	c, f, _ := authForTest(t)
	ctx := context.Background()
	if _, err := c.Login(ctx, "code", "12345", "wrong"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	login, err := c.Login(ctx, "phone", "+15551234567", "")
	if err != nil || login.State != "waiting_code" || login.ID == "" {
		t.Fatalf("%+v %v", login, err)
	}
	if _, err := c.Login(ctx, "phone", "+15551234567", ""); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := c.Login(ctx, "code", "12345", "stale"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	f.signErr = tgerr.New(400, "PHONE_CODE_INVALID")
	if _, err := c.Login(ctx, "code", "12345", login.ID); !errors.Is(err, ErrInvalidCode) {
		t.Fatal(err)
	}
	f.signErr = tgerr.New(401, "SESSION_PASSWORD_NEEDED")
	login, err = c.Login(ctx, "code", "12345", login.ID)
	if err != nil || login.State != "waiting_password" {
		t.Fatalf("%+v %v", login, err)
	}
	f.passwordErr = auth.ErrPasswordInvalid
	if _, err := c.Login(ctx, "password", "sensitive-password", login.ID); !errors.Is(err, ErrInvalidPassword) {
		t.Fatal(err)
	}
	f.passwordErr = nil
	login, err = c.Login(ctx, "password", "sensitive-password", login.ID)
	if err != nil || login.State != "authorized" {
		t.Fatalf("%+v %v", login, err)
	}
	if c.phone != "" || c.codeHash != "" || login.URL != "" {
		t.Fatal("login secrets retained")
	}
	select {
	case <-c.authDone:
	default:
		t.Fatal("runtime not notified")
	}
	if _, err := c.Login(ctx, "password", "sensitive-password", login.ID); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if f.codeCalls != 1 || f.passwordCalls != 2 {
		t.Fatal("unexpected auth RPC count")
	}
}
func TestLoginExpiryAndCancellation(t *testing.T) {
	c, _, cancel := authForTest(t)
	login, err := c.Login(context.Background(), "phone", "+15551234567", "")
	if err != nil {
		t.Fatal(err)
	}
	c.login.ExpiresAt = time.Now().Add(-time.Second)
	if status := c.LoginStatus(); status.State != "expired" {
		t.Fatal(status.State)
	}
	if c.phone != "" || c.codeHash != "" {
		t.Fatal("expired secrets retained")
	}
	if _, err := c.Login(context.Background(), "code", "12345", login.ID); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	cancel()
	if _, err := c.Login(context.Background(), "phone", "+15551234567", ""); !errors.Is(err, ErrAccountNotRunning) {
		t.Fatal(err)
	}
}
func TestQRTokenAndShutdown(t *testing.T) {
	c, _, cancel := authForTest(t)
	c.qrAuth = func(ctx context.Context, _ qrlogin.LoggedIn, show func(context.Context, qrlogin.Token) error) (*tg.AuthAuthorization, error) {
		if err := show(ctx, qrlogin.NewToken([]byte("fake-token"), int(time.Now().Add(time.Minute).Unix()))); err != nil {
			return nil, err
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	login, err := c.Login(context.Background(), "qr", "", "")
	if err != nil || login.State != "waiting_qr" || !strings.HasPrefix(login.URL, "tg://login?token=") {
		t.Fatalf("QR state=%s err=%v", login.State, err)
	}
	cancel()
	done := make(chan struct{})
	go func() { c.authWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("QR goroutine leaked")
	}
	if c.LoginStatus().URL != "" {
		t.Fatal("expired QR retained")
	}
}
func TestQRPasswordTransition(t *testing.T) {
	c, _, _ := authForTest(t)
	c.qrAuth = func(context.Context, qrlogin.LoggedIn, func(context.Context, qrlogin.Token) error) (*tg.AuthAuthorization, error) {
		return nil, tgerr.New(401, "SESSION_PASSWORD_NEEDED")
	}
	login, err := c.Login(context.Background(), "qr", "", "")
	if err != nil || login.State != "waiting_password" {
		t.Fatalf("state=%s err=%v", login.State, err)
	}
	login, err = c.Login(context.Background(), "password", "secret", login.ID)
	if err != nil || login.State != "authorized" {
		t.Fatalf("state=%s err=%v", login.State, err)
	}
}
func TestSafeErrorsPreserveCause(t *testing.T) {
	for _, tt := range []struct {
		cause error
		want  error
	}{
		{tgerr.New(401, "SESSION_PASSWORD_NEEDED"), ErrPasswordRequired},
		{tgerr.New(401, "SESSION_REVOKED"), ErrSessionInvalid},
		{tgerr.New(420, "FLOOD_WAIT_60"), ErrFloodWait},
		{errors.New("private-message-and-session-secret"), ErrUnavailable},
	} {
		got := rpcError(tt.cause)
		if !errors.Is(got, tt.want) || !errors.Is(got, tt.cause) {
			t.Fatalf("classification %v", got)
		}
		if strings.Contains(got.Error(), "private-message") {
			t.Fatal("secret error exposed")
		}
	}
}
