package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/store"
	"github.com/jackc/pgx/v5"
)

const (
	resetTokenTTL     = time.Hour
	resetMinInterval  = time.Minute
	minPasswordLength = 8
	maxPasswordBytes  = 72 // bcrypt limit
)

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the request.")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		writeErr(w, http.StatusBadRequest, "Email is required.")
		return
	}
	user, err := s.store.GetUserByEmail(r.Context(), req.Email)
	switch {
	case err == nil:
		s.issuePasswordReset(r, user)
	case !errors.Is(err, pgx.ErrNoRows):
		log.Printf("component=auth operation=password_reset_lookup status=failed err=%v", err)
	}
	// Same answer for unknown emails so the endpoint does not reveal who is registered.
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) issuePasswordReset(r *http.Request, user *domain.User) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		log.Printf("component=auth operation=password_reset_token status=failed err=%v", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	created, err := s.store.CreatePasswordReset(r.Context(), user.ID, hashResetToken(token), resetTokenTTL, resetMinInterval)
	if err != nil {
		log.Printf("component=auth operation=password_reset_store status=failed user=%s err=%v", user.ID, err)
		return
	}
	if !created {
		return
	}
	link := strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/reset-password?token=" + token
	subject, body := resetEmail(r.Header.Get("Accept-Language"), link)
	// Sent in the background so response time does not depend on whether the account exists.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.mailer.Send(ctx, user.Email, subject, body); err != nil {
			log.Printf("component=auth operation=password_reset_mail status=failed user=%s err=%v", user.ID, err)
		}
	}()
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the request.")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeErr(w, http.StatusBadRequest, store.ErrResetInvalid.Error())
		return
	}
	if utf8.RuneCountInString(req.Password) < minPasswordLength {
		writeErr(w, http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	}
	if len(req.Password) > maxPasswordBytes {
		writeErr(w, http.StatusBadRequest, "Password is too long.")
		return
	}
	if _, err := s.store.ResetPassword(r.Context(), hashResetToken(req.Token), req.Password); err != nil {
		if errors.Is(err, store.ErrResetInvalid) {
			writeErr(w, http.StatusBadRequest, store.ErrResetInvalid.Error())
			return
		}
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func hashResetToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func resetEmail(acceptLanguage, link string) (string, string) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(acceptLanguage)), "ru") {
		return "PRobot: сброс пароля", "Здравствуйте!\n\n" +
			"Кто-то запросил сброс пароля для вашего аккаунта PRobot. Чтобы задать новый пароль, откройте ссылку:\n\n" +
			link + "\n\n" +
			"Ссылка действует 1 час и сработает один раз. Если вы не запрашивали сброс, просто проигнорируйте это письмо.\n"
	}
	return "PRobot: reset your password", "Hello,\n\n" +
		"Someone requested a password reset for your PRobot account. To set a new password, open:\n\n" +
		link + "\n\n" +
		"The link is valid for 1 hour and works once. If you did not request this, you can ignore this email.\n"
}
