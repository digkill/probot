package instagram

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/platforms/metagraph"
)

const captionLimit = 2200

// Adapter publishes to an Instagram professional account via the Graph API.
// Token: access token with instagram_content_publish. Tokens issued by
// Instagram Login (prefix "IG") go to graph.instagram.com, others to
// graph.facebook.com. ExternalRef = Instagram account ID.
// Instagram has no text-only posts: every post needs an image or video URL.
type Adapter struct {
	fb       *metagraph.Client
	ig       *metagraph.Client
	pollStep time.Duration
}

func New() *Adapter {
	return &Adapter{
		fb:       metagraph.New("instagram", "https://graph.facebook.com/v23.0"),
		ig:       metagraph.New("instagram", "https://graph.instagram.com/v23.0"),
		pollStep: 3 * time.Second,
	}
}

func (a *Adapter) ID() string { return "instagram" }

func (a *Adapter) client(token string) *metagraph.Client {
	if strings.HasPrefix(token, "IG") {
		return a.ig
	}
	return a.fb
}

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	if auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("instagram: access token required")
	}
	g := a.client(auth.Token)
	igID := strings.TrimSpace(auth.ExternalRef)
	if igID == "" {
		if g != a.ig {
			return platforms.Credentials{}, fmt.Errorf("instagram: Instagram account ID (external_ref) required")
		}
		var me struct {
			UserID string `json:"user_id"`
			ID     string `json:"id"`
		}
		if err := g.Get(ctx, "/me", auth.Token, url.Values{"fields": {"user_id,username"}}, &me); err != nil {
			return platforms.Credentials{}, err
		}
		igID = me.UserID
		if igID == "" {
			igID = me.ID
		}
	}
	var acc struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := g.Get(ctx, "/"+url.PathEscape(igID), auth.Token, url.Values{"fields": {"id,username"}}, &acc); err != nil {
		return platforms.Credentials{}, err
	}
	return platforms.Credentials{
		AccessToken: auth.Token,
		ExternalRef: igID,
		Meta:        map[string]string{"username": acc.Username},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	if creds.AccessToken == "" || creds.ExternalRef == "" {
		return platforms.PublishResult{}, fmt.Errorf("instagram: account id and token required")
	}
	if len(post.MediaURLs) == 0 {
		return platforms.PublishResult{}, fmt.Errorf("instagram: a post needs an image or video (add a media URL to the content)")
	}
	g := a.client(creds.AccessToken)
	user := "/" + url.PathEscape(creds.ExternalRef)
	media := post.MediaURLs[0]
	form := url.Values{"caption": {metagraph.ComposeText(post.Text, post.CTAURL, captionLimit)}}
	timeout := 30 * time.Second
	if metagraph.IsVideoURL(media) {
		form.Set("media_type", "REELS")
		form.Set("video_url", media)
		timeout = 5 * time.Minute
	} else {
		form.Set("image_url", media)
	}
	var container struct {
		ID string `json:"id"`
	}
	if err := g.Post(ctx, user+"/media", creds.AccessToken, form, &container); err != nil {
		return platforms.PublishResult{}, err
	}
	err := metagraph.WaitReady(ctx, a.pollStep, timeout, func() (bool, error) {
		var st struct {
			StatusCode string `json:"status_code"`
		}
		if err := g.Get(ctx, "/"+container.ID, creds.AccessToken, url.Values{"fields": {"status_code"}}, &st); err != nil {
			return false, err
		}
		switch st.StatusCode {
		case "ERROR", "EXPIRED":
			return false, fmt.Errorf("instagram: media processing failed (%s)", st.StatusCode)
		case "IN_PROGRESS":
			return false, nil
		default:
			return true, nil
		}
	})
	if err != nil {
		return platforms.PublishResult{}, err
	}
	var published struct {
		ID string `json:"id"`
	}
	if err := g.Post(ctx, user+"/media_publish", creds.AccessToken, url.Values{"creation_id": {container.ID}}, &published); err != nil {
		return platforms.PublishResult{}, err
	}
	var link struct {
		Permalink string `json:"permalink"`
	}
	_ = g.Get(ctx, "/"+published.ID, creds.AccessToken, url.Values{"fields": {"permalink"}}, &link)
	return platforms.PublishResult{ExternalID: published.ID, URL: link.Permalink}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	if creds.AccessToken == "" || externalID == "" {
		return platforms.PlatformStats{}, nil
	}
	var out struct {
		LikeCount     int64 `json:"like_count"`
		CommentsCount int64 `json:"comments_count"`
	}
	if err := a.client(creds.AccessToken).Get(ctx, "/"+url.PathEscape(externalID), creds.AccessToken, url.Values{"fields": {"like_count,comments_count"}}, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	return platforms.PlatformStats{Likes: out.LikeCount, Comments: out.CommentsCount}, nil
}
