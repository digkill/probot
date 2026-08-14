package store

import (
	"context"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) CreateChannel(ctx context.Context, ch *domain.Channel, credentials []byte) error {
	return s.Pool.QueryRow(ctx, `
		INSERT INTO channels (
			workspace_id, brand_id, platform_def_id, custom_platform_id, name, external_ref, credentials, health
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id
	`, ch.WorkspaceID, ch.BrandID, ch.PlatformDefID, ch.CustomPlatformID, ch.Name, ch.ExternalRef, credentials, ch.Health).
		Scan(&ch.ID)
}

func (s *Store) ListChannels(ctx context.Context, workspaceID uuid.UUID) ([]domain.Channel, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, brand_id, platform_def_id, custom_platform_id, name, external_ref, health
		FROM channels WHERE workspace_id=$1 ORDER BY created_at
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Channel
	for rows.Next() {
		var ch domain.Channel
		if err := rows.Scan(&ch.ID, &ch.WorkspaceID, &ch.BrandID, &ch.PlatformDefID, &ch.CustomPlatformID, &ch.Name, &ch.ExternalRef, &ch.Health); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *Store) GetChannel(ctx context.Context, id uuid.UUID) (*domain.Channel, []byte, error) {
	var ch domain.Channel
	var creds []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, brand_id, platform_def_id, custom_platform_id, name, external_ref, credentials, health
		FROM channels WHERE id=$1
	`, id).Scan(&ch.ID, &ch.WorkspaceID, &ch.BrandID, &ch.PlatformDefID, &ch.CustomPlatformID, &ch.Name, &ch.ExternalRef, &creds, &ch.Health)
	if err != nil {
		return nil, nil, err
	}
	return &ch, creds, nil
}

func (s *Store) UpsertPlatformDefinition(ctx context.Context, p *domain.PlatformDefinition) error {
	return s.Pool.QueryRow(ctx, `
		INSERT INTO platform_definitions (
			slug, name, kind, base_url, publish_mode, char_limit, tags, audience_fit, effort, risk, lang, region, status, meta
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (slug) DO UPDATE SET
			name=EXCLUDED.name,
			kind=EXCLUDED.kind,
			base_url=EXCLUDED.base_url,
			publish_mode=EXCLUDED.publish_mode,
			char_limit=EXCLUDED.char_limit,
			tags=EXCLUDED.tags,
			audience_fit=EXCLUDED.audience_fit,
			effort=EXCLUDED.effort,
			risk=EXCLUDED.risk,
			lang=EXCLUDED.lang,
			region=EXCLUDED.region,
			status=EXCLUDED.status,
			meta=EXCLUDED.meta,
			updated_at=now()
		RETURNING id
	`, p.Slug, p.Name, p.Kind, p.BaseURL, p.PublishMode, p.CharLimit, p.Tags, p.AudienceFit, p.Effort, p.Risk, p.Lang, p.Region, p.Status, p.Meta).
		Scan(&p.ID)
}

func (s *Store) ListPlatformDefinitions(ctx context.Context) ([]domain.PlatformDefinition, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, slug, name, kind, base_url, publish_mode, char_limit, tags, audience_fit, effort, risk, lang, region, status, meta
		FROM platform_definitions
		WHERE status IN ('active', 'pending_review')
		ORDER BY audience_fit DESC, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlatformDefinition
	for rows.Next() {
		var p domain.PlatformDefinition
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Kind, &p.BaseURL, &p.PublishMode, &p.CharLimit, &p.Tags,
			&p.AudienceFit, &p.Effort, &p.Risk, &p.Lang, &p.Region, &p.Status, &p.Meta); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPlatformDefinition(ctx context.Context, id uuid.UUID) (*domain.PlatformDefinition, error) {
	var p domain.PlatformDefinition
	err := s.Pool.QueryRow(ctx, `
		SELECT id, slug, name, kind, base_url, publish_mode, char_limit, tags, audience_fit, effort, risk, lang, region, status, meta
		FROM platform_definitions WHERE id=$1
	`, id).Scan(&p.ID, &p.Slug, &p.Name, &p.Kind, &p.BaseURL, &p.PublishMode, &p.CharLimit, &p.Tags,
		&p.AudienceFit, &p.Effort, &p.Risk, &p.Lang, &p.Region, &p.Status, &p.Meta)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetPlatformDefinitionBySlug(ctx context.Context, slug string) (*domain.PlatformDefinition, error) {
	var p domain.PlatformDefinition
	err := s.Pool.QueryRow(ctx, `
		SELECT id, slug, name, kind, base_url, publish_mode, char_limit, tags, audience_fit, effort, risk, lang, region, status, meta
		FROM platform_definitions WHERE slug=$1
	`, slug).Scan(&p.ID, &p.Slug, &p.Name, &p.Kind, &p.BaseURL, &p.PublishMode, &p.CharLimit, &p.Tags,
		&p.AudienceFit, &p.Effort, &p.Risk, &p.Lang, &p.Region, &p.Status, &p.Meta)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateCustomPlatform(ctx context.Context, p *domain.CustomPlatform) error {
	return s.Pool.QueryRow(ctx, `
		INSERT INTO custom_platforms (workspace_id, name, slug, publish_mode, webhook_url, http_template)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
	`, p.WorkspaceID, p.Name, p.Slug, p.PublishMode, p.WebhookURL, p.HTTPTemplate).Scan(&p.ID)
}

func (s *Store) ListCustomPlatforms(ctx context.Context, workspaceID uuid.UUID) ([]domain.CustomPlatform, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, name, slug, publish_mode, webhook_url, http_template
		FROM custom_platforms WHERE workspace_id=$1 ORDER BY created_at
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CustomPlatform
	for rows.Next() {
		var p domain.CustomPlatform
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Slug, &p.PublishMode, &p.WebhookURL, &p.HTTPTemplate); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetCustomPlatform(ctx context.Context, id uuid.UUID) (*domain.CustomPlatform, error) {
	var p domain.CustomPlatform
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, slug, publish_mode, webhook_url, http_template
		FROM custom_platforms WHERE id=$1
	`, id).Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Slug, &p.PublishMode, &p.WebhookURL, &p.HTTPTemplate)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
