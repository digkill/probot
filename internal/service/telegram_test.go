package service

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/digkill/probot/internal/domain"
	tgclient "github.com/digkill/probot/internal/telegram"
	"github.com/google/uuid"
)

type ownershipRepo struct {
	tgclient.Repository
	TelegramRepository
	account      domain.TelegramAccount
	eventsCalled bool
}

func (r *ownershipRepo) TelegramAccount(context.Context, uuid.UUID) (domain.TelegramAccount, error) {
	return r.account, nil
}
func (r *ownershipRepo) TelegramEvents(context.Context, uuid.UUID, int64, int) ([]domain.TelegramEvent, error) {
	r.eventsCalled = true
	return nil, nil
}
func TestTelegramRejectsOtherWorkspaceBeforeRuntimeOrEvents(t *testing.T) {
	owner, other, id := uuid.New(), uuid.New(), uuid.New()
	r := &ownershipRepo{account: domain.TelegramAccount{ID: id, WorkspaceID: owner}}
	called := false
	m, err := tgclient.NewAccountManager(context.Background(), r, func(domain.TelegramAccount) (tgclient.Client, error) {
		called = true
		return nil, tgclient.ErrUnavailable
	}, log.New(io.Discard, "", 0), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop(context.Background())
	s, err := NewTelegramService(r, m)
	if err != nil {
		t.Fatal(err)
	}
	checks := []func() error{
		func() error { _, err := s.GetAccount(context.Background(), other, id); return err },
		func() error { return s.StartAccount(context.Background(), other, id) },
		func() error { return s.StopAccount(context.Background(), other, id) },
		func() error { return s.Logout(context.Background(), other, id) },
		func() error { _, err := s.Login(context.Background(), other, id, "qr", "", ""); return err },
		func() error {
			_, err := s.SendMessage(context.Background(), other, id, domain.TelegramSendRequest{})
			return err
		},
		func() error { _, err := s.Events(context.Background(), other, id, 0, 50); return err },
	}
	for _, check := range checks {
		if err := check(); !errors.Is(err, tgclient.ErrAccountNotFound) {
			t.Fatal(err)
		}
	}
	if called || r.eventsCalled {
		t.Fatal("cross-workspace request reached infrastructure")
	}
}
