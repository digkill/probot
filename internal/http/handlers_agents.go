package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListAgents(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req domain.AIAgent
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.WorkspaceID = mustWorkspaceID(r)
	req.Provider = ai.NormalizeProvider(req.Provider)
	preset := ai.Preset(req.Provider)
	if req.APIKeyEnv == "" {
		if req.Role == domain.AgentImage {
			req.APIKeyEnv = "IMAGE_API_KEY"
		} else {
			req.APIKeyEnv = preset.APIKeyEnv
		}
	}
	if req.BaseURL == "" {
		req.BaseURL = preset.BaseURL
	}
	if req.Model == "" {
		req.Model = preset.Model
	}
	if req.Params == nil {
		req.Params = json.RawMessage(`{}`)
	}
	req.Enabled = true
	if err := s.store.CreateAgent(r.Context(), &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.store.GetAgent(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "agent not found")
		return
	}
	if existing.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var req domain.AIAgent
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.ID = id
	req.WorkspaceID = existing.WorkspaceID
	if req.Params == nil {
		req.Params = existing.Params
	}
	if err := s.store.UpdateAgent(r.Context(), &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteAgent(r.Context(), id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleRunAgent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	agent, err := s.store.GetAgent(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "agent not found")
		return
	}
	if agent.WorkspaceID != mustWorkspaceID(r) || !agent.Enabled {
		writeErr(w, http.StatusForbidden, "agent unavailable")
		return
	}
	var input ai.RunInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if input.Context == nil {
		input.Context = map[string]any{}
	}
	if brandRaw, ok := input.Context["brand_id"]; ok {
		if brandIDStr, ok := brandRaw.(string); ok {
			if brandID, err := uuid.Parse(brandIDStr); err == nil {
				if brand, err := s.store.GetBrand(r.Context(), brandID); err == nil && brand.WorkspaceID == mustWorkspaceID(r) {
					input.Context["brand"] = map[string]any{
						"name": brand.Name, "slug": brand.Slug,
						"canonical_url": brand.CanonicalURL, "tone_of_voice": brand.ToneOfVoice,
						"forbidden_words": brand.ForbiddenWords, "cta_default": brand.CTADefault,
					}
				}
			}
		}
	}
	inJSON, _ := json.Marshal(input)
	run := &domain.AIRun{
		WorkspaceID: mustWorkspaceID(r),
		AgentID:     agent.ID,
		Input:       inJSON,
		Status:      "running",
	}
	if err := s.store.CreateAIRun(r.Context(), run); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, err := s.ai.Run(r.Context(), *agent, input)
	if err != nil {
		_ = s.store.FinishAIRun(r.Context(), run.ID, "failed", json.RawMessage(`{}`), 0, 0, err.Error())
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	outJSON, _ := json.Marshal(out)
	_ = s.store.FinishAIRun(r.Context(), run.ID, "done", outJSON, out.TokensIn, out.TokensOut, "")
	run.Output = outJSON
	run.Status = "done"
	run.TokensIn = out.TokensIn
	run.TokensOut = out.TokensOut
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "result": out})
}

func (s *Server) handleListCrawlSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCrawlSources(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateCrawlSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind        string     `json:"kind"`
		URL         string     `json:"url"`
		Query       string     `json:"query"`
		IntervalSec int        `json:"interval_sec"`
		Purpose     string     `json:"purpose"`
		BrandID     *uuid.UUID `json:"brand_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.IntervalSec <= 0 {
		req.IntervalSec = 3600
	}
	if req.Kind == "" {
		req.Kind = "rss"
	}
	if req.Purpose == "" {
		req.Purpose = "mentions"
	}
	src := &domain.CrawlSource{
		WorkspaceID: mustWorkspaceID(r),
		BrandID:     req.BrandID,
		Kind:        req.Kind,
		URL:         req.URL,
		Query:       req.Query,
		Purpose:     req.Purpose,
		IntervalSec: req.IntervalSec,
		Enabled:     true,
	}
	if err := s.store.InsertCrawlSource(r.Context(), src); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, src)
}

func (s *Server) handleRunCrawlSource(w http.ResponseWriter, r *http.Request) {
	sourceID, err := uuid.Parse(chi.URLParam(r, "sourceID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	src, err := s.store.GetCrawlSource(r.Context(), sourceID)
	if err != nil || src.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "source not found")
		return
	}
	if err := s.enqueueCrawlSource(src); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enqueued"})
}
