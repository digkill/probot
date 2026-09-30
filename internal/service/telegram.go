package service

import (
	"context"
	"io"
	"strings"

	"github.com/digkill/probot/internal/domain"
	tgclient "github.com/digkill/probot/internal/telegram"
	"github.com/google/uuid"
)

type TelegramRepository interface {
	TelegramCreateAccount(context.Context, uuid.UUID, string) (domain.TelegramAccount, error)
	TelegramAccount(context.Context, uuid.UUID) (domain.TelegramAccount, error)
	TelegramListAccounts(context.Context, uuid.UUID) ([]domain.TelegramAccount, error)
	TelegramEvents(context.Context, uuid.UUID, int64, int) ([]domain.TelegramEvent, error)
}

type TelegramService struct {
	repo    TelegramRepository
	manager *tgclient.AccountManager
}

func NewTelegramService(repo TelegramRepository, manager *tgclient.AccountManager) (*TelegramService, error) {
	if repo == nil || manager == nil {
		return nil, tgclient.ErrInvalidInput
	}
	return &TelegramService{repo: repo, manager: manager}, nil
}
func (s *TelegramService) CreateAccount(ctx context.Context, ws uuid.UUID, name string) (domain.TelegramAccount, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return domain.TelegramAccount{}, tgclient.ErrInvalidInput
	}
	return s.repo.TelegramCreateAccount(ctx, ws, name)
}
func (s *TelegramService) GetAccount(ctx context.Context, ws, id uuid.UUID) (domain.TelegramAccount, error) {
	a, err := s.repo.TelegramAccount(ctx, id)
	if err != nil {
		return a, err
	}
	if a.WorkspaceID != ws {
		return domain.TelegramAccount{}, tgclient.ErrAccountNotFound
	}
	if runtime, ok := s.manager.GetAccount(id); ok {
		a.Status = runtime.Status
	} else if a.Enabled {
		a.Status = domain.TelegramConnecting
	}
	return a, nil
}
func (s *TelegramService) ListAccounts(ctx context.Context, ws uuid.UUID) ([]domain.TelegramAccount, error) {
	as, err := s.repo.TelegramListAccounts(ctx, ws)
	if err != nil {
		return nil, err
	}
	for i := range as {
		if a, ok := s.manager.GetAccount(as[i].ID); ok {
			as[i].Status = a.Status
		} else if as[i].Enabled {
			as[i].Status = domain.TelegramConnecting
		}
	}
	return as, nil
}
func (s *TelegramService) StartAccount(ctx context.Context, ws, id uuid.UUID) error {
	if _, err := s.GetAccount(ctx, ws, id); err != nil {
		return err
	}
	return s.manager.StartAccount(ctx, id)
}
func (s *TelegramService) StopAccount(ctx context.Context, ws, id uuid.UUID) error {
	if _, err := s.GetAccount(ctx, ws, id); err != nil {
		return err
	}
	return s.manager.StopAccount(ctx, id)
}
func (s *TelegramService) Logout(ctx context.Context, ws, id uuid.UUID) error {
	if _, err := s.GetAccount(ctx, ws, id); err != nil {
		return err
	}
	return s.manager.LogoutAccount(ctx, id)
}
func (s *TelegramService) with(ctx context.Context, ws, id uuid.UUID, authorized bool, fn func(context.Context, tgclient.Client) error) error {
	if _, err := s.GetAccount(ctx, ws, id); err != nil {
		return err
	}
	return s.manager.WithClient(ctx, id, authorized, fn)
}
func (s *TelegramService) Login(ctx context.Context, ws, id uuid.UUID, action, value, loginID string) (out domain.TelegramLogin, err error) {
	err = s.with(ctx, ws, id, false, func(ctx context.Context, c tgclient.Client) error {
		var e error
		out, e = c.Login(ctx, action, value, loginID)
		return e
	})
	return
}
func (s *TelegramService) LoginStatus(ctx context.Context, ws, id uuid.UUID) (out domain.TelegramLogin, err error) {
	err = s.with(ctx, ws, id, false, func(ctx context.Context, c tgclient.Client) error { out = c.LoginStatus(); return nil })
	return
}
func (s *TelegramService) SendMessage(ctx context.Context, ws, id uuid.UUID, req domain.TelegramSendRequest) (out *domain.TelegramMessage, err error) {
	err = s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error {
		var e error
		out, e = c.SendMessage(ctx, req)
		return e
	})
	return
}
func (s *TelegramService) GetHistory(ctx context.Context, ws, id uuid.UUID, peer string, limit, offset int) (out []domain.TelegramMessage, err error) {
	err = s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error {
		var e error
		out, e = c.GetHistory(ctx, peer, limit, offset)
		return e
	})
	return
}
func (s *TelegramService) GetDialogs(ctx context.Context, ws, id uuid.UUID, limit int, cursor string) (out domain.TelegramDialogs, err error) {
	err = s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error {
		var e error
		out, e = c.GetDialogs(ctx, limit, cursor)
		return e
	})
	return
}
func (s *TelegramService) Resolve(ctx context.Context, ws, id uuid.UUID, peer string) (out domain.TelegramPeer, err error) {
	err = s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error {
		var e error
		out, e = c.Resolve(ctx, peer)
		return e
	})
	return
}
func (s *TelegramService) EditMessage(ctx context.Context, ws, id uuid.UUID, peer string, msg int, text string) error {
	return s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error { return c.EditMessage(ctx, peer, msg, text) })
}
func (s *TelegramService) DeleteMessage(ctx context.Context, ws, id uuid.UUID, peer string, msg int, revoke bool) error {
	return s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error { return c.DeleteMessage(ctx, peer, msg, revoke) })
}
func (s *TelegramService) ForwardMessage(ctx context.Context, ws, id uuid.UUID, from, to string, msg int) error {
	return s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error { return c.ForwardMessage(ctx, from, to, msg) })
}
func (s *TelegramService) SendDocument(ctx context.Context, ws, id uuid.UUID, peer, name string, size int64, src io.Reader) error {
	return s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error {
		return c.SendFile(ctx, peer, name, size, src, false)
	})
}
func (s *TelegramService) SendPhoto(ctx context.Context, ws, id uuid.UUID, peer, name string, size int64, src io.Reader) error {
	return s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error {
		return c.SendFile(ctx, peer, name, size, src, true)
	})
}
func (s *TelegramService) Download(ctx context.Context, ws, id uuid.UUID, peer string, msg int, dst io.Writer) error {
	return s.with(ctx, ws, id, true, func(ctx context.Context, c tgclient.Client) error { return c.Download(ctx, peer, msg, dst) })
}
func (s *TelegramService) Events(ctx context.Context, ws, id uuid.UUID, after int64, limit int) ([]domain.TelegramEvent, error) {
	if _, err := s.GetAccount(ctx, ws, id); err != nil {
		return nil, err
	}
	if after < 0 || limit < 1 || limit > 100 {
		return nil, tgclient.ErrInvalidInput
	}
	return s.repo.TelegramEvents(ctx, id, after, limit)
}
