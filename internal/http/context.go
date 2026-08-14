package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

func contextWith(ctx context.Context, key ctxKey, val uuid.UUID) context.Context {
	return context.WithValue(ctx, key, val)
}

func mustUserID(r *http.Request) uuid.UUID {
	return r.Context().Value(ctxUserID).(uuid.UUID)
}

func mustWorkspaceID(r *http.Request) uuid.UUID {
	return r.Context().Value(ctxWorkspaceID).(uuid.UUID)
}
