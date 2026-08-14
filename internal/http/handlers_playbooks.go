package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/digkill/probot/internal/playbook"
	"github.com/digkill/probot/internal/service"
	"github.com/digkill/probot/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleListPlaybooks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, playbook.All())
}

func (s *Server) handleApplyPlaybook(w http.ResponseWriter, r *http.Request) {
	campaignID, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	var req struct {
		PlaybookID string    `json:"playbook_id"`
		ContentID  uuid.UUID `json:"content_id"`
		Enqueue    bool      `json:"enqueue"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.PlaybookID == "" || req.ContentID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "playbook_id and content_id required")
		return
	}
	svc := &service.PlaybookService{Store: s.store, Asynq: s.asynq}
	res, err := svc.Apply(r.Context(), mustWorkspaceID(r), service.ApplyPlaybookInput{
		CampaignID: campaignID,
		ContentID:  req.ContentID,
		PlaybookID: req.PlaybookID,
		Enqueue:    req.Enqueue,
		BaseTime:   time.Now(),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) handleListShortLinks(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListShortLinks(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type item struct {
		store.ShortLink
		ShortURL string `json:"short_url"`
	}
	out := make([]item, 0, len(list))
	for _, l := range list {
		out = append(out, item{
			ShortLink: l,
			ShortURL:  strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/r/" + l.Code,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateShortLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetURL  string     `json:"target_url"`
		Label      string     `json:"label"`
		CampaignID *uuid.UUID `json:"campaign_id"`
		BrandID    *uuid.UUID `json:"brand_id"`
		Code       string     `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetURL == "" {
		writeErr(w, http.StatusBadRequest, "target_url required")
		return
	}
	link := &store.ShortLink{
		WorkspaceID: mustWorkspaceID(r),
		TargetURL:   req.TargetURL,
		Label:       req.Label,
		CampaignID:  req.CampaignID,
		BrandID:     req.BrandID,
		Code:        req.Code,
	}
	if err := s.store.CreateShortLink(r.Context(), link); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"link":      link,
		"short_url": strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/r/" + link.Code,
	})
}

func (s *Server) handleRedirectShortLink(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	link, err := s.store.GetShortLinkByCode(r.Context(), code)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sum := sha256.Sum256([]byte(r.RemoteAddr))
	_ = s.store.RecordShortLinkClick(r.Context(), link.ID, r.Referer(), r.UserAgent(), hex.EncodeToString(sum[:8]))
	http.Redirect(w, r, link.TargetURL, http.StatusFound)
}
