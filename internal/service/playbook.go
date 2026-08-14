package service

import (
	"context"
	"fmt"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/playbook"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/digkill/probot/internal/queue"
)

type PlaybookService struct {
	Store *store.Store
	Asynq *asynq.Client
}

type ApplyPlaybookInput struct {
	CampaignID uuid.UUID
	ContentID  uuid.UUID
	PlaybookID string
	Enqueue    bool
	BaseTime   time.Time
}

type ApplyPlaybookResult struct {
	PlaybookID    string                `json:"playbook_id"`
	Publications  []domain.Publication  `json:"publications"`
	Skipped       []string              `json:"skipped"`
	MissingRequired []string            `json:"missing_required"`
}

func (s *PlaybookService) Apply(ctx context.Context, workspaceID uuid.UUID, in ApplyPlaybookInput) (*ApplyPlaybookResult, error) {
	pb, ok := playbook.Get(in.PlaybookID)
	if !ok {
		return nil, fmt.Errorf("unknown playbook %s", in.PlaybookID)
	}
	content, err := s.Store.GetContent(ctx, in.ContentID)
	if err != nil {
		return nil, err
	}
	if content.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("content not in workspace")
	}
	channels, err := s.Store.ListChannels(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// Map platform slug -> channel for this brand
	bySlug := map[string]domain.Channel{}
	for _, ch := range channels {
		if ch.BrandID != content.BrandID {
			continue
		}
		if ch.PlatformDefID == nil {
			continue
		}
		def, err := s.Store.GetPlatformDefinition(ctx, *ch.PlatformDefID)
		if err != nil {
			continue
		}
		bySlug[def.Slug] = ch
	}

	base := in.BaseTime
	if base.IsZero() {
		base = time.Now()
	}
	result := &ApplyPlaybookResult{PlaybookID: pb.ID}

	// Update campaign playbook field
	_, _ = s.Store.Pool.Exec(ctx, `UPDATE campaigns SET playbook=$2, updated_at=now() WHERE id=$1`, in.CampaignID, pb.ID)

	for i, step := range pb.Steps {
		ch, found := bySlug[step.PlatformSlug]
		if !found {
			result.Skipped = append(result.Skipped, step.PlatformSlug)
			if step.Required {
				result.MissingRequired = append(result.MissingRequired, step.PlatformSlug)
			}
			continue
		}
		at := playbook.ScheduleAt(base, step.DelayMinutes)
		pub := &domain.Publication{
			WorkspaceID:    workspaceID,
			ContentID:      in.ContentID,
			ChannelID:      ch.ID,
			CampaignID:     &in.CampaignID,
			Status:         domain.PubScheduled,
			ScheduledAt:    &at,
			SortOrder:      i,
			IdempotencyKey: uuid.NewString(),
		}
		if err := s.Store.CreatePublication(ctx, pub); err != nil {
			return nil, err
		}
		result.Publications = append(result.Publications, *pub)
		if in.Enqueue {
			task, err := queue.NewPublishTask(pub.ID)
			if err != nil {
				return nil, err
			}
			opts := []asynq.Option{asynq.Queue("default"), asynq.MaxRetry(5)}
			if step.DelayMinutes > 0 {
				opts = append(opts, asynq.ProcessAt(at))
			}
			if _, err := s.Asynq.Enqueue(task, opts...); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
