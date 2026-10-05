package facebook

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/platforms/metagraph"
)

// Adapter publishes to a Facebook Page. Token = Page access token
// (pages_manage_posts); ExternalRef = Page ID (optional, taken from the token).
type Adapter struct {
	graph *metagraph.Client
}

func New() *Adapter {
	return &Adapter{graph: metagraph.New("facebook", "https://graph.facebook.com/v23.0")}
}

func (a *Adapter) ID() string { return "facebook" }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	if auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("facebook: page access token required")
	}
	var me struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := a.graph.Get(ctx, "/me", auth.Token, url.Values{"fields": {"id,name"}}, &me); err != nil {
		return platforms.Credentials{}, err
	}
	pageID := strings.TrimSpace(auth.ExternalRef)
	name := me.Name
	if pageID == "" {
		pageID = me.ID
	} else if pageID != me.ID {
		var page struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := a.graph.Get(ctx, "/"+url.PathEscape(pageID), auth.Token, url.Values{"fields": {"id,name"}}, &page); err != nil {
			return platforms.Credentials{}, err
		}
		name = page.Name
	}
	return platforms.Credentials{
		AccessToken: auth.Token,
		ExternalRef: pageID,
		Meta:        map[string]string{"page_name": name},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	if creds.AccessToken == "" || creds.ExternalRef == "" {
		return platforms.PublishResult{}, fmt.Errorf("facebook: page id and token required")
	}
	page := "/" + url.PathEscape(creds.ExternalRef)
	var out struct {
		ID     string `json:"id"`
		PostID string `json:"post_id"`
	}
	if len(post.MediaURLs) > 0 && !metagraph.IsVideoURL(post.MediaURLs[0]) {
		form := url.Values{
			"url":     {post.MediaURLs[0]},
			"caption": {metagraph.ComposeText(post.Text, post.CTAURL, 63000)},
		}
		if err := a.graph.Post(ctx, page+"/photos", creds.AccessToken, form, &out); err != nil {
			return platforms.PublishResult{}, err
		}
	} else {
		form := url.Values{"message": {metagraph.TruncateRunes(strings.TrimSpace(post.Text), 63000)}}
		if post.CTAURL != "" {
			form.Set("link", post.CTAURL)
		}
		if err := a.graph.Post(ctx, page+"/feed", creds.AccessToken, form, &out); err != nil {
			return platforms.PublishResult{}, err
		}
	}
	id := out.PostID
	if id == "" {
		id = out.ID
	}
	if id == "" {
		return platforms.PublishResult{}, fmt.Errorf("facebook: empty post id in response")
	}
	return platforms.PublishResult{ExternalID: id, URL: "https://www.facebook.com/" + id}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	if creds.AccessToken == "" || externalID == "" {
		return platforms.PlatformStats{}, nil
	}
	var out struct {
		Reactions struct {
			Summary struct {
				TotalCount int64 `json:"total_count"`
			} `json:"summary"`
		} `json:"reactions"`
		Comments struct {
			Summary struct {
				TotalCount int64 `json:"total_count"`
			} `json:"summary"`
		} `json:"comments"`
		Shares struct {
			Count int64 `json:"count"`
		} `json:"shares"`
	}
	fields := "reactions.summary(total_count).limit(0),comments.summary(total_count).limit(0),shares"
	if err := a.graph.Get(ctx, "/"+url.PathEscape(externalID), creds.AccessToken, url.Values{"fields": {fields}}, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	return platforms.PlatformStats{
		Likes:    out.Reactions.Summary.TotalCount,
		Comments: out.Comments.Summary.TotalCount,
		Shares:   out.Shares.Count,
	}, nil
}
