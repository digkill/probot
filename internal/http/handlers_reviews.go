package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/crawler"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/queue"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) enqueueCrawlSource(src *domain.CrawlSource) error {
	task, err := queue.NewCrawlTask(queue.CrawlPayload{
		SourceID:    src.ID,
		WorkspaceID: src.WorkspaceID,
		BrandID:     src.BrandID,
		URL:         src.URL,
		Kind:        src.Kind,
		Query:       src.Query,
		Purpose:     src.Purpose,
	})
	if err != nil {
		return err
	}
	_, err = s.asynq.Enqueue(task)
	return err
}

func (s *Server) pickAgent(r *http.Request, agentID *uuid.UUID, prefer domain.AgentRole) (*domain.AIAgent, error) {
	agents, err := s.store.ListAgents(r.Context(), mustWorkspaceID(r))
	if err != nil {
		return nil, err
	}
	var preferred, fallback *domain.AIAgent
	for i := range agents {
		a := &agents[i]
		if !a.Enabled {
			continue
		}
		if agentID != nil && a.ID == *agentID {
			return a, nil
		}
		if a.Role == prefer && preferred == nil {
			preferred = a
		}
		if a.Role == domain.AgentContent && fallback == nil {
			fallback = a
		}
	}
	if preferred != nil {
		return preferred, nil
	}
	return fallback, nil
}

func (s *Server) seedReviewWatch(ctx context.Context, brand domain.Brand, refresh bool) (created, enqueued int, sources []domain.CrawlSource, err error) {
	if refresh {
		if err := s.store.DisableReviewSources(ctx, brand.ID); err != nil {
			return 0, 0, nil, err
		}
	}
	planned := crawler.ReviewWatchSources(brand)
	for i := range planned {
		src := planned[i]
		wasNew, err := s.store.EnsureCrawlSource(ctx, &src)
		if err != nil {
			return created, enqueued, sources, err
		}
		if wasNew {
			created++
		}
		if err := s.enqueueCrawlSource(&src); err == nil {
			enqueued++
		}
		sources = append(sources, src)
	}
	return created, enqueued, sources, nil
}

func (s *Server) handleSeedReviewWatch(w http.ResponseWriter, r *http.Request) {
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
	created, enqueued, sources, err := s.seedReviewWatch(r.Context(), *brand, true)
	if err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"watch_query": crawler.BrandWatchQuery(brand.Name, brand.Slug, brand.CanonicalURL),
		"created":     created,
		"enqueued":    enqueued,
		"sources":     sources,
	})
}

func (s *Server) handleMentionInbox(w http.ResponseWriter, r *http.Request) {
	var brandID *uuid.UUID
	if raw := r.URL.Query().Get("brand_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid brand_id")
			return
		}
		brandID = &id
	}
	inbox, err := s.store.MentionInbox(r.Context(), mustWorkspaceID(r), brandID)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, inbox)
}

func (s *Server) mentionBrand(r *http.Request, m *domain.Mention, override *uuid.UUID) *domain.Brand {
	id := override
	if id == nil {
		id = m.BrandID
	}
	if id == nil {
		return nil
	}
	brand, err := s.store.GetBrand(r.Context(), *id)
	if err != nil {
		return nil
	}
	return brand
}

func (s *Server) handleMentionObjection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "mentionID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid mention id")
		return
	}
	m, err := s.store.GetMention(r.Context(), id)
	if err != nil || m.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "mention not found")
		return
	}
	var req struct {
		AgentID *uuid.UUID `json:"agent_id"`
		BrandID *uuid.UUID `json:"brand_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	brand := s.mentionBrand(r, m, req.BrandID)
	agent, err := s.pickAgent(r, req.AgentID, domain.AgentReputation)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}

	text := crawler.DraftObjection(*m, brand)
	via := "template"
	if agent != nil {
		ctxMap := map[string]any{
			"mention": map[string]any{
				"source": m.Source, "url": m.URL, "title": m.Title,
				"snippet": m.Snippet, "author": m.Author,
				"sentiment": m.Sentiment, "severity": m.Severity,
			},
			"goal":           "handle_objection",
			"fallback_draft": text,
		}
		if brand != nil {
			ctxMap["brand"] = brandContextMap(brand)
		}
		input := ai.RunInput{
			Prompt: `You are a brand reputation specialist. Write a public reply that handles this negative review or objection.
Rules:
- Acknowledge the specific complaint without being defensive
- Correct facts briefly only if clearly needed
- Offer one concrete next step (support, fix, how to reach us)
- Stay on-brand; no legal threats, no insults, no fake discounts, no hashtag spam
- Keep it short (2-6 sentences)
- Match the language of the original mention
Return only the reply text.`,
			Context: ctxMap,
		}
		if out, err := s.ai.Run(r.Context(), *agent, input); err == nil && out.Text != "" {
			text = out.Text
			via = "ai"
		}
	}

	status := "reviewed"
	if m.Sentiment == "negative" && (m.Severity == "high" || m.Severity == "medium") {
		status = "escalated"
	}
	_ = s.store.SetMentionDraft(r.Context(), m.ID, text, status)
	m.DraftReply = text
	m.Status = status
	writeJSON(w, http.StatusOK, map[string]any{"mention": m, "draft_reply": text, "via": via})
}

func (s *Server) handleMentionStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "mentionID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid mention id")
		return
	}
	m, err := s.store.GetMention(r.Context(), id)
	if err != nil || m.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "mention not found")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	switch req.Status {
	case "new", "reviewed", "replied", "ignored", "escalated", "false_positive":
	default:
		writeErr(w, http.StatusBadRequest, "status must be new|reviewed|replied|ignored|escalated|false_positive")
		return
	}
	if err := s.store.SetMentionStatus(r.Context(), m.ID, req.Status); err != nil {
		writeCause(w, http.StatusBadRequest, err)
		return
	}
	m.Status = req.Status
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleTriggerCrawlDue(w http.ResponseWriter, r *http.Request) {
	if _, err := s.asynq.Enqueue(queue.NewCrawlDueTask()); err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enqueued"})
}
