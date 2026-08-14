package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/crawler"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/queue"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleUpdateBrand(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "brandID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid brand id")
		return
	}
	existing, err := s.store.GetBrand(r.Context(), id)
	if err != nil || existing.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "brand not found")
		return
	}
	var req domain.Brand
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.ID = id
	req.WorkspaceID = existing.WorkspaceID
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Slug == "" {
		req.Slug = existing.Slug
	}
	if err := s.store.UpdateBrand(r.Context(), &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != existing.Name || req.Slug != existing.Slug || req.CanonicalURL != existing.CanonicalURL {
		_, _, _, _ = s.seedReviewWatch(r.Context(), req, true)
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	ws := mustWorkspaceID(r)
	metrics, err := s.store.LatestMetricsByWorkspace(r.Context(), ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	clicks, _ := s.store.SumShortLinkClicks(r.Context(), ws)
	statuses, _ := s.store.CountByPublicationStatus(r.Context(), ws)
	mentions, _ := s.store.ListMentions(r.Context(), ws)
	inbox, _ := s.store.MentionInbox(r.Context(), ws, nil)
	channels, _ := s.store.ListChannels(r.Context(), ws)

	var reach, likes, comments, shares int64
	for _, m := range metrics {
		reach += m.Reach
		likes += m.Likes
		comments += m.Comments
		shares += m.Shares
	}
	health := map[string]int{}
	for _, ch := range channels {
		health[ch.Health]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"totals": map[string]any{
			"reach": reach, "likes": likes, "comments": comments, "shares": shares,
			"short_link_clicks": clicks,
			"mentions":          len(mentions),
			"negative_mentions": inbox.Negative,
			"open_negative":     inbox.OpenNegative,
			"escalated_mentions": inbox.Escalated,
		},
		"mention_inbox":      inbox,
		"publication_status": statuses,
		"channel_health":     health,
		"latest_metrics":     metrics,
		"recent_mentions":    truncateMentions(mentions, 10),
	})
}

func truncateMentions(in []domain.Mention, n int) []domain.Mention {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func (s *Server) handleAnalyticsAdvise(w http.ResponseWriter, r *http.Request) {
	ws := mustWorkspaceID(r)
	var req struct {
		AgentID    *uuid.UUID `json:"agent_id"`
		BrandID    *uuid.UUID `json:"brand_id"`
		CampaignID *uuid.UUID `json:"campaign_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	agents, err := s.store.ListAgents(r.Context(), ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var agent *domain.AIAgent
	for i := range agents {
		a := &agents[i]
		if req.AgentID != nil && a.ID == *req.AgentID {
			agent = a
			break
		}
		if agent == nil && a.Role == domain.AgentAnalyticsAdvisor && a.Enabled {
			agent = a
		}
	}
	if agent == nil {
		writeErr(w, http.StatusBadRequest, "no analytics_advisor agent configured")
		return
	}

	summary := map[string]any{}
	// reuse summary builder via internal call pattern
	metrics, _ := s.store.LatestMetricsByWorkspace(r.Context(), ws)
	clicks, _ := s.store.SumShortLinkClicks(r.Context(), ws)
	statuses, _ := s.store.CountByPublicationStatus(r.Context(), ws)
	mentions, _ := s.store.ListMentions(r.Context(), ws)
	summary["metrics"] = metrics
	summary["short_link_clicks"] = clicks
	summary["publication_status"] = statuses
	summary["mention_count"] = len(mentions)
	inbox, _ := s.store.MentionInbox(r.Context(), ws, nil)
	summary["mention_inbox"] = inbox
	if req.CampaignID != nil {
		g, err := s.graph.CampaignGraph(r.Context(), *req.CampaignID)
		if err == nil {
			summary["graph"] = g
		}
	}

	input := ai.RunInput{
		Prompt:  "Analyze this marketing/PR snapshot and suggest the next amplification moves. Be concrete: which platform, what to post, which weak edge to reinforce.",
		Context: summary,
	}
	if req.BrandID != nil {
		if brand, err := s.store.GetBrand(r.Context(), *req.BrandID); err == nil {
			input.Context["brand"] = brandContextMap(brand)
		}
	}

	inJSON, _ := json.Marshal(input)
	run := &domain.AIRun{WorkspaceID: ws, AgentID: agent.ID, Input: inJSON, Status: "running"}
	_ = s.store.CreateAIRun(r.Context(), run)
	out, err := s.ai.Run(r.Context(), *agent, input)
	if err != nil {
		_ = s.store.FinishAIRun(r.Context(), run.ID, "failed", json.RawMessage(`{}`), 0, 0, err.Error())
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	outJSON, _ := json.Marshal(out)
	_ = s.store.FinishAIRun(r.Context(), run.ID, "done", outJSON, out.TokensIn, out.TokensOut, "")
	writeJSON(w, http.StatusOK, map[string]any{"result": out, "agent_id": agent.ID})
}

func brandContextMap(b *domain.Brand) map[string]any {
	return map[string]any{
		"name":            b.Name,
		"slug":            b.Slug,
		"canonical_url":   b.CanonicalURL,
		"tone_of_voice":   b.ToneOfVoice,
		"forbidden_words": b.ForbiddenWords,
		"cta_default":     b.CTADefault,
	}
}

func (s *Server) handleMentionDraftReply(w http.ResponseWriter, r *http.Request) {
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

	agent, err := s.pickAgent(r, req.AgentID, domain.AgentContent)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	brand := s.mentionBrand(r, m, req.BrandID)
	text := crawler.DraftObjection(*m, brand)
	via := "template"
	if agent != nil {
		ctxMap := map[string]any{
			"mention": map[string]any{
				"source": m.Source, "url": m.URL, "title": m.Title,
				"snippet": m.Snippet, "author": m.Author,
			},
		}
		if brand != nil {
			ctxMap["brand"] = brandContextMap(brand)
		}
		input := ai.RunInput{
			Prompt:  "Write a short, friendly public reply to this mention. Stay on-brand. No hashtag spam. Return only the reply text.",
			Context: ctxMap,
		}
		if out, err := s.ai.Run(r.Context(), *agent, input); err == nil && out.Text != "" {
			text = out.Text
			via = "ai"
		}
	}
	_ = s.store.SetMentionDraft(r.Context(), m.ID, text, "reviewed")
	m.DraftReply = text
	m.Status = "reviewed"
	writeJSON(w, http.StatusOK, map[string]any{"mention": m, "draft_reply": text, "via": via})
}

func (s *Server) handleSetChannelHealth(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid channel id")
		return
	}
	var req struct {
		Health string `json:"health"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Health != "ok" && req.Health != "warn" && req.Health != "paused" {
		writeErr(w, http.StatusBadRequest, "health must be ok|warn|paused")
		return
	}
	ch, _, err := s.store.GetChannel(r.Context(), id)
	if err != nil || ch.WorkspaceID != mustWorkspaceID(r) {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}
	if err := s.store.SetChannelHealth(r.Context(), id, req.Health); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "health": req.Health})
}

func (s *Server) handleTriggerStatsPoll(w http.ResponseWriter, r *http.Request) {
	if _, err := s.asynq.Enqueue(queue.NewPollAllStatsTask()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enqueued"})
}
