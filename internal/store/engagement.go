package store

import (
	"context"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) CreateEngagementTask(ctx context.Context, t *domain.EngagementTask) error {
	if t.Status == "" {
		t.Status = domain.EngagePending
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO engagement_tasks (
			workspace_id, brand_id, channel_id, partner_id, mention_id, kind, status,
			target_url, target_external_id, target_title, draft_text, points, scheduled_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at
	`, t.WorkspaceID, t.BrandID, t.ChannelID, t.PartnerID, t.MentionID, t.Kind, t.Status,
		t.TargetURL, t.TargetExternalID, t.TargetTitle, t.DraftText, t.Points, t.ScheduledAt).
		Scan(&t.ID, &t.CreatedAt)
}

func (s *Store) GetEngagementTask(ctx context.Context, id uuid.UUID) (*domain.EngagementTask, error) {
	var t domain.EngagementTask
	var kind, status string
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, brand_id, channel_id, partner_id, mention_id, kind, status,
		       target_url, target_external_id, target_title, draft_text, result_url, points,
		       error_message, scheduled_at, executed_at, created_at
		FROM engagement_tasks WHERE id=$1
	`, id).Scan(&t.ID, &t.WorkspaceID, &t.BrandID, &t.ChannelID, &t.PartnerID, &t.MentionID,
		&kind, &status, &t.TargetURL, &t.TargetExternalID, &t.TargetTitle, &t.DraftText, &t.ResultURL,
		&t.Points, &t.ErrorMessage, &t.ScheduledAt, &t.ExecutedAt, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.Kind = domain.EngagementKind(kind)
	t.Status = domain.EngagementStatus(status)
	return &t, nil
}

func (s *Store) ListEngagementTasks(ctx context.Context, workspaceID uuid.UUID, status string) ([]domain.EngagementTask, error) {
	q := `
		SELECT id, workspace_id, brand_id, channel_id, partner_id, mention_id, kind, status,
		       target_url, target_external_id, target_title, draft_text, result_url, points,
		       error_message, scheduled_at, executed_at, created_at
		FROM engagement_tasks WHERE workspace_id=$1
	`
	args := []any{workspaceID}
	if status != "" {
		q += ` AND status=$2`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.EngagementTask{}
	for rows.Next() {
		var t domain.EngagementTask
		var kind, st string
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.BrandID, &t.ChannelID, &t.PartnerID, &t.MentionID,
			&kind, &st, &t.TargetURL, &t.TargetExternalID, &t.TargetTitle, &t.DraftText, &t.ResultURL,
			&t.Points, &t.ErrorMessage, &t.ScheduledAt, &t.ExecutedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Kind = domain.EngagementKind(kind)
		t.Status = domain.EngagementStatus(st)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) UpdateEngagementDraft(ctx context.Context, id uuid.UUID, draft string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE engagement_tasks SET draft_text=$2, status='pending_approval', updated_at=now() WHERE id=$1
	`, id, draft)
	return err
}

func (s *Store) SetEngagementStatus(ctx context.Context, id uuid.UUID, status domain.EngagementStatus, errMsg, resultURL string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE engagement_tasks
		SET status=$2, error_message=$3, result_url=CASE WHEN $4='' THEN result_url ELSE $4 END,
		    executed_at=CASE WHEN $2 IN ('done','failed','needs_manual') THEN now() ELSE executed_at END,
		    updated_at=now()
		WHERE id=$1
	`, id, status, errMsg, resultURL)
	return err
}

func (s *Store) MarkEngagementExecuting(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE engagement_tasks SET status='executing', updated_at=now()
		WHERE id=$1 AND status='approved'
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) CountTodayEngagement(ctx context.Context, channelID uuid.UUID, kind string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM engagement_tasks
		WHERE channel_id=$1 AND kind=$2
		  AND status IN ('done','executing','approved')
		  AND created_at >= date_trunc('day', now())
	`, channelID, kind).Scan(&n)
	return n, err
}

func (s *Store) InsertKarmaEvent(ctx context.Context, workspaceID, channelID, taskID uuid.UUID, kind string, points int) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO karma_events (workspace_id, channel_id, task_id, kind, points)
		VALUES ($1,$2,$3,$4,$5)
	`, workspaceID, channelID, taskID, kind, points)
	return err
}

func (s *Store) KarmaSummary(ctx context.Context, workspaceID uuid.UUID) (*domain.KarmaSummary, error) {
	sum := &domain.KarmaSummary{
		ByKind:          map[string]int{},
		TodayByKind:     map[string]int{},
		TodayTaskCounts: map[string]int{},
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT kind, SUM(points), SUM(CASE WHEN created_at >= date_trunc('day', now()) THEN points ELSE 0 END)
		FROM karma_events WHERE workspace_id=$1 GROUP BY kind
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var total, today int
		if err := rows.Scan(&kind, &total, &today); err != nil {
			return nil, err
		}
		sum.ByKind[kind] = total
		sum.TodayByKind[kind] = today
		sum.TotalPoints += total
		sum.TodayPoints += today
	}
	rows2, err := s.Pool.Query(ctx, `
		SELECT kind, COUNT(*) FROM engagement_tasks
		WHERE workspace_id=$1 AND status='done' AND created_at >= date_trunc('day', now())
		GROUP BY kind
	`, workspaceID)
	if err != nil {
		return sum, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var kind string
		var n int
		if err := rows2.Scan(&kind, &n); err != nil {
			return nil, err
		}
		sum.TodayTaskCounts[kind] = n
	}
	return sum, rows2.Err()
}

func (s *Store) CreateLinkPartner(ctx context.Context, p *domain.LinkPartner) error {
	if p.Status == "" {
		p.Status = "outreach"
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO link_partners (workspace_id, brand_id, name, platform, their_url, our_url, contact, status, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at
	`, p.WorkspaceID, p.BrandID, p.Name, p.Platform, p.TheirURL, p.OurURL, p.Contact, p.Status, p.Notes).
		Scan(&p.ID, &p.CreatedAt)
}

func (s *Store) ListLinkPartners(ctx context.Context, workspaceID uuid.UUID) ([]domain.LinkPartner, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, workspace_id, brand_id, name, platform, their_url, our_url, contact, status, notes, created_at
		FROM link_partners WHERE workspace_id=$1 ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LinkPartner
	for rows.Next() {
		var p domain.LinkPartner
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.BrandID, &p.Name, &p.Platform, &p.TheirURL, &p.OurURL, &p.Contact, &p.Status, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLinkPartner(ctx context.Context, p *domain.LinkPartner) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE link_partners SET
			name=$2, platform=$3, their_url=$4, our_url=$5, contact=$6, status=$7, notes=$8, updated_at=now()
		WHERE id=$1 AND workspace_id=$9
	`, p.ID, p.Name, p.Platform, p.TheirURL, p.OurURL, p.Contact, p.Status, p.Notes, p.WorkspaceID)
	return err
}

func (s *Store) GetLinkPartner(ctx context.Context, id uuid.UUID) (*domain.LinkPartner, error) {
	var p domain.LinkPartner
	err := s.Pool.QueryRow(ctx, `
		SELECT id, workspace_id, brand_id, name, platform, their_url, our_url, contact, status, notes, created_at
		FROM link_partners WHERE id=$1
	`, id).Scan(&p.ID, &p.WorkspaceID, &p.BrandID, &p.Name, &p.Platform, &p.TheirURL, &p.OurURL, &p.Contact, &p.Status, &p.Notes, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
