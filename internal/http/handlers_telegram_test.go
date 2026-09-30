package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgclient "github.com/digkill/probot/internal/telegram"
	"github.com/go-chi/chi/v5"
)

func TestTelegramErrorsDoNotExposeRPCCause(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{&tgclient.SafeError{Kind: tgclient.ErrInvalidCode, Cause: errors.New("login-code=123456")}, 400},
		{&tgclient.SafeError{Kind: tgclient.ErrSessionInvalid, Cause: errors.New("auth-key=secret")}, 409},
		{errors.New("private-message-and-session"), 503},
		{tgclient.ErrFloodWait, 429}, {tgclient.ErrAccountNotFound, 404}, {context.DeadlineExceeded, 504},
	} {
		w := httptest.NewRecorder()
		telegramError(w, tc.err)
		if w.Code != tc.status {
			t.Fatalf("status=%d", w.Code)
		}
		for _, secret := range []string{"123456", "auth-key", "private-message"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("error exposed secret")
			}
		}
	}
}
func TestTelegramJSONRejectsOversizeAndTrailingInput(t *testing.T) {
	for _, body := range []string{`{"text":"a"} {"text":"b"}`, `{"unexpected":"x"}`, `{"text":"` + strings.Repeat("a", 9000) + `"}`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		var req struct {
			Text string `json:"text"`
		}
		if tgJSON(w, r, &req) || w.Code != 400 {
			t.Fatal("invalid body accepted")
		}
	}
}
func TestTelegramAccessLogRedactsQueryAndPathParameters(t *testing.T) {
	var out bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(previous)
	router := chi.NewRouter()
	router.Use(safeAccessLogger)
	router.Get("/api/v1/workspaces/{ws}/telegram/accounts/{id}/chats/{peer}/messages", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/workspaces/x/telegram/accounts/y/chats/private-peer/messages?password=secret-password&token=qr-secret", nil))
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	for _, secret := range []string{"secret-password", "qr-secret", "private-peer"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("access log exposed secret")
		}
	}
	if !strings.Contains(out.String(), "{peer}") {
		t.Fatal("route template absent")
	}
}
func TestTelegramAccountRouteMatchesWithoutTrailingSlash(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/accounts/{accountID}", func(r chi.Router) { r.Get("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/accounts/id", nil))
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
}
