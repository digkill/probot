package store

import (
	"context"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) UpdateBrand(ctx context.Context, b *domain.Brand) error {
	if b.ForbiddenWords == nil {
		b.ForbiddenWords = []string{}
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE brands SET
			name=$2, slug=$3, canonical_url=$4, tone_of_voice=$5,
			forbidden_words=$6, cta_default=$7, updated_at=now()
		WHERE id=$1 AND workspace_id=$8
	`, b.ID, b.Name, b.Slug, b.CanonicalURL, b.ToneOfVoice, b.ForbiddenWords, b.CTADefault, b.WorkspaceID)
	return err
}

func (s *Store) MarkChannelSuccess(ctx context.Context, channelID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE channels
		SET health='ok', fail_count=0, last_error='', last_success_at=now(), updated_at=now()
		WHERE id=$1
	`, channelID)
	return err
}

func (s *Store) MarkChannelFailure(ctx context.Context, channelID uuid.UUID, errMsg string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE channels
		SET
			fail_count = fail_count + 1,
			last_error = $2,
			health = CASE WHEN fail_count + 1 >= 3 THEN 'paused' ELSE 'warn' END,
			updated_at = now()
		WHERE id=$1
	`, channelID, errMsg)
	return err
}

func (s *Store) SetChannelHealth(ctx context.Context, channelID uuid.UUID, health string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE channels SET health=$2, updated_at=now() WHERE id=$1
	`, channelID, health)
	return err
}

func (s *Store) GetMention(ctx context.Context, id uuid.UUID) (*domain.Mention, error) {
	var m domain.Mention
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, brand_id, campaign_id, source, url, title, snippet, author,
		       status, found_at, draft_reply, sentiment, severity, watch_query
		FROM mentions WHERE id=$1
	`, id).Scan(&m.ID, &m.WorkspaceID, &m.BrandID, &m.CampaignID, &m.Source, &m.URL, &m.Title, &m.Snippet, &m.Author, &m.Status, &m.FoundAt, &m.DraftReply, &m.Sentiment, &m.Severity, &m.WatchQuery)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) SetMentionDraft(ctx context.Context, id uuid.UUID, draft, status string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE mentions SET draft_reply=$2, status=$3 WHERE id=$1
	`, id, draft, status)
	return err
}
