package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrMentionSuppressed means the link was deleted or marked as a false positive earlier.
var ErrMentionSuppressed = errors.New("This link was deleted or marked as wrong earlier.")

func (s *Store) CreateAgent(ctx context.Context, a *domain.AIAgent) error {
	if a.Params == nil {
		a.Params = json.RawMessage(`{}`)
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO ai_agents (
			workspace_id, name, role, provider, model, base_url, api_key_env, system_prompt, params, enabled
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`, a.WorkspaceID, a.Name, a.Role, a.Provider, a.Model, a.BaseURL, a.APIKeyEnv, a.SystemPrompt, a.Params, a.Enabled).
		Scan(&a.ID)
}

func (s *Store) ListAgents(ctx context.Context, workspaceID uuid.UUID) ([]domain.AIAgent, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, name, role, provider, model, base_url, api_key_env, system_prompt, params, enabled
		FROM ai_agents WHERE workspace_id=$1 ORDER BY created_at
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AIAgent
	for rows.Next() {
		var a domain.AIAgent
		var role string
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Name, &role, &a.Provider, &a.Model, &a.BaseURL, &a.APIKeyEnv, &a.SystemPrompt, &a.Params, &a.Enabled); err != nil {
			return nil, err
		}
		a.Role = domain.AgentRole(role)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAgent(ctx context.Context, id uuid.UUID) (*domain.AIAgent, error) {
	var a domain.AIAgent
	var role string
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, role, provider, model, base_url, api_key_env, system_prompt, params, enabled
		FROM ai_agents WHERE id=$1
	`, id).Scan(&a.ID, &a.WorkspaceID, &a.Name, &role, &a.Provider, &a.Model, &a.BaseURL, &a.APIKeyEnv, &a.SystemPrompt, &a.Params, &a.Enabled)
	if err != nil {
		return nil, err
	}
	a.Role = domain.AgentRole(role)
	return &a, nil
}

func (s *Store) UpdateAgent(ctx context.Context, a *domain.AIAgent) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE ai_agents SET
			name=$2, role=$3, provider=$4, model=$5, base_url=$6, api_key_env=$7,
			system_prompt=$8, params=$9, enabled=$10, updated_at=now()
		WHERE id=$1
	`, a.ID, a.Name, a.Role, a.Provider, a.Model, a.BaseURL, a.APIKeyEnv, a.SystemPrompt, a.Params, a.Enabled)
	return err
}

func (s *Store) DeleteAgent(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM ai_agents WHERE id=$1`, id)
	return err
}

func (s *Store) CreateAIRun(ctx context.Context, r *domain.AIRun) error {
	if r.Input == nil {
		r.Input = json.RawMessage(`{}`)
	}
	if r.Output == nil {
		r.Output = json.RawMessage(`{}`)
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO ai_runs (workspace_id, agent_id, input, output, status)
		VALUES ($1,$2,$3,$4,$5) RETURNING id
	`, r.WorkspaceID, r.AgentID, r.Input, r.Output, r.Status).Scan(&r.ID)
}

func (s *Store) FinishAIRun(ctx context.Context, id uuid.UUID, status string, output json.RawMessage, tokensIn, tokensOut int, errMsg string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE ai_runs
		SET status=$2, output=$3, tokens_in=$4, tokens_out=$5, error_message=$6, finished_at=now()
		WHERE id=$1
	`, id, status, output, tokensIn, tokensOut, errMsg)
	return err
}

func (s *Store) UpsertMention(ctx context.Context, m *domain.Mention, hash string) error {
	if m.Sentiment == "" {
		m.Sentiment = "unknown"
	}
	if m.Severity == "" {
		m.Severity = "none"
	}
	if m.Status == "" {
		m.Status = "new"
	}
	var found any
	if !m.FoundAt.IsZero() {
		found = m.FoundAt
	}
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO mentions (
			workspace_id, brand_id, campaign_id, source, url, title, snippet, author,
			status, hash, found_at, sentiment, severity, watch_query
		)
		SELECT $1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, COALESCE($11::timestamptz, now()), $12, $13, $14
		WHERE NOT EXISTS (
			SELECT 1 FROM mentions
			WHERE workspace_id = $1::uuid AND url = $5
			  AND (deleted_at IS NOT NULL OR status = 'false_positive')
		)
		ON CONFLICT (workspace_id, hash) DO UPDATE SET
			title=EXCLUDED.title,
			snippet=EXCLUDED.snippet,
			sentiment=CASE WHEN EXCLUDED.sentiment='negative' THEN 'negative' ELSE mentions.sentiment END,
			severity=CASE
				WHEN EXCLUDED.severity='high' THEN 'high'
				WHEN EXCLUDED.severity='medium' AND mentions.severity NOT IN ('high') THEN 'medium'
				ELSE mentions.severity
			END,
			brand_id=COALESCE(EXCLUDED.brand_id, mentions.brand_id)
		RETURNING id, found_at
	`, m.WorkspaceID, m.BrandID, m.CampaignID, m.Source, m.URL, m.Title, m.Snippet, m.Author, m.Status, hash, found, m.Sentiment, m.Severity, m.WatchQuery).
		Scan(&m.ID, &m.FoundAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMentionSuppressed
	}
	return err
}

func (s *Store) ListMentions(ctx context.Context, workspaceID uuid.UUID) ([]domain.Mention, error) {
	return s.ListMentionsFiltered(ctx, workspaceID, MentionFilter{})
}

type MentionFilter struct {
	Sentiment string
	Severity  string
	BrandID   *uuid.UUID
	Status    string
	OpenOnly  bool
}

func (s *Store) ListMentionsFiltered(ctx context.Context, workspaceID uuid.UUID, f MentionFilter) ([]domain.Mention, error) {
	q := `
		SELECT id, workspace_id, brand_id, campaign_id, source, url, title, snippet, author,
		       status, found_at, draft_reply, sentiment, severity, watch_query
		FROM mentions WHERE workspace_id=$1 AND deleted_at IS NULL
	`
	args := []any{workspaceID}
	n := 2
	if f.Sentiment != "" {
		q += fmt.Sprintf(" AND sentiment=$%d", n)
		args = append(args, f.Sentiment)
		n++
	}
	if f.Severity != "" {
		q += fmt.Sprintf(" AND severity=$%d", n)
		args = append(args, f.Severity)
		n++
	}
	if f.BrandID != nil {
		q += fmt.Sprintf(" AND brand_id=$%d", n)
		args = append(args, *f.BrandID)
		n++
	}
	if f.Status != "" {
		q += fmt.Sprintf(" AND status=$%d", n)
		args = append(args, f.Status)
		n++
	} else {
		q += " AND status <> 'false_positive'"
	}
	if f.OpenOnly {
		q += " AND status IN ('new','reviewed','escalated')"
	}
	q += " ORDER BY CASE WHEN sentiment='negative' THEN 0 ELSE 1 END, CASE severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 ELSE 3 END, found_at DESC LIMIT 200"
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Mention{}
	for rows.Next() {
		var m domain.Mention
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.BrandID, &m.CampaignID, &m.Source, &m.URL, &m.Title, &m.Snippet, &m.Author, &m.Status, &m.FoundAt, &m.DraftReply, &m.Sentiment, &m.Severity, &m.WatchQuery); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) CreateCrawlSource(ctx context.Context, workspaceID uuid.UUID, kind, url, query string, intervalSec int) (uuid.UUID, error) {
	src := &domain.CrawlSource{
		WorkspaceID: workspaceID,
		Kind:        kind,
		URL:         url,
		Query:       query,
		Purpose:     "mentions",
		IntervalSec: intervalSec,
		Enabled:     true,
	}
	if err := s.InsertCrawlSource(ctx, src); err != nil {
		return uuid.Nil, err
	}
	return src.ID, nil
}

func (s *Store) InsertCrawlSource(ctx context.Context, src *domain.CrawlSource) error {
	if src.IntervalSec <= 0 {
		src.IntervalSec = 3600
	}
	if src.Purpose == "" {
		src.Purpose = "mentions"
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO crawl_sources (workspace_id, brand_id, kind, url, query, interval_sec, purpose, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id
	`, src.WorkspaceID, src.BrandID, src.Kind, src.URL, src.Query, src.IntervalSec, src.Purpose, src.Enabled).Scan(&src.ID)
}

func (s *Store) GetCrawlSource(ctx context.Context, id uuid.UUID) (*domain.CrawlSource, error) {
	var src domain.CrawlSource
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, brand_id, kind, url, query, interval_sec, purpose, enabled, last_run_at
		FROM crawl_sources WHERE id=$1
	`, id).Scan(&src.ID, &src.WorkspaceID, &src.BrandID, &src.Kind, &src.URL, &src.Query, &src.IntervalSec, &src.Purpose, &src.Enabled, &src.LastRunAt)
	if err != nil {
		return nil, err
	}
	return &src, nil
}

func (s *Store) ListCrawlSources(ctx context.Context, workspaceID uuid.UUID) ([]domain.CrawlSource, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, brand_id, kind, url, query, interval_sec, purpose, enabled, last_run_at
		FROM crawl_sources WHERE workspace_id=$1 ORDER BY created_at
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CrawlSource{}
	for rows.Next() {
		var src domain.CrawlSource
		if err := rows.Scan(&src.ID, &src.WorkspaceID, &src.BrandID, &src.Kind, &src.URL, &src.Query, &src.IntervalSec, &src.Purpose, &src.Enabled, &src.LastRunAt); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) ListDueCrawlSources(ctx context.Context, limit int) ([]domain.CrawlSource, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, brand_id, kind, url, query, interval_sec, purpose, enabled, last_run_at
		FROM crawl_sources
		WHERE enabled = TRUE
		  AND (last_run_at IS NULL OR last_run_at + (interval_sec * INTERVAL '1 second') <= now())
		ORDER BY last_run_at NULLS FIRST
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CrawlSource{}
	for rows.Next() {
		var src domain.CrawlSource
		if err := rows.Scan(&src.ID, &src.WorkspaceID, &src.BrandID, &src.Kind, &src.URL, &src.Query, &src.IntervalSec, &src.Purpose, &src.Enabled, &src.LastRunAt); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) TouchCrawlSource(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE crawl_sources SET last_run_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) FindCrawlSource(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID, kind, url, query string) (*domain.CrawlSource, error) {
	var src domain.CrawlSource
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, brand_id, kind, url, query, interval_sec, purpose, enabled, last_run_at
		FROM crawl_sources
		WHERE workspace_id=$1 AND kind=$2 AND url=$3 AND query=$4
		  AND brand_id IS NOT DISTINCT FROM $5
		LIMIT 1
	`, workspaceID, kind, url, query, brandID).
		Scan(&src.ID, &src.WorkspaceID, &src.BrandID, &src.Kind, &src.URL, &src.Query, &src.IntervalSec, &src.Purpose, &src.Enabled, &src.LastRunAt)
	if err != nil {
		return nil, err
	}
	return &src, nil
}

func (s *Store) EnsureCrawlSource(ctx context.Context, src *domain.CrawlSource) (created bool, err error) {
	existing, err := s.FindCrawlSource(ctx, src.WorkspaceID, src.BrandID, src.Kind, src.URL, src.Query)
	if err == nil {
		_, err = s.Pool.Exec(ctx, `
			UPDATE crawl_sources
			SET enabled=TRUE, purpose=$2, interval_sec=$3, query=$4
			WHERE id=$1
		`, existing.ID, src.Purpose, src.IntervalSec, src.Query)
		existing.Enabled = true
		existing.Purpose = src.Purpose
		existing.IntervalSec = src.IntervalSec
		existing.Query = src.Query
		*src = *existing
		return false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if err := s.InsertCrawlSource(ctx, src); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) DisableReviewSources(ctx context.Context, brandID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE crawl_sources SET enabled=FALSE WHERE brand_id=$1 AND purpose='reviews'
	`, brandID)
	return err
}

func (s *Store) SetMentionStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE mentions SET status=$2 WHERE id=$1 AND deleted_at IS NULL`, id, status)
	return err
}

// DeleteMention hides a mention for good; the row stays as a tombstone so crawlers skip its link.
func (s *Store) DeleteMention(ctx context.Context, workspaceID, id uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE mentions SET deleted_at=now(), draft_reply=''
		WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL
	`, id, workspaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteFalsePositiveMentions(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID) (int64, error) {
	q := `
		UPDATE mentions SET deleted_at=now(), draft_reply=''
		WHERE workspace_id=$1 AND status='false_positive' AND deleted_at IS NULL
	`
	args := []any{workspaceID}
	if brandID != nil {
		q += " AND brand_id=$2"
		args = append(args, *brandID)
	}
	tag, err := s.Pool.Exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type MentionInbox struct {
	Total         int `json:"total"`
	Negative      int `json:"negative"`
	OpenNegative  int `json:"open_negative"`
	Escalated     int `json:"escalated"`
	High          int `json:"high"`
	FalsePositive int `json:"false_positive"`
}

func (s *Store) MentionInbox(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID) (MentionInbox, error) {
	q := `
		SELECT
			COUNT(*) FILTER (WHERE status<>'false_positive')::int,
			COUNT(*) FILTER (WHERE sentiment='negative' AND status<>'false_positive')::int,
			COUNT(*) FILTER (WHERE sentiment='negative' AND status IN ('new','reviewed','escalated'))::int,
			COUNT(*) FILTER (WHERE status='escalated')::int,
			COUNT(*) FILTER (WHERE severity='high' AND status<>'false_positive')::int,
			COUNT(*) FILTER (WHERE status='false_positive')::int
		FROM mentions WHERE workspace_id=$1 AND deleted_at IS NULL
	`
	args := []any{workspaceID}
	if brandID != nil {
		q += " AND brand_id=$2"
		args = append(args, *brandID)
	}
	var out MentionInbox
	err := s.Pool.QueryRow(ctx, q, args...).Scan(&out.Total, &out.Negative, &out.OpenNegative, &out.Escalated, &out.High, &out.FalsePositive)
	return out, err
}

func (s *Store) CountMentionsBySentiment(ctx context.Context, workspaceID uuid.UUID) (map[string]int, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT sentiment, COUNT(*) FROM mentions
		WHERE workspace_id=$1 AND deleted_at IS NULL AND status<>'false_positive'
		GROUP BY sentiment
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var sentiment string
		var n int
		if err := rows.Scan(&sentiment, &n); err != nil {
			return nil, err
		}
		out[sentiment] = n
	}
	return out, rows.Err()
}

func (s *Store) AgentNameExists(ctx context.Context, workspaceID uuid.UUID, name string) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_agents WHERE workspace_id=$1 AND name=$2`, workspaceID, name).Scan(&n)
	return n > 0, err
}
