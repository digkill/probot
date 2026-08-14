package httpapi

import (
	"fmt"
	"testing"

	"github.com/digkill/probot/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPublicMessageDuplicateEmail(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "users_email_key",
		Message:        `duplicate key value violates unique constraint "users_email_key"`,
		Detail:         "Key (email)=(alexcatleva@gmail.com) already exists.",
	}
	wrapped := fmt.Errorf("create user: %w", pgErr)
	got := publicMessage(wrapped)
	want := "This email is already registered. Sign in instead."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPublicMessageSentinel(t *testing.T) {
	got := publicMessage(store.ErrEmailTaken)
	if got != store.ErrEmailTaken.Error() {
		t.Fatalf("got %q", got)
	}
}

func TestSanitizeSQLString(t *testing.T) {
	raw := `create user: ERROR: duplicate key value violates unique constraint "users_email_key" (SQLSTATE 23505)`
	got := sanitizeString(raw)
	want := "This email is already registered. Sign in instead."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
