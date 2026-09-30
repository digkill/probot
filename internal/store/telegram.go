package store

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/digkill/probot/internal/auth"
	"github.com/digkill/probot/internal/domain"
	tgclient "github.com/digkill/probot/internal/telegram"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TelegramStore struct {
	Pool *pgxpool.Pool
	key  []byte
}

func NewTelegramStore(pool *pgxpool.Pool, key string) (*TelegramStore, error) {
	if pool == nil || len(key) != 32 {
		return nil, tgclient.ErrInvalidInput
	}
	return &TelegramStore{Pool: pool, key: []byte(key)}, nil
}

const telegramAccountColumns = `id,workspace_id,name,enabled,telegram_user_id,phone,username,first_name,last_name,status,created_at,updated_at,last_connected_at`

func scanTelegramAccount(row pgx.Row) (domain.TelegramAccount, error) {
	var a domain.TelegramAccount
	err := row.Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Enabled, &a.UserID, &a.Phone, &a.Username, &a.FirstName, &a.LastName, &a.Status, &a.CreatedAt, &a.UpdatedAt, &a.LastConnectedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tgclient.ErrAccountNotFound
	}
	return a, err
}
func (s *TelegramStore) TelegramCreateAccount(ctx context.Context, ws uuid.UUID, name string) (domain.TelegramAccount, error) {
	return scanTelegramAccount(s.Pool.QueryRow(ctx, `INSERT INTO telegram_accounts(workspace_id,name) VALUES($1,$2) RETURNING `+telegramAccountColumns, ws, name))
}
func (s *TelegramStore) TelegramAccount(ctx context.Context, id uuid.UUID) (domain.TelegramAccount, error) {
	return scanTelegramAccount(s.Pool.QueryRow(ctx, `SELECT `+telegramAccountColumns+` FROM telegram_accounts WHERE id=$1`, id))
}
func (s *TelegramStore) listAccounts(ctx context.Context, where string, args ...any) ([]domain.TelegramAccount, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+telegramAccountColumns+` FROM telegram_accounts WHERE `+where+` ORDER BY created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TelegramAccount{}
	for rows.Next() {
		a, err := scanTelegramAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *TelegramStore) TelegramListAccounts(ctx context.Context, ws uuid.UUID) ([]domain.TelegramAccount, error) {
	return s.listAccounts(ctx, "workspace_id=$1", ws)
}
func (s *TelegramStore) TelegramEnabledAccounts(ctx context.Context) ([]domain.TelegramAccount, error) {
	return s.listAccounts(ctx, "enabled=true")
}
func (s *TelegramStore) TelegramSetEnabled(ctx context.Context, id uuid.UUID, v bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE telegram_accounts SET enabled=$2,updated_at=now() WHERE id=$1`, id, v)
	return err
}
func (s *TelegramStore) TelegramSetStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE telegram_accounts SET status=$2,updated_at=now(),last_connected_at=CASE WHEN $2='connected' THEN now() ELSE last_connected_at END WHERE id=$1`, id, status)
	return err
}
func (s *TelegramStore) TelegramSaveIdentity(ctx context.Context, a domain.TelegramAccount) error {
	_, err := s.Pool.Exec(ctx, `UPDATE telegram_accounts SET telegram_user_id=$2,phone=$3,username=$4,first_name=$5,last_name=$6,updated_at=now() WHERE id=$1`, a.ID, a.UserID, a.Phone, a.Username, a.FirstName, a.LastName)
	return err
}
func (s *TelegramStore) TelegramLoad(ctx context.Context, id uuid.UUID, key string) ([]byte, error) {
	var v []byte
	err := s.Pool.QueryRow(ctx, `SELECT value FROM telegram_state WHERE account_id=$1 AND key=$2`, id, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}
func (s *TelegramStore) TelegramSave(ctx context.Context, id uuid.UUID, key string, v []byte) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO telegram_state(account_id,key,value) VALUES($1,$2,$3) ON CONFLICT(account_id,key) DO UPDATE SET value=excluded.value,updated_at=now()`, id, key, v)
	return err
}
func (s *TelegramStore) TelegramDeleteState(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM telegram_state WHERE account_id=$1`, id)
	return err
}

// A dedicated connection owns the session-level advisory lock. Losing it cancels
// the runtime; closing it releases the lock even when the request was cancelled.
func (s *TelegramStore) TelegramAcquire(ctx context.Context, id uuid.UUID) (context.Context, func(), error) {
	conn, err := pgx.ConnectConfig(ctx, s.Pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}
	key := int64(binary.BigEndian.Uint64(id[:8]))
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&ok); err != nil {
		cleanup()
		return nil, nil, err
	}
	if !ok {
		cleanup()
		return nil, nil, tgclient.ErrConflict
	}
	life, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cleanup()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-life.Done():
				return
			case <-ticker.C:
				pingCtx, stop := context.WithTimeout(life, 3*time.Second)
				err := conn.Ping(pingCtx)
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	var once sync.Once
	return life, func() { once.Do(func() { cancel(); <-done }) }, nil
}
func eventAAD(id uuid.UUID, kind string, chat int64, msg int) []byte {
	return []byte(fmt.Sprintf("telegram-event:%s:%s:%d:%d", id, kind, chat, msg))
}
func (s *TelegramStore) PublishTelegramEvent(ctx context.Context, e domain.TelegramEvent) error {
	plain, err := json.Marshal(e.Message)
	if err != nil {
		return err
	}
	payload, err := auth.Seal(s.key, plain, eventAAD(e.AccountID, e.Type, e.Message.ChatID, e.Message.ID))
	if err != nil {
		return err
	}
	// Serialize commits per account before allocating the BIGSERIAL cursor. Without
	// this, a later ID could become visible first and a subscriber could skip the
	// earlier transaction. Two-int advisory keys are distinct from runtime leases.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	lockID := int32(binary.BigEndian.Uint32(e.AccountID[:4]))
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1::int,$2::int)`, 1413956982, lockID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO telegram_events(account_id,type,chat_id,message_id,payload) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id,type,chat_id,message_id) DO NOTHING`, e.AccountID, e.Type, e.Message.ChatID, e.Message.ID, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *TelegramStore) TelegramEvents(ctx context.Context, id uuid.UUID, after int64, limit int) ([]domain.TelegramEvent, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,type,chat_id,message_id,payload,created_at FROM telegram_events WHERE account_id=$1 AND id>$2 ORDER BY id LIMIT $3`, id, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TelegramEvent{}
	for rows.Next() {
		var e domain.TelegramEvent
		var payload []byte
		var chat int64
		var msg int
		e.AccountID = id
		if err := rows.Scan(&e.ID, &e.Type, &chat, &msg, &payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		plain, err := auth.Open(s.key, payload, eventAAD(id, e.Type, chat, msg))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(plain, &e.Message); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
