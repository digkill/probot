package catalog

import (
	"context"
	"encoding/json"

	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/store"
)

func SeedPlatforms(ctx context.Context, st *store.Store) error {
	limit := func(n int) *int { return &n }
	items := []domain.PlatformDefinition{
		{Slug: "telegram", Name: "Telegram", Kind: "builtin", BaseURL: "https://telegram.org", PublishMode: "api", CharLimit: limit(4096), Tags: []string{"chat", "channels"}, AudienceFit: 0.9, Effort: "low", Risk: "low", Lang: "multi", Region: "global", Status: "active", Meta: json.RawMessage(`{}`)},
		{Slug: "vk", Name: "VK", Kind: "builtin", BaseURL: "https://vk.com", PublishMode: "api", CharLimit: limit(15895), Tags: []string{"social", "ru"}, AudienceFit: 0.85, Effort: "low", Risk: "medium", Lang: "ru", Region: "ru", Status: "active", Meta: json.RawMessage(`{}`)},
		{Slug: "bluesky", Name: "Bluesky", Kind: "atproto", BaseURL: "https://bsky.app", PublishMode: "api", CharLimit: limit(300), Tags: []string{"social", "atproto", "niche"}, AudienceFit: 0.7, Effort: "low", Risk: "low", Lang: "en", Region: "global", Status: "active", Meta: json.RawMessage(`{}`)},
		{Slug: "x", Name: "X (Twitter)", Kind: "builtin", BaseURL: "https://x.com", PublishMode: "api", CharLimit: limit(280), Tags: []string{"social"}, AudienceFit: 0.8, Effort: "high", Risk: "high", Lang: "multi", Region: "global", Status: "active", Meta: json.RawMessage(`{"auth":"oauth2_user_token"}`)},
		{Slug: "reddit", Name: "Reddit", Kind: "builtin", BaseURL: "https://reddit.com", PublishMode: "api", CharLimit: limit(40000), Tags: []string{"communities"}, AudienceFit: 0.75, Effort: "medium", Risk: "medium", Lang: "en", Region: "global", Status: "active", Meta: json.RawMessage(`{"auth":"oauth_access_token","external_ref":"subreddit"}`)},
		{Slug: "mastodon", Name: "Mastodon", Kind: "mastodon_like", BaseURL: "https://joinmastodon.org", PublishMode: "api", CharLimit: limit(500), Tags: []string{"fediverse", "niche"}, AudienceFit: 0.55, Effort: "medium", Risk: "low", Lang: "multi", Region: "global", Status: "active", Meta: json.RawMessage(`{"external_ref":"instance_url"}`)},
		{Slug: "lemmy", Name: "Lemmy", Kind: "mastodon_like", BaseURL: "https://join-lemmy.org", PublishMode: "api", Tags: []string{"fediverse", "niche"}, AudienceFit: 0.4, Effort: "medium", Risk: "low", Lang: "en", Region: "global", Status: "active", Meta: json.RawMessage(`{}`)},
		{Slug: "vc", Name: "VC.ru", Kind: "manual", BaseURL: "https://vc.ru", PublishMode: "manual", Tags: []string{"ru", "media"}, AudienceFit: 0.7, Effort: "high", Risk: "medium", Lang: "ru", Region: "ru", Status: "active", Meta: json.RawMessage(`{}`)},
		{Slug: "habr", Name: "Habr", Kind: "manual", BaseURL: "https://habr.com", PublishMode: "manual", Tags: []string{"ru", "tech"}, AudienceFit: 0.65, Effort: "high", Risk: "medium", Lang: "ru", Region: "ru", Status: "active", Meta: json.RawMessage(`{}`)},
		{Slug: "dzen", Name: "Дзен", Kind: "manual", BaseURL: "https://dzen.ru", PublishMode: "manual", Tags: []string{"ru", "media"}, AudienceFit: 0.6, Effort: "high", Risk: "medium", Lang: "ru", Region: "ru", Status: "active", Meta: json.RawMessage(`{}`)},
	}
	for i := range items {
		if err := st.UpsertPlatformDefinition(ctx, &items[i]); err != nil {
			return err
		}
	}
	return nil
}
