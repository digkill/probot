package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrResetInvalid = errors.New("This reset link is invalid or has expired.")

// CreatePasswordReset stores a reset token hash unless one was issued for the user
// within minInterval; it reports whether a new token was stored.
func (s *Store) CreatePasswordReset(ctx context.Context, userID uuid.UUID, tokenHash []byte, ttl, minInterval time.Duration) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO password_resets (token_hash, user_id, expires_at)
		SELECT $1, $2, now() + make_interval(secs => $3)
		WHERE NOT EXISTS (
			SELECT 1 FROM password_resets
			WHERE user_id = $2 AND created_at > now() - make_interval(secs => $4)
		)
	`, tokenHash, userID, ttl.Seconds(), minInterval.Seconds())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ResetPassword consumes a valid token, sets the new password and invalidates
// every other outstanding token of that user.
func (s *Store) ResetPassword(ctx context.Context, tokenHash []byte, newPassword string) (uuid.UUID, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return uuid.Nil, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM password_resets
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		FOR UPDATE
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrResetInvalid
	}
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`, string(hash), userID); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE password_resets SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}
