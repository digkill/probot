package store

import (
	"context"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) CreateCampaign(ctx context.Context, c *domain.Campaign) error {
	return s.Pool.QueryRow(ctx, `
		INSERT INTO campaigns (workspace_id, brand_id, name, playbook, status)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id
	`, c.WorkspaceID, c.BrandID, c.Name, c.Playbook, c.Status).Scan(&c.ID)
}

func (s *Store) ListCampaigns(ctx context.Context, workspaceID uuid.UUID) ([]domain.Campaign, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, brand_id, name, playbook, status
		FROM campaigns WHERE workspace_id=$1 ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Campaign
	for rows.Next() {
		var c domain.Campaign
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.BrandID, &c.Name, &c.Playbook, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateContent(ctx context.Context, c *domain.ContentPiece) error {
	if c.MediaURLs == nil {
		c.MediaURLs = []string{}
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO content_pieces (
			workspace_id, brand_id, campaign_id, title, body, cta_url, media_urls,
			utm_source, utm_medium, utm_campaign, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id
	`, c.WorkspaceID, c.BrandID, c.CampaignID, c.Title, c.Body, c.CTAURL, c.MediaURLs,
		c.UTMSource, c.UTMMedium, c.UTMCampaign, c.Status).Scan(&c.ID)
}

func (s *Store) GetContent(ctx context.Context, id uuid.UUID) (*domain.ContentPiece, error) {
	var c domain.ContentPiece
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, brand_id, campaign_id, title, body, cta_url, media_urls,
		       utm_source, utm_medium, utm_campaign, status
		FROM content_pieces WHERE id=$1
	`, id).Scan(&c.ID, &c.WorkspaceID, &c.BrandID, &c.CampaignID, &c.Title, &c.Body, &c.CTAURL, &c.MediaURLs,
		&c.UTMSource, &c.UTMMedium, &c.UTMCampaign, &c.Status)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListContent(ctx context.Context, workspaceID uuid.UUID) ([]domain.ContentPiece, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, brand_id, campaign_id, title, body, cta_url, media_urls,
		       utm_source, utm_medium, utm_campaign, status
		FROM content_pieces WHERE workspace_id=$1 ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ContentPiece
	for rows.Next() {
		var c domain.ContentPiece
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.BrandID, &c.CampaignID, &c.Title, &c.Body, &c.CTAURL, &c.MediaURLs,
			&c.UTMSource, &c.UTMMedium, &c.UTMCampaign, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreatePublication(ctx context.Context, p *domain.Publication) error {
	return s.Pool.QueryRow(ctx, `
		INSERT INTO publications (
			workspace_id, content_id, channel_id, campaign_id, status, body_override,
			scheduled_at, idempotency_key, sort_order
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id
	`, p.WorkspaceID, p.ContentID, p.ChannelID, p.CampaignID, p.Status, p.BodyOverride,
		p.ScheduledAt, p.IdempotencyKey, p.SortOrder).Scan(&p.ID)
}

func (s *Store) GetPublication(ctx context.Context, id uuid.UUID) (*domain.Publication, error) {
	var p domain.Publication
	var status string
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, content_id, channel_id, campaign_id, status, body_override,
		       scheduled_at, published_at, external_id, external_url, idempotency_key, error_message, sort_order
		FROM publications WHERE id=$1
	`, id).Scan(&p.ID, &p.WorkspaceID, &p.ContentID, &p.ChannelID, &p.CampaignID, &status, &p.BodyOverride,
		&p.ScheduledAt, &p.PublishedAt, &p.ExternalID, &p.ExternalURL, &p.IdempotencyKey, &p.ErrorMessage, &p.SortOrder)
	if err != nil {
		return nil, err
	}
	p.Status = domain.PublicationStatus(status)
	return &p, nil
}

func (s *Store) ListPublications(ctx context.Context, workspaceID uuid.UUID) ([]domain.Publication, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, content_id, channel_id, campaign_id, status, body_override,
		       scheduled_at, published_at, external_id, external_url, idempotency_key, error_message, sort_order
		FROM publications WHERE workspace_id=$1 ORDER BY sort_order, created_at
	`, workspaceID)
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

func (s *Store) ListDuePublications(ctx context.Context, now time.Time, limit int) ([]domain.Publication, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, content_id, channel_id, campaign_id, status, body_override,
		       scheduled_at, published_at, external_id, external_url, idempotency_key, error_message, sort_order
		FROM publications
		WHERE status='scheduled' AND (scheduled_at IS NULL OR scheduled_at <= $1)
		ORDER BY sort_order, scheduled_at NULLS FIRST
		LIMIT $2
	`, now, limit)
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

func (s *Store) MarkPublicationPublishing(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE publications SET status='publishing', updated_at=now()
		WHERE id=$1 AND status IN ('scheduled', 'draft')
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) MarkPublicationLive(ctx context.Context, id uuid.UUID, externalID, externalURL string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE publications
		SET status='live', external_id=$2, external_url=$3, published_at=now(), error_message='', updated_at=now()
		WHERE id=$1
	`, id, externalID, externalURL)
	return err
}

func (s *Store) MarkPublicationFailed(ctx context.Context, id uuid.UUID, msg string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE publications SET status='failed', error_message=$2, updated_at=now() WHERE id=$1
	`, id, msg)
	return err
}

func (s *Store) MarkPublicationManual(ctx context.Context, id uuid.UUID, hintURL string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE publications SET status='needs_manual_confirm', external_url=$2, updated_at=now() WHERE id=$1
	`, id, hintURL)
	return err
}

func (s *Store) ConfirmManualPublication(ctx context.Context, id uuid.UUID, externalURL string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE publications
		SET status='live', external_url=$2, published_at=now(), updated_at=now()
		WHERE id=$1 AND status='needs_manual_confirm'
	`, id, externalURL)
	return err
}

func (s *Store) CreateCrossLink(ctx context.Context, l *domain.CrossLink) error {
	return s.Pool.QueryRow(ctx, `
		INSERT INTO cross_links (workspace_id, campaign_id, from_publication_id, to_publication_id, to_url, kind)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
	`, l.WorkspaceID, l.CampaignID, l.FromPublicationID, l.ToPublicationID, l.ToURL, l.Kind).Scan(&l.ID)
}

func (s *Store) ListCrossLinksByCampaign(ctx context.Context, campaignID uuid.UUID) ([]domain.CrossLink, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, campaign_id, from_publication_id, to_publication_id, to_url, kind
		FROM cross_links WHERE campaign_id=$1
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CrossLink
	for rows.Next() {
		var l domain.CrossLink
		if err := rows.Scan(&l.ID, &l.WorkspaceID, &l.CampaignID, &l.FromPublicationID, &l.ToPublicationID, &l.ToURL, &l.Kind); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) ListPublicationsByCampaign(ctx context.Context, campaignID uuid.UUID) ([]domain.Publication, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, content_id, channel_id, campaign_id, status, body_override,
		       scheduled_at, published_at, external_id, external_url, idempotency_key, error_message, sort_order
		FROM publications WHERE campaign_id=$1 ORDER BY sort_order, created_at
	`, campaignID)
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
