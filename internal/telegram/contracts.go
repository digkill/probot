// Package telegram contains the MTProto infrastructure boundary. gotd entities
// must not escape this package; transports and services use domain DTOs.
package telegram

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrAccountNotFound   = errors.New("Telegram account not found")
	ErrAccountNotRunning = errors.New("Telegram account is not running")
	ErrAuthRequired      = errors.New("Telegram authentication required")
	ErrInvalidCode       = errors.New("Invalid or expired Telegram code")
	ErrPasswordRequired  = errors.New("Telegram 2FA password required")
	ErrInvalidPassword   = errors.New("Invalid Telegram password")
	ErrPeerNotFound      = errors.New("Telegram peer not found; load dialogs first for numeric IDs")
	ErrFloodWait         = errors.New("Telegram rate limit; try again later")
	ErrConflict          = errors.New("Telegram operation conflicts with current state")
	ErrInvalidInput      = errors.New("Invalid Telegram request")
	ErrUnavailable       = errors.New("Telegram service unavailable")
	ErrStorage           = errors.New("Telegram storage unavailable")
	ErrSessionInvalid    = errors.New("Telegram session invalid")
)

// SafeError preserves the cause for errors.Is/As without disclosing RPC data.
type SafeError struct {
	Kind  error
	Cause error
}

func (e *SafeError) Error() string   { return e.Kind.Error() }
func (e *SafeError) Unwrap() []error { return []error{e.Kind, e.Cause} }
func safe(kind, cause error) error {
	if cause == nil {
		return nil
	}
	return &SafeError{Kind: kind, Cause: cause}
}

type Config struct {
	AppID         int
	AppHash       string
	EncryptionKey string
	RPCRate       float64
	MaxAccounts   int
	AuthTTL       time.Duration
}

// Repository owns account metadata and an exclusive, cancellable account lease.
// The release function must release the lease even after ctx cancellation.
type Repository interface {
	TelegramAccount(context.Context, uuid.UUID) (domain.TelegramAccount, error)
	TelegramEnabledAccounts(context.Context) ([]domain.TelegramAccount, error)
	TelegramSetEnabled(context.Context, uuid.UUID, bool) error
	TelegramDeleteState(context.Context, uuid.UUID) error
	TelegramSaveIdentity(context.Context, domain.TelegramAccount) error
	TelegramSetStatus(context.Context, uuid.UUID, string) error
	TelegramAcquire(context.Context, uuid.UUID) (context.Context, func(), error)
}

type KV interface {
	TelegramLoad(context.Context, uuid.UUID, string) ([]byte, error)
	TelegramSave(context.Context, uuid.UUID, string, []byte) error
	TelegramDeleteState(context.Context, uuid.UUID) error
}

type SessionStorage interface {
	Load(context.Context, uuid.UUID) ([]byte, error)
	Save(context.Context, uuid.UUID, []byte) error
	Delete(context.Context, uuid.UUID) error
}

type EventSink interface {
	PublishTelegramEvent(context.Context, domain.TelegramEvent) error
}

type Callbacks struct {
	Status    func(string)
	FloodWait func(time.Duration)
	Identity  func(context.Context, domain.TelegramAccount) error
}

// Client is the replaceable MTProto boundary used by the manager and fakes.
type Client interface {
	Run(context.Context, Callbacks) error
	Login(context.Context, string, string, string) (domain.TelegramLogin, error)
	LoginStatus() domain.TelegramLogin
	Logout(context.Context) error
	Resolve(context.Context, string) (domain.TelegramPeer, error)
	SendMessage(context.Context, domain.TelegramSendRequest) (*domain.TelegramMessage, error)
	GetHistory(context.Context, string, int, int) ([]domain.TelegramMessage, error)
	GetDialogs(context.Context, int, string) (domain.TelegramDialogs, error)
	EditMessage(context.Context, string, int, string) error
	DeleteMessage(context.Context, string, int, bool) error
	ForwardMessage(context.Context, string, string, int) error
	SendFile(context.Context, string, string, int64, io.Reader, bool) error
	Download(context.Context, string, int, io.Writer) error
}

type Factory func(domain.TelegramAccount) (Client, error)
