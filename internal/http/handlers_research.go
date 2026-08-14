package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/queue"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleListAIProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ai.ProviderStatus())
}

func (s *Server) handleBrandAIResearch(w http.ResponseWriter, r *http.Request) {
	brandID, err := uuid.Parse(chi.URLParam(r, "brandID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid brand id")
		return
	}
	brand, err := s.store.GetBrand(r.Context(), brandID)
	if err != nil || brand.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "brand not found")
		return
	}
	task, err := queue.NewAIResearchTask(queue.AIResearchPayload{
		WorkspaceID: brand.WorkspaceID,
		BrandID:     brand.ID,
	})
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.asynq.Enqueue(task); err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "enqueued",
		"brand":  brand.Name,
		"hint":   "Worker/crawler will search the web and ask OpenAI, Claude, Grok, Gemini (whichever keys + research agents are set).",
	})
}

func (s *Server) handleSeedResearchAgents(w http.ResponseWriter, r *http.Request) {
	ws := mustWorkspaceID(r)
	prompt := `You research public reviews, complaints, and brand mentions. Extract only real findings from provided search snippets and live search. Return JSON findings only.`
	created := 0
	for _, p := range ai.Presets() {
		name := p.Name + " research"
		exists, err := s.store.AgentNameExists(r.Context(), ws, name)
		if err != nil {
			writeCause(w, http.StatusInternalServerError, err)
			return
		}
		if exists {
			continue
		}
		a := &domain.AIAgent{
			WorkspaceID:  ws,
			Name:         name,
			Role:         domain.AgentResearch,
			Provider:     p.ID,
			Model:        p.Model,
			BaseURL:      p.BaseURL,
			APIKeyEnv:    p.APIKeyEnv,
			SystemPrompt: prompt,
			Params:       json.RawMessage(`{}`),
			Enabled:      true,
		}
		if err := s.store.CreateAgent(r.Context(), a); err != nil {
			writeCause(w, http.StatusBadRequest, err)
			return
		}
		created++
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "providers": ai.Presets()})
}
