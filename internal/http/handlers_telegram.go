package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/digkill/probot/internal/domain"
	tgclient "github.com/digkill/probot/internal/telegram"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

func (s *Server) telegramRoutes(r chi.Router) {
	r.Use(s.telegramAccess)
	r.Get("/accounts", s.telegramAccounts)
	r.Post("/accounts", s.telegramCreate)
	r.Route("/accounts/{accountID}", func(r chi.Router) {
		r.Get("/", s.telegramAccount)
		r.Post("/start", s.telegramStart)
		r.Post("/stop", s.telegramStop)
		r.Post("/logout", s.telegramLogout)
		r.Get("/auth", s.telegramAuthStatus)
		r.Post("/auth/{action}", s.telegramLogin)
		r.Get("/dialogs", s.telegramDialogs)
		r.Get("/peers/{peer}", s.telegramPeer)
		r.Get("/chats/{peer}/messages", s.telegramHistory)
		r.Post("/chats/{peer}/messages", s.telegramSend)
		r.Patch("/chats/{peer}/messages/{messageID}", s.telegramEdit)
		r.Delete("/chats/{peer}/messages/{messageID}", s.telegramDelete)
		r.Post("/chats/{peer}/messages/{messageID}/forward", s.telegramForward)
		r.Get("/events", s.telegramEvents)
		r.Get("/events/stream", s.telegramStream)
	})
}
func (s *Server) telegramAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.telegram == nil {
			telegramError(w, tgclient.ErrUnavailable)
			return
		}
		// Account sessions and private messages are restricted to workspace owners.
		role, err := s.store.UserRole(r.Context(), mustWorkspaceID(r), mustUserID(r))
		if err != nil || role != domain.RoleOwner {
			writeErr(w, http.StatusForbidden, "Workspace owner access required.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		if strings.HasSuffix(r.URL.Path, "/events/stream") {
			cancel()
			next.ServeHTTP(w, r)
			return
		}
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func telegramError(w http.ResponseWriter, err error) {
	status, msg := http.StatusServiceUnavailable, "Telegram service unavailable."
	switch {
	case errors.Is(err, tgclient.ErrAccountNotFound):
		status, msg = 404, tgclient.ErrAccountNotFound.Error()
	case errors.Is(err, tgclient.ErrInvalidInput):
		status, msg = 400, tgclient.ErrInvalidInput.Error()
	case errors.Is(err, tgclient.ErrInvalidCode):
		status, msg = 400, tgclient.ErrInvalidCode.Error()
	case errors.Is(err, tgclient.ErrInvalidPassword):
		status, msg = 400, tgclient.ErrInvalidPassword.Error()
	case errors.Is(err, tgclient.ErrPasswordRequired):
		status, msg = 409, tgclient.ErrPasswordRequired.Error()
	case errors.Is(err, tgclient.ErrAuthRequired), errors.Is(err, tgclient.ErrSessionInvalid):
		status, msg = 409, tgclient.ErrAuthRequired.Error()
	case errors.Is(err, tgclient.ErrAccountNotRunning):
		status, msg = 409, tgclient.ErrAccountNotRunning.Error()
	case errors.Is(err, tgclient.ErrConflict):
		status, msg = 409, tgclient.ErrConflict.Error()
	case errors.Is(err, tgclient.ErrPeerNotFound):
		status, msg = 404, tgclient.ErrPeerNotFound.Error()
	case errors.Is(err, tgclient.ErrFloodWait):
		status, msg = 429, tgclient.ErrFloodWait.Error()
		w.Header().Set("Retry-After", "60")
	case errors.Is(err, context.DeadlineExceeded):
		status, msg = 504, "Telegram operation timed out."
	case errors.Is(err, context.Canceled):
		status, msg = 408, "Telegram operation cancelled."
	}
	// Do not route raw Telegram errors through writeCause, which may log causes.
	writeJSON(w, status, map[string]string{"error": msg})
}
func tgID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		telegramError(w, tgclient.ErrInvalidInput)
		return uuid.Nil, false
	}
	return id, true
}
func tgJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		telegramError(w, tgclient.ErrInvalidInput)
		return false
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		telegramError(w, tgclient.ErrInvalidInput)
		return false
	}
	return true
}
func tgResult(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		telegramError(w, err)
		return
	}
	writeJSON(w, status, v)
}
func tgLimit(r *http.Request) (int, error) {
	if r.URL.Query().Get("limit") == "" {
		return 50, nil
	}
	v, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || v < 1 || v > 100 {
		return 0, tgclient.ErrInvalidInput
	}
	return v, nil
}
func tgMessageID(r *http.Request) (int, error) {
	v, err := strconv.Atoi(chi.URLParam(r, "messageID"))
	if err != nil || v < 1 {
		return 0, tgclient.ErrInvalidInput
	}
	return v, nil
}
func (s *Server) telegramAccounts(w http.ResponseWriter, r *http.Request) {
	v, err := s.telegram.ListAccounts(r.Context(), mustWorkspaceID(r))
	tgResult(w, 200, v, err)
}
func (s *Server) telegramCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !tgJSON(w, r, &req) {
		return
	}
	v, err := s.telegram.CreateAccount(r.Context(), mustWorkspaceID(r), req.Name)
	tgResult(w, 201, v, err)
}
func (s *Server) telegramAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	v, err := s.telegram.GetAccount(r.Context(), mustWorkspaceID(r), id)
	tgResult(w, 200, v, err)
}
func (s *Server) telegramStart(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	err := s.telegram.StartAccount(r.Context(), mustWorkspaceID(r), id)
	tgResult(w, 202, map[string]string{"status": "starting"}, err)
}
func (s *Server) telegramStop(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	err := s.telegram.StopAccount(r.Context(), mustWorkspaceID(r), id)
	tgResult(w, 200, map[string]string{"status": "stopped"}, err)
}
func (s *Server) telegramLogout(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	err := s.telegram.Logout(r.Context(), mustWorkspaceID(r), id)
	tgResult(w, 200, map[string]string{"status": "auth_required"}, err)
}
func (s *Server) telegramAuthStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	v, err := s.telegram.LoginStatus(r.Context(), mustWorkspaceID(r), id)
	tgResult(w, 200, v, err)
}
func (s *Server) telegramLogin(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	action := chi.URLParam(r, "action")
	var req struct {
		Phone    string `json:"phone"`
		Code     string `json:"code"`
		Password string `json:"password"`
		LoginID  string `json:"login_id"`
	}
	if action != "qr" && !tgJSON(w, r, &req) {
		return
	}
	value := ""
	switch action {
	case "qr":
	case "phone":
		value = req.Phone
	case "code":
		value = req.Code
	case "password":
		value = req.Password
	default:
		telegramError(w, tgclient.ErrInvalidInput)
		return
	}
	v, err := s.telegram.Login(r.Context(), mustWorkspaceID(r), id, action, value, req.LoginID)
	req.Password = ""
	req.Code = ""
	value = ""
	tgResult(w, 200, v, err)
}
func (s *Server) telegramDialogs(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	limit, err := tgLimit(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	v, err := s.telegram.GetDialogs(r.Context(), mustWorkspaceID(r), id, limit, r.URL.Query().Get("cursor"))
	tgResult(w, 200, v, err)
}
func (s *Server) telegramPeer(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	v, err := s.telegram.Resolve(r.Context(), mustWorkspaceID(r), id, chi.URLParam(r, "peer"))
	tgResult(w, 200, v, err)
}
func (s *Server) telegramHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	limit, err := tgLimit(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset_id"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			telegramError(w, tgclient.ErrInvalidInput)
			return
		}
	}
	v, err := s.telegram.GetHistory(r.Context(), mustWorkspaceID(r), id, chi.URLParam(r, "peer"), limit, offset)
	tgResult(w, 200, v, err)
}
func (s *Server) telegramSend(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if !tgJSON(w, r, &req) {
		return
	}
	v, err := s.telegram.SendMessage(r.Context(), mustWorkspaceID(r), id, domain.TelegramSendRequest{Peer: chi.URLParam(r, "peer"), Text: req.Text})
	tgResult(w, 201, v, err)
}
func (s *Server) telegramEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	mid, err := tgMessageID(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if !tgJSON(w, r, &req) {
		return
	}
	err = s.telegram.EditMessage(r.Context(), mustWorkspaceID(r), id, chi.URLParam(r, "peer"), mid, req.Text)
	tgResult(w, 200, map[string]bool{"ok": true}, err)
}
func (s *Server) telegramDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	mid, err := tgMessageID(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	revoke := r.URL.Query().Get("revoke") == "true"
	err = s.telegram.DeleteMessage(r.Context(), mustWorkspaceID(r), id, chi.URLParam(r, "peer"), mid, revoke)
	tgResult(w, 200, map[string]bool{"ok": true}, err)
}
func (s *Server) telegramForward(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	mid, err := tgMessageID(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if !tgJSON(w, r, &req) {
		return
	}
	err = s.telegram.ForwardMessage(r.Context(), mustWorkspaceID(r), id, chi.URLParam(r, "peer"), req.To, mid)
	tgResult(w, 200, map[string]bool{"ok": true}, err)
}
func eventCursor(r *http.Request) (int64, error) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("after")
	}
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0, tgclient.ErrInvalidInput
	}
	return v, nil
}
func (s *Server) telegramEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	after, err := eventCursor(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	limit, err := tgLimit(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	v, err := s.telegram.Events(r.Context(), mustWorkspaceID(r), id, after, limit)
	tgResult(w, 200, v, err)
}
func (s *Server) telegramStream(w http.ResponseWriter, r *http.Request) {
	id, ok := tgID(w, r)
	if !ok {
		return
	}
	after, err := eventCursor(r)
	if err != nil {
		telegramError(w, err)
		return
	}
	if _, err := s.telegram.GetAccount(r.Context(), mustWorkspaceID(r), id); err != nil {
		telegramError(w, err)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		telegramError(w, tgclient.ErrUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	ctl := http.NewResponseController(w)
	// Reconnect periodically so membership/JWT expiry are rechecked.
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		rows, err := s.telegram.Events(ctx, mustWorkspaceID(r), id, after, 100)
		if err != nil {
			return
		}
		_ = ctl.SetWriteDeadline(time.Now().Add(5 * time.Second))
		for _, e := range rows {
			b, err := json.Marshal(e)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b); err != nil {
				return
			}
			after = e.ID
		}
		if len(rows) == 0 {
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
		}
		f.Flush()
		_ = ctl.SetWriteDeadline(time.Time{})
		if len(rows) == 100 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// chi's standard access logger includes query strings. Telegram routes log only
// route templates, never user-provided URLs, auth inputs or response bodies.
func safeAccessLogger(next http.Handler) http.Handler {
	normal := middleware.Logger(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/telegram/") {
			normal.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		pattern := chi.RouteContext(r.Context()).RoutePattern()
		log.Printf("component=telegram operation=http method=%s route=%s status=%d duration=%s", r.Method, pattern, wrapped.Status(), time.Since(start))
	})
}
