package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/auth"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
)

var dailyCaps = map[domain.EngagementKind]int{
	domain.EngageLike:         25,
	domain.EngageComment:      8,
	domain.EngageFollow:       10,
	domain.EngageShare:        6,
	domain.EngageLinkExchange: 5,
}

var karmaPoints = map[domain.EngagementKind]int{
	domain.EngageLike:         1,
	domain.EngageComment:      3,
	domain.EngageFollow:       1,
	domain.EngageShare:        2,
	domain.EngageLinkExchange: 5,
}

type Engager struct {
	Store    *store.Store
	Registry *platforms.Registry
	EncKey   []byte
	AI       *ai.Client
}

func (e *Engager) Execute(ctx context.Context, taskID uuid.UUID) error {
	task, err := e.Store.GetEngagementTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == domain.EngageDone {
		return nil
	}
	ok, err := e.Store.MarkEngagementExecuting(ctx, taskID)
	if err != nil {
		return err
	}
	if !ok && task.Status != domain.EngageExecuting {
		return nil
	}

	ch, credBlob, err := e.Store.GetChannel(ctx, task.ChannelID)
	if err != nil {
		_ = e.Store.SetEngagementStatus(ctx, taskID, domain.EngageFailed, err.Error(), "")
		return err
	}
	if ch.Health == "paused" {
		err := fmt.Errorf("channel paused")
		_ = e.Store.SetEngagementStatus(ctx, taskID, domain.EngageFailed, err.Error(), "")
		return err
	}

	cap := dailyCaps[task.Kind]
	if cap > 0 {
		n, err := e.Store.CountTodayEngagement(ctx, task.ChannelID, string(task.Kind))
		if err == nil && n >= cap {
			msg := fmt.Sprintf("daily cap reached for %s (%d)", task.Kind, cap)
			_ = e.Store.SetEngagementStatus(ctx, taskID, domain.EngageFailed, msg, "")
			return fmt.Errorf("%s", msg)
		}
	}

	if task.Kind == domain.EngageLinkExchange {
		points := karmaPoints[task.Kind]
		_ = e.Store.InsertKarmaEvent(ctx, task.WorkspaceID, task.ChannelID, task.ID, string(task.Kind), points)
		return e.Store.SetEngagementStatus(ctx, taskID, domain.EngageDone, "", task.TargetURL)
	}

	if ch.PlatformDefID == nil {
		return e.Store.SetEngagementStatus(ctx, taskID, domain.EngageManual, "no platform adapter", "")
	}
	def, err := e.Store.GetPlatformDefinition(ctx, *ch.PlatformDefID)
	if err != nil {
		_ = e.Store.SetEngagementStatus(ctx, taskID, domain.EngageFailed, err.Error(), "")
		return err
	}
	adapter, ok := e.Registry.Get(def.Slug)
	if !ok {
		return e.Store.SetEngagementStatus(ctx, taskID, domain.EngageManual, "no adapter", "")
	}
	engager, ok := platforms.AsEngager(adapter)
	if !ok {
		return e.Store.SetEngagementStatus(ctx, taskID, domain.EngageManual, "adapter has no engage API", "")
	}
	creds, err := auth.DecryptCredentials(e.EncKey, credBlob)
	if err != nil {
		_ = e.Store.SetEngagementStatus(ctx, taskID, domain.EngageFailed, err.Error(), "")
		return err
	}
	if creds.ExternalRef == "" {
		creds.ExternalRef = ch.ExternalRef
	}

	kind := platforms.EngageKind(task.Kind)
	if task.Kind == domain.EngageLinkExchange {
		kind = platforms.EngageComment
	}
	res, err := engager.Engage(ctx, creds, platforms.EngageTarget{
		Kind:       kind,
		ExternalID: task.TargetExternalID,
		URL:        task.TargetURL,
		Text:       task.DraftText,
	})
	if err != nil {
		if strings.Contains(err.Error(), "manual") || strings.Contains(err.Error(), "not supported") {
			_ = e.Store.SetEngagementStatus(ctx, taskID, domain.EngageManual, err.Error(), "")
			return nil
		}
		_ = e.Store.MarkChannelFailure(ctx, ch.ID, err.Error())
		_ = e.Store.SetEngagementStatus(ctx, taskID, domain.EngageFailed, err.Error(), "")
		return err
	}
	_ = e.Store.MarkChannelSuccess(ctx, ch.ID)
	points := karmaPoints[task.Kind]
	_ = e.Store.InsertKarmaEvent(ctx, task.WorkspaceID, task.ChannelID, task.ID, string(task.Kind), points)
	return e.Store.SetEngagementStatus(ctx, taskID, domain.EngageDone, "", res.URL)
}

func (e *Engager) DraftComment(ctx context.Context, workspaceID, taskID uuid.UUID) (string, error) {
	task, err := e.Store.GetEngagementTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task.WorkspaceID != workspaceID {
		return "", fmt.Errorf("task not in workspace")
	}
	agents, err := e.Store.ListAgents(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	var agent *domain.AIAgent
	for i := range agents {
		if agents[i].Enabled && agents[i].Role == domain.AgentContent {
			agent = &agents[i]
			break
		}
	}
	if agent == nil || e.AI == nil {
		return "", fmt.Errorf("no content agent configured")
	}
	ctxMap := map[string]any{
		"kind":   task.Kind,
		"url":    task.TargetURL,
		"title":  task.TargetTitle,
		"goal":   "helpful community comment, not spam, no hard sell",
	}
	if task.BrandID != nil {
		if brand, err := e.Store.GetBrand(ctx, *task.BrandID); err == nil {
			ctxMap["brand"] = map[string]any{
				"name": brand.Name, "tone_of_voice": brand.ToneOfVoice,
				"forbidden_words": brand.ForbiddenWords, "cta_default": brand.CTADefault,
			}
		}
	}
	out, err := e.AI.Run(ctx, *agent, ai.RunInput{
		Prompt:  "Write a short genuine comment for this post. Add value first. At most one soft mention of the product if it truly fits. No hashtag spam. Return only the comment.",
		Context: ctxMap,
	})
	if err != nil {
		return "", err
	}
	if err := e.Store.UpdateEngagementDraft(ctx, task.ID, out.Text); err != nil {
		return "", err
	}
	return out.Text, nil
}
