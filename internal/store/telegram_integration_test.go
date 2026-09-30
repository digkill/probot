package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/digkill/probot/internal/domain"
	tgclient "github.com/digkill/probot/internal/telegram"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in: use a disposable database. Each run uses and drops only its own schema.
func TestTelegramPostgresIntegration(t *testing.T) {
	url := os.Getenv("TEST_TELEGRAM_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_TELEGRAM_DATABASE_URL to an isolated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal("connect to integration database:", err)
	}
	defer admin.Close(context.Background())
	schema := "telegram_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(string(b), "-- +goose Down")[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(file), err)
		}
	}
	key := "test-only-key-012345678901234567"
	s, err := NewTelegramStore(pool, key)
	if err != nil {
		t.Fatal(err)
	}
	var ws uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('Test','test') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	a, err := s.TelegramCreateAccount(ctx, ws, "Test account")
	if err != nil {
		t.Fatal(err)
	}
	session, err := tgclient.NewSessions(s, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Save(ctx, a.ID, []byte("secret-session")); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := tgclient.NewSessions(s, key)
	if b, err := reloaded.Load(ctx, a.ID); err != nil || string(b) != "secret-session" {
		t.Fatalf("session roundtrip: %v", err)
	}
	life, release, err := s.TelegramAcquire(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, unlock, err := s.TelegramAcquire(ctx, a.ID); err == nil {
		unlock()
		release()
		t.Fatal("duplicate runtime lease granted")
	}
	release()
	if life.Err() == nil {
		t.Fatal("lease context remains live")
	}
	_, unlock, err := s.TelegramAcquire(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	e := domain.TelegramEvent{AccountID: a.ID, Type: domain.TelegramMessageReceived, Message: domain.TelegramMessage{ID: 7, AccountID: a.ID, ChatID: 42, Text: "private", Date: time.Now()}}
	if err := s.PublishTelegramEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.PublishTelegramEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	events, err := s.TelegramEvents(ctx, a.ID, 0, 100)
	if err != nil || len(events) != 1 || events[0].Message.Text != "private" {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	later, err := s.TelegramEvents(ctx, a.ID, events[0].ID, 100)
	if err != nil || len(later) != 0 {
		t.Fatal("cursor replayed old event")
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM telegram_events WHERE account_id=$1`, a.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private") {
		t.Fatal("plaintext message stored")
	}
	// Validate down migration only for the new subsystem, then reapply it.
	b, err := os.ReadFile("../../migrations/00008_telegram_client.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(b), "-- +goose Down")
	if _, err := pool.Exec(ctx, parts[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, parts[0]); err != nil {
		t.Fatal(err)
	}
}
