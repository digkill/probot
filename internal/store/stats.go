package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

type MetricSnapshot struct {
	ID            uuid.UUID       `json:"id"`
	PublicationID uuid.UUID       `json:"publication_id"`
	Reach         int64           `json:"reach"`
	Likes         int64           `json:"likes"`
	Comments      int64           `json:"comments"`
	Shares        int64           `json:"shares"`
	Clicks        int64           `json:"clicks"`
	Raw           json.RawMessage `json:"raw"`
	CapturedAt    time.Time       `json:"captured_at"`
}

func (s *Store) ListLivePublications(ctx context.Context, limit int) ([]domain.Publication, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, content_id, channel_id, campaign_id, status, body_override,
		       scheduled_at, published_at, external_id, external_url, idempotency_key, error_message, sort_order
		FROM publications
		WHERE status='live' AND external_id <> ''
		ORDER BY published_at DESC NULLS LAST
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Publication
	for rows.Next() {
		var p domain.Publication
		var status string
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.ContentID, &p.ChannelID, &p.CampaignID, &status, &p.BodyOverride,
			&p.ScheduledAt, &p.PublishedAt, &p.ExternalID, &p.ExternalURL, &p.IdempotencyKey, &p.ErrorMessage, &p.SortOrder); err != nil {
			return nil, err
		}
		p.Status = domain.PublicationStatus(status)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) InsertMetricSnapshot(ctx context.Context, m *MetricSnapshot) error {
	if m.Raw == nil {
		m.Raw = json.RawMessage(`{}`)
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO metric_snapshots (publication_id, reach, likes, comments, shares, clicks, raw)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, captured_at
	`, m.PublicationID, m.Reach, m.Likes, m.Comments, m.Shares, m.Clicks, m.Raw).
		Scan(&m.ID, &m.CapturedAt)
}

func (s *Store) LatestMetricsByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]MetricSnapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT ON (ms.publication_id)
			ms.id, ms.publication_id, ms.reach, ms.likes, ms.comments, ms.shares, ms.clicks, ms.raw, ms.captured_at
		FROM metric_snapshots ms
		JOIN publications p ON p.id = ms.publication_id
		WHERE p.workspace_id = $1
		ORDER BY ms.publication_id, ms.captured_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetricSnapshot
	for rows.Next() {
		var m MetricSnapshot
		if err := rows.Scan(&m.ID, &m.PublicationID, &m.Reach, &m.Likes, &m.Comments, &m.Shares, &m.Clicks, &m.Raw, &m.CapturedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SumShortLinkClicks(ctx context.Context, workspaceID uuid.UUID) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(clicks),0) FROM short_links WHERE workspace_id=$1`, workspaceID).Scan(&n)
	return n, err
}

func (s *Store) CountByPublicationStatus(ctx context.Context, workspaceID uuid.UUID) (map[string]int, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT status, COUNT(*) FROM publications WHERE workspace_id=$1 GROUP BY status
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}
