package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/queue"
	"github.com/digkill/probot/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func (s *Server) engager() *service.Engager {
	return &service.Engager{
		Store:    s.store,
		Registry: s.registry,
		EncKey:   []byte(s.cfg.EncryptionKey),
		AI:       s.ai,
	}
}

func (s *Server) handleListEngagement(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListEngagementTasks(r.Context(), mustWorkspaceID(r), r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateEngagement(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BrandID          *uuid.UUID `json:"brand_id"`
		ChannelID        uuid.UUID  `json:"channel_id"`
		PartnerID        *uuid.UUID `json:"partner_id"`
		MentionID        *uuid.UUID `json:"mention_id"`
		Kind             string     `json:"kind"`
		TargetURL        string     `json:"target_url"`
		TargetExternalID string     `json:"target_external_id"`
		TargetTitle      string     `json:"target_title"`
		DraftText        string     `json:"draft_text"`
		Approve          bool       `json:"approve"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	kind := domain.EngagementKind(req.Kind)
	switch kind {
	case domain.EngageLike, domain.EngageComment, domain.EngageFollow, domain.EngageShare, domain.EngageLinkExchange:
	default:
		writeErr(w, http.StatusBadRequest, "kind must be like|comment|follow|share|link_exchange")
		return
	}
	if req.ChannelID == uuid.Nil || req.TargetURL == "" {
		writeErr(w, http.StatusBadRequest, "channel_id and target_url required")
		return
	}
	status := domain.EngagePending
	if req.Approve && kind == domain.EngageLike {
		status = domain.EngageApproved
	}
	points := map[domain.EngagementKind]int{
		domain.EngageLike: 1, domain.EngageComment: 3, domain.EngageFollow: 1,
		domain.EngageShare: 2, domain.EngageLinkExchange: 5,
	}[kind]
	t := &domain.EngagementTask{
		WorkspaceID:      mustWorkspaceID(r),
		BrandID:          req.BrandID,
		ChannelID:        req.ChannelID,
		PartnerID:        req.PartnerID,
		MentionID:        req.MentionID,
		Kind:             kind,
		Status:           status,
		TargetURL:        req.TargetURL,
		TargetExternalID: req.TargetExternalID,
		TargetTitle:      req.TargetTitle,
		DraftText:        req.DraftText,
		Points:           points,
	}
	if err := s.store.CreateEngagementTask(r.Context(), t); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if status == domain.EngageApproved {
		if err := s.enqueueEngage(t.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleApproveEngagement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := s.store.GetEngagementTask(r.Context(), id)
	if err != nil || task.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if task.Kind == domain.EngageComment && task.DraftText == "" {
		writeErr(w, http.StatusBadRequest, "comment has empty draft — generate or write text first")
		return
	}
	if err := s.store.SetEngagementStatus(r.Context(), id, domain.EngageApproved, "", ""); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.enqueueEngage(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enqueued"})
}

func (s *Server) handleSkipEngagement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := s.store.GetEngagementTask(r.Context(), id)
	if err != nil || task.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if err := s.store.SetEngagementStatus(r.Context(), id, domain.EngageSkipped, "skipped", ""); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "skipped"})
}

func (s *Server) handleDraftEngagement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	text, err := s.engager().DraftComment(r.Context(), mustWorkspaceID(r), id)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"draft_text": text})
}

func (s *Server) handleConfirmEngagementManual(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		ResultURL string `json:"result_url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	task, err := s.store.GetEngagementTask(r.Context(), id)
	if err != nil || task.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	points := task.Points
	if points == 0 {
		points = 1
	}
	_ = s.store.InsertKarmaEvent(r.Context(), task.WorkspaceID, task.ChannelID, task.ID, string(task.Kind), points)
	if err := s.store.SetEngagementStatus(r.Context(), id, domain.EngageDone, "", req.ResultURL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "done"})
}

func (s *Server) handleKarmaSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := s.store.KarmaSummary(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleListPartners(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListLinkPartners(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreatePartner(w http.ResponseWriter, r *http.Request) {
	var p domain.LinkPartner
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p.WorkspaceID = mustWorkspaceID(r)
	if p.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if err := s.store.CreateLinkPartner(r.Context(), &p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdatePartner(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "partnerID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.store.GetLinkPartner(r.Context(), id)
	if err != nil || existing.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "partner not found")
		return
	}
	var p domain.LinkPartner
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p.ID = id
	p.WorkspaceID = existing.WorkspaceID
	if p.Name == "" {
		p.Name = existing.Name
	}
	if err := s.store.UpdateLinkPartner(r.Context(), &p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleEngagementFromMention(w http.ResponseWriter, r *http.Request) {
	mentionID, err := uuid.Parse(chi.URLParam(r, "mentionID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid mention id")
		return
	}
	m, err := s.store.GetMention(r.Context(), mentionID)
	if err != nil || m.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "mention not found")
		return
	}
	var req struct {
		ChannelID uuid.UUID `json:"channel_id"`
		Kind      string    `json:"kind"`
		BrandID   *uuid.UUID `json:"brand_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ChannelID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "channel_id required")
		return
	}
	kind := domain.EngagementKind(req.Kind)
	if kind == "" {
		kind = domain.EngageComment
	}
	brandID := req.BrandID
	if brandID == nil {
		brandID = m.BrandID
	}
	t := &domain.EngagementTask{
		WorkspaceID: mustWorkspaceID(r),
		BrandID:     brandID,
		ChannelID:   req.ChannelID,
		MentionID:   &m.ID,
		Kind:        kind,
		Status:      domain.EngagePending,
		TargetURL:   m.URL,
		TargetTitle: m.Title,
		DraftText:   m.DraftReply,
		Points:      3,
	}
	if err := s.store.CreateEngagementTask(r.Context(), t); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) enqueueEngage(id uuid.UUID) error {
	task, err := queue.NewEngageTask(id)
	if err != nil {
		return err
	}
	_, err = s.asynq.Enqueue(task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}
