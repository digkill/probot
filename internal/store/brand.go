package store

import (
	"context"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) CreateBrand(ctx context.Context, b *domain.Brand) error {
	if b.ForbiddenWords == nil {
		b.ForbiddenWords = []string{}
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO brands (workspace_id, name, slug, canonical_url, tone_of_voice, forbidden_words, cta_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, created_at
	`, b.WorkspaceID, b.Name, b.Slug, b.CanonicalURL, b.ToneOfVoice, b.ForbiddenWords, b.CTADefault).
		Scan(&b.ID, &b.CreatedAt)
}

func (s *Store) ListBrands(ctx context.Context, workspaceID uuid.UUID) ([]domain.Brand, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, name, slug, canonical_url, tone_of_voice, forbidden_words, cta_default, created_at
		FROM brands WHERE workspace_id=$1 ORDER BY created_at
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Brand
	for rows.Next() {
		var b domain.Brand
		if err := rows.Scan(&b.ID, &b.WorkspaceID, &b.Name, &b.Slug, &b.CanonicalURL, &b.ToneOfVoice, &b.ForbiddenWords, &b.CTADefault, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetBrand(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	var b domain.Brand
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, slug, canonical_url, tone_of_voice, forbidden_words, cta_default, created_at
		FROM brands WHERE id=$1
	`, id).Scan(&b.ID, &b.WorkspaceID, &b.Name, &b.Slug, &b.CanonicalURL, &b.ToneOfVoice, &b.ForbiddenWords, &b.CTADefault, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
