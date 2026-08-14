package httpapi

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/digkill/probot/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": publicMessage(errors.New(msg))})
}

func writeCause(w http.ResponseWriter, status int, err error) {
	if err == nil {
		writeJSON(w, status, map[string]string{"error": "Something went wrong. Please try again."})
		return
	}
	if mappedStatus, msg, ok := mapKnownError(err); ok {
		if mappedStatus >= 500 {
			log.Printf("http error: %v", err)
		}
		writeJSON(w, mappedStatus, map[string]string{"error": msg})
		return
	}
	msg := publicMessage(err)
	if status >= 500 || isInternalLooking(err.Error()) {
		log.Printf("http error: %v", err)
	}
	if isInternalLooking(err.Error()) && status >= 500 {
		msg = "Something went wrong. Please try again."
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func mapKnownError(err error) (int, string, bool) {
	switch {
	case errors.Is(err, store.ErrEmailTaken):
		return http.StatusConflict, store.ErrEmailTaken.Error(), true
	case errors.Is(err, store.ErrWorkspaceSlugTaken):
		return http.StatusConflict, store.ErrWorkspaceSlugTaken.Error(), true
	case errors.Is(err, pgx.ErrNoRows):
		return http.StatusNotFound, "Not found.", true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		status, msg := mapPgError(pgErr)
		return status, msg, true
	}
	return 0, "", false
}

func mapPgError(pgErr *pgconn.PgError) (int, string) {
	switch pgErr.Code {
	case "23505":
		return http.StatusConflict, uniqueConstraintMessage(pgErr)
	case "23503":
		return http.StatusBadRequest, "Related item not found."
	case "23502":
		return http.StatusBadRequest, "A required field is missing."
	case "23514":
		return http.StatusBadRequest, "Invalid value."
	case "22P02":
		return http.StatusBadRequest, "Invalid value."
	default:
		return http.StatusBadRequest, "Could not save. Please try again."
	}
}

func uniqueConstraintMessage(pgErr *pgconn.PgError) string {
	name := strings.ToLower(pgErr.ConstraintName + " " + pgErr.Message + " " + pgErr.Detail)
	switch {
	case strings.Contains(name, "users_email") || strings.Contains(name, "email"):
		return "This email is already registered. Sign in instead."
	case strings.Contains(name, "workspaces_slug"):
		return "This workspace slug is already taken."
	case strings.Contains(name, "brands") && strings.Contains(name, "slug"):
		return "A brand with this slug already exists."
	case strings.Contains(name, "custom_platforms") && strings.Contains(name, "slug"):
		return "A platform with this slug already exists."
	case strings.Contains(name, "short_links") || strings.Contains(name, "code"):
		return "This short link code is already in use."
	case strings.Contains(name, "idempotency"):
		return "This publication was already created."
	default:
		return "This value is already in use."
	}
}

func publicMessage(err error) string {
	if err == nil {
		return "Something went wrong. Please try again."
	}
	if _, msg, ok := mapKnownError(err); ok {
		return msg
	}
	return sanitizeString(err.Error())
}

func sanitizeString(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "Something went wrong. Please try again."
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "users_email") || (strings.Contains(lower, "duplicate key") && strings.Contains(lower, "email")):
		return "This email is already registered. Sign in instead."
	case strings.Contains(lower, "workspaces_slug"):
		return "This workspace slug is already taken."
	case strings.Contains(lower, "already registered"):
		return "This email is already registered. Sign in instead."
	case strings.Contains(lower, "api key not configured"):
		return "AI provider is not configured. Add an API key in settings."
	case strings.Contains(lower, "status 401") || strings.Contains(lower, "incorrect api key") || strings.Contains(lower, "invalid_api_key"):
		return "AI provider rejected the API key."
	case strings.Contains(lower, "status 429"):
		return "AI provider rate limit hit. Try again in a minute."
	case isInternalLooking(lower):
		return "Something went wrong. Please try again."
	default:
		return msg
	}
}

func isInternalLooking(msg string) bool {
	lower := strings.ToLower(msg)
	needles := []string{
		"sqlstate",
		"duplicate key",
		"violates unique",
		"violates foreign key",
		"violates not-null",
		"violates check",
		"pq:",
		"connection refused",
		"dial tcp",
		"connect: ",
		"tls:",
		"panic:",
		"goroutine",
		"pgx",
	}
	for _, n := range needles {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}
