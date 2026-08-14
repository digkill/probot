package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/digkill/probot/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		Name          string `json:"name"`
		WorkspaceName string `json:"workspace_name"`
		WorkspaceSlug string `json:"workspace_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the request.")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.WorkspaceSlug = strings.ToLower(strings.TrimSpace(req.WorkspaceSlug))
	if req.Email == "" || req.Password == "" || req.WorkspaceSlug == "" {
		writeErr(w, http.StatusBadRequest, "Email, password and workspace name are required.")
		return
	}
	if req.WorkspaceName == "" {
		req.WorkspaceName = req.WorkspaceSlug
	}
	if req.Name == "" {
		req.Name = req.Email
	}
	ws, user, err := s.store.CreateWorkspaceWithOwner(r.Context(), req.WorkspaceName, req.WorkspaceSlug, req.Email, req.Password, req.Name)
	if err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	token, err := auth.Issue(s.cfg.JWTSecret, user.ID, user.Email, 72*time.Hour)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":     token,
		"user":      user,
		"workspace": ws,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the request.")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	user, err := s.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "Wrong email or password.")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeErr(w, http.StatusUnauthorized, "Wrong email or password.")
		return
	}
	token, err := auth.Issue(s.cfg.JWTSecret, user.ID, user.Email, 72*time.Hour)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	workspaces, _ := s.store.ListWorkspacesForUser(r.Context(), user.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"user":       user,
		"workspaces": workspaces,
	})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListWorkspacesForUser(r.Context(), mustUserID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
