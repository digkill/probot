package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/digkill/probot/internal/auth"
	"github.com/digkill/probot/internal/crawler"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/queue"
	"github.com/digkill/probot/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func (s *Server) handleListBrands(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListBrands(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateBrand(w http.ResponseWriter, r *http.Request) {
	var req domain.Brand
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.WorkspaceID = mustWorkspaceID(r)
	if req.Slug == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name and slug required")
		return
	}
	if err := s.store.CreateBrand(r.Context(), &req); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	_, _, _, _ = s.seedReviewWatch(r.Context(), req, false)
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCampaigns(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var req domain.Campaign
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.WorkspaceID = mustWorkspaceID(r)
	if req.Status == "" {
		req.Status = "draft"
	}
	if err := s.store.CreateCampaign(r.Context(), &req); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleCampaignGraph(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	g, err := s.graph.CampaignGraph(r.Context(), id)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleListContent(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListContent(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateContent(w http.ResponseWriter, r *http.Request) {
	var req domain.ContentPiece
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.WorkspaceID = mustWorkspaceID(r)
	if req.Status == "" {
		req.Status = "draft"
	}
	if err := s.store.CreateContent(r.Context(), &req); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListChannels(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BrandID          uuid.UUID         `json:"brand_id"`
		PlatformSlug     string            `json:"platform_slug"`
		CustomPlatformID *uuid.UUID        `json:"custom_platform_id"`
		Name             string            `json:"name"`
		ExternalRef      string            `json:"external_ref"`
		Token            string            `json:"token"`
		Meta             map[string]string `json:"meta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	wsID := mustWorkspaceID(r)
	ch := &domain.Channel{
		WorkspaceID:      wsID,
		BrandID:          req.BrandID,
		CustomPlatformID: req.CustomPlatformID,
		Name:             req.Name,
		ExternalRef:      req.ExternalRef,
		Health:           "ok",
	}

	creds := platforms.Credentials{
		AccessToken: req.Token,
		ExternalRef: req.ExternalRef,
		Meta:        req.Meta,
	}

	if req.PlatformSlug != "" {
		def, err := s.store.GetPlatformDefinitionBySlug(r.Context(), req.PlatformSlug)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "unknown platform_slug")
			return
		}
		ch.PlatformDefID = &def.ID
		if adapter, ok := s.registry.Get(def.Slug); ok && req.Token != "" {
			connected, err := adapter.Connect(r.Context(), platforms.AuthInput{
				Token:       req.Token,
				ExternalRef: req.ExternalRef,
				Meta:        req.Meta,
			})
			if err != nil {
				writeCause(w, http.StatusBadRequest, err)
				return
			}
			creds = connected
			if ch.ExternalRef == "" {
				ch.ExternalRef = connected.ExternalRef
			}
		}
	}

	blob, err := auth.EncryptJSON([]byte(s.cfg.EncryptionKey), creds)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.store.CreateChannel(r.Context(), ch, blob); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, ch)
}

func (s *Server) handleListPublications(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPublications(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreatePublication(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ContentID    uuid.UUID  `json:"content_id"`
		ChannelID    uuid.UUID  `json:"channel_id"`
		CampaignID   *uuid.UUID `json:"campaign_id"`
		BodyOverride string     `json:"body_override"`
		ScheduledAt  *time.Time `json:"scheduled_at"`
		SortOrder    int        `json:"sort_order"`
		Enqueue      bool       `json:"enqueue"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	pub := &domain.Publication{
		WorkspaceID:    mustWorkspaceID(r),
		ContentID:      req.ContentID,
		ChannelID:      req.ChannelID,
		CampaignID:     req.CampaignID,
		BodyOverride:   req.BodyOverride,
		ScheduledAt:    req.ScheduledAt,
		SortOrder:      req.SortOrder,
		Status:         domain.PubScheduled,
		IdempotencyKey: uuid.NewString(),
	}
	if err := s.store.CreatePublication(r.Context(), pub); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	if req.Enqueue {
		if err := s.enqueuePublish(pub.ID); err != nil {
			writeCause(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, pub)
}

func (s *Server) handleEnqueuePublication(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "publicationID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.enqueuePublish(id); err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enqueued"})
}

func (s *Server) handleConfirmPublication(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "publicationID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		ExternalURL string `json:"external_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExternalURL == "" {
		writeErr(w, http.StatusBadRequest, "external_url required")
		return
	}
	if err := s.store.ConfirmManualPublication(r.Context(), id, req.ExternalURL); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
}

func (s *Server) enqueuePublish(id uuid.UUID) error {
	task, err := queue.NewPublishTask(id)
	if err != nil {
		return err
	}
	_, err = s.asynq.Enqueue(task, asynq.Queue("default"), asynq.MaxRetry(5))
	return err
}

func (s *Server) handleListPlatforms(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPlatformDefinitions(r.Context())
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleListCustomPlatforms(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCustomPlatforms(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateCustomPlatform(w http.ResponseWriter, r *http.Request) {
	var req domain.CustomPlatform
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.WorkspaceID = mustWorkspaceID(r)
	if req.HTTPTemplate == nil {
		req.HTTPTemplate = json.RawMessage(`{}`)
	}
	if err := s.store.CreateCustomPlatform(r.Context(), &req); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleListMentions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.MentionFilter{
		Sentiment: q.Get("sentiment"),
		Severity:  q.Get("severity"),
		Status:    q.Get("status"),
		OpenOnly:  q.Get("open") == "1" || q.Get("open") == "true",
	}
	if raw := q.Get("brand_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid brand_id")
			return
		}
		f.BrandID = &id
	}
	list, err := s.store.ListMentionsFiltered(r.Context(), mustWorkspaceID(r), f)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateMention(w http.ResponseWriter, r *http.Request) {
	var req domain.Mention
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.WorkspaceID = mustWorkspaceID(r)
	if req.Status == "" {
		req.Status = "new"
	}
	if req.Sentiment == "" {
		req.Sentiment, req.Severity = crawler.Classify(req.Title, req.Snippet)
	}
	if req.Sentiment == "negative" && req.Severity == "high" && req.Status == "new" {
		req.Status = "escalated"
	}
	sum := sha256.Sum256([]byte(req.URL + "|" + req.Title))
	hash := hex.EncodeToString(sum[:])
	if err := s.store.UpsertMention(r.Context(), &req, hash); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func mentionHash(url, title string) string {
	sum := sha256.Sum256([]byte(url + "|" + title))
	return hex.EncodeToString(sum[:])
}

var _ = mentionHash
