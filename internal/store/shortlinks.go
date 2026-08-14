package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

type ShortLink struct {
	ID            uuid.UUID  `json:"id"`
	WorkspaceID   uuid.UUID  `json:"workspace_id"`
	Code          string     `json:"code"`
	TargetURL     string     `json:"target_url"`
	PublicationID *uuid.UUID `json:"publication_id,omitempty"`
	CampaignID    *uuid.UUID `json:"campaign_id,omitempty"`
	BrandID       *uuid.UUID `json:"brand_id,omitempty"`
	Label         string     `json:"label"`
	Clicks        int64      `json:"clicks"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (s *Store) CreateShortLink(ctx context.Context, link *ShortLink) error {
	if link.Code == "" {
		code, err := randomCode(8)
		if err != nil {
			return err
		}
		link.Code = code
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO short_links (workspace_id, code, target_url, publication_id, campaign_id, brand_id, label)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, clicks, created_at
	`, link.WorkspaceID, link.Code, link.TargetURL, link.PublicationID, link.CampaignID, link.BrandID, link.Label).
		Scan(&link.ID, &link.Clicks, &link.CreatedAt)
}

func (s *Store) GetShortLinkByCode(ctx context.Context, code string) (*ShortLink, error) {
	var l ShortLink
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, code, target_url, publication_id, campaign_id, brand_id, label, clicks, created_at
		FROM short_links WHERE code=$1
	`, code).Scan(&l.ID, &l.WorkspaceID, &l.Code, &l.TargetURL, &l.PublicationID, &l.CampaignID, &l.BrandID, &l.Label, &l.Clicks, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Store) ListShortLinks(ctx context.Context, workspaceID uuid.UUID) ([]ShortLink, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, code, target_url, publication_id, campaign_id, brand_id, label, clicks, created_at
		FROM short_links WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 200
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShortLink
	for rows.Next() {
		var l ShortLink
		if err := rows.Scan(&l.ID, &l.WorkspaceID, &l.Code, &l.TargetURL, &l.PublicationID, &l.CampaignID, &l.BrandID, &l.Label, &l.Clicks, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) RecordShortLinkClick(ctx context.Context, linkID uuid.UUID, referer, userAgent, ipHash string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO short_link_clicks (short_link_id, referer, user_agent, ip_hash)
		VALUES ($1,$2,$3,$4)
	`, linkID, referer, userAgent, ipHash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE short_links SET clicks = clicks + 1 WHERE id=$1`, linkID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func randomCode(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
