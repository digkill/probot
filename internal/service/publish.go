package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/digkill/probot/internal/auth"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/platforms/generic"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
)

type Publisher struct {
	Store         *store.Store
	Registry      *platforms.Registry
	Generic       *generic.Publisher
	EncKey        []byte
	PublicBaseURL string
}

func (p *Publisher) PublishPublication(ctx context.Context, publicationID uuid.UUID) error {
	pub, err := p.Store.GetPublication(ctx, publicationID)
	if err != nil {
		return err
	}
	if pub.Status == domain.PubLive {
		return nil
	}
	ok, err := p.Store.MarkPublicationPublishing(ctx, publicationID)
	if err != nil {
		return err
	}
	if !ok && pub.Status != domain.PubPublishing {
		return nil
	}

	content, err := p.Store.GetContent(ctx, pub.ContentID)
	if err != nil {
		_ = p.Store.MarkPublicationFailed(ctx, publicationID, err.Error())
		return err
	}
	ch, credBlob, err := p.Store.GetChannel(ctx, pub.ChannelID)
	if err != nil {
		_ = p.Store.MarkPublicationFailed(ctx, publicationID, err.Error())
		return err
	}
	if ch.Health == "paused" {
		err := fmt.Errorf("channel paused due to repeated failures")
		_ = p.Store.MarkPublicationFailed(ctx, publicationID, err.Error())
		return err
	}

	body := content.Body
	if pub.BodyOverride != "" {
		body = pub.BodyOverride
	}
	cta := withUTM(content.CTAURL, content.UTMSource, content.UTMMedium, content.UTMCampaign)
	cta = p.shortenCTA(ctx, pub, content, cta)

	// Inject previous live URLs from campaign as cross-links (via short links when possible).
	if pub.CampaignID != nil {
		prev, _ := p.Store.ListPublicationsByCampaign(ctx, *pub.CampaignID)
		var links []string
		for _, pr := range prev {
			if pr.ID == pub.ID || pr.ExternalURL == "" || pr.Status != domain.PubLive {
				continue
			}
			if pr.SortOrder < pub.SortOrder {
				links = append(links, p.maybeShortURL(ctx, pub, content, pr.ExternalURL, "cross:"+pr.ID.String()))
			}
		}
		if len(links) > 0 {
			body = strings.TrimSpace(body + "\n\nAlso: " + strings.Join(links, " | "))
			for _, u := range links {
				fromID := pub.ID
				link := &domain.CrossLink{
					WorkspaceID:       pub.WorkspaceID,
					CampaignID:        pub.CampaignID,
					FromPublicationID: &fromID,
					ToURL:             u,
					Kind:              "cross_post",
				}
				_ = p.Store.CreateCrossLink(ctx, link)
			}
		}
		if brand, err := p.Store.GetBrand(ctx, content.BrandID); err == nil && brand.CanonicalURL != "" {
			canon := withUTM(brand.CanonicalURL, content.UTMSource, content.UTMMedium, content.UTMCampaign)
			canon = p.maybeShortURL(ctx, pub, content, canon, "canonical")
			if !strings.Contains(body, canon) {
				body = strings.TrimSpace(body + "\n\n" + canon)
			}
			fromID := pub.ID
			_ = p.Store.CreateCrossLink(ctx, &domain.CrossLink{
				WorkspaceID:       pub.WorkspaceID,
				CampaignID:        pub.CampaignID,
				FromPublicationID: &fromID,
				ToURL:             canon,
				Kind:              "canonical",
			})
		}
	}

	post := platforms.NormalizedPost{
		Text:      body,
		MediaURLs: content.MediaURLs,
		CTAURL:    cta,
	}

	if ch.CustomPlatformID != nil {
		cp, err := p.Store.GetCustomPlatform(ctx, *ch.CustomPlatformID)
		if err != nil {
			_ = p.Store.MarkPublicationFailed(ctx, publicationID, err.Error())
			return err
		}
		switch cp.PublishMode {
		case "webhook":
			res, err := p.Generic.PublishWebhook(ctx, generic.WebhookConfig{URL: cp.WebhookURL}, post)
			if err != nil {
				return p.failPub(ctx, publicationID, ch.ID, err)
			}
			_ = p.Store.MarkChannelSuccess(ctx, ch.ID)
			return p.Store.MarkPublicationLive(ctx, publicationID, res.ExternalID, res.URL)
		default:
			_ = p.Generic.PrepareManual(post)
			return p.Store.MarkPublicationManual(ctx, publicationID, "")
		}
	}

	if ch.PlatformDefID == nil {
		return p.failPub(ctx, publicationID, ch.ID, fmt.Errorf("channel has no platform"))
	}
	def, err := p.Store.GetPlatformDefinition(ctx, *ch.PlatformDefID)
	if err != nil {
		return p.failPub(ctx, publicationID, ch.ID, err)
	}
	if def.PublishMode == "manual" || def.PublishMode == "crawl_only" {
		return p.Store.MarkPublicationManual(ctx, publicationID, "")
	}

	adapter, ok := p.Registry.Get(def.Slug)
	if !ok {
		return p.failPub(ctx, publicationID, ch.ID, fmt.Errorf("no adapter for platform %s", def.Slug))
	}
	creds, err := auth.DecryptCredentials(p.EncKey, credBlob)
	if err != nil {
		return p.failPub(ctx, publicationID, ch.ID, err)
	}
	if creds.ExternalRef == "" {
		creds.ExternalRef = ch.ExternalRef
	}
	res, err := adapter.Publish(ctx, creds, post)
	if err != nil {
		return p.failPub(ctx, publicationID, ch.ID, err)
	}
	_ = p.Store.MarkChannelSuccess(ctx, ch.ID)
	return p.Store.MarkPublicationLive(ctx, publicationID, res.ExternalID, res.URL)
}

func (p *Publisher) failPub(ctx context.Context, publicationID, channelID uuid.UUID, err error) error {
	_ = p.Store.MarkPublicationFailed(ctx, publicationID, err.Error())
	_ = p.Store.MarkChannelFailure(ctx, channelID, err.Error())
	return err
}

func withUTM(raw, source, medium, campaign string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if source != "" {
		q.Set("utm_source", source)
	}
	if medium != "" {
		q.Set("utm_medium", medium)
	}
	if campaign != "" {
		q.Set("utm_campaign", campaign)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *Publisher) shortenCTA(ctx context.Context, pub *domain.Publication, content *domain.ContentPiece, cta string) string {
	return p.maybeShortURL(ctx, pub, content, cta, "cta")
}

func (p *Publisher) maybeShortURL(ctx context.Context, pub *domain.Publication, content *domain.ContentPiece, target, label string) string {
	if target == "" || p.PublicBaseURL == "" || p.Store == nil {
		return target
	}
	link := &store.ShortLink{
		WorkspaceID:   pub.WorkspaceID,
		TargetURL:     target,
		PublicationID: &pub.ID,
		CampaignID:    pub.CampaignID,
		BrandID:       &content.BrandID,
		Label:         label,
	}
	if err := p.Store.CreateShortLink(ctx, link); err != nil {
		return target
	}
	return strings.TrimRight(p.PublicBaseURL, "/") + "/r/" + link.Code
}
