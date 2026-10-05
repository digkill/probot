package threads

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/platforms/metagraph"
)

const textLimit = 500

// Adapter publishes to Threads via the Threads API. Token: Threads user
// access token (threads_basic, threads_content_publish).
type Adapter struct {
	graph    *metagraph.Client
	pollStep time.Duration
}

func New() *Adapter {
	return &Adapter{graph: metagraph.New("threads", "https://graph.threads.net/v1.0"), pollStep: 3 * time.Second}
}

func (a *Adapter) ID() string { return "threads" }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	if auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("threads: access token required")
	}
	var me struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := a.graph.Get(ctx, "/me", auth.Token, url.Values{"fields": {"id,username"}}, &me); err != nil {
		return platforms.Credentials{}, err
	}
	return platforms.Credentials{
		AccessToken: auth.Token,
		ExternalRef: me.ID,
		Meta:        map[string]string{"username": me.Username},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	if creds.AccessToken == "" || creds.ExternalRef == "" {
		return platforms.PublishResult{}, fmt.Errorf("threads: user id and token required")
	}
	user := "/" + url.PathEscape(creds.ExternalRef)
	form := url.Values{"text": {metagraph.ComposeText(post.Text, post.CTAURL, textLimit)}}
	needsProcessing := false
	switch {
	case len(post.MediaURLs) > 0 && metagraph.IsVideoURL(post.MediaURLs[0]):
		form.Set("media_type", "VIDEO")
		form.Set("video_url", post.MediaURLs[0])
		needsProcessing = true
	case len(post.MediaURLs) > 0:
		form.Set("media_type", "IMAGE")
		form.Set("image_url", post.MediaURLs[0])
		needsProcessing = true
	default:
		form.Set("media_type", "TEXT")
	}
	var container struct {
		ID string `json:"id"`
	}
	if err := a.graph.Post(ctx, user+"/threads", creds.AccessToken, form, &container); err != nil {
		return platforms.PublishResult{}, err
	}
	if needsProcessing {
		err := metagraph.WaitReady(ctx, a.pollStep, 5*time.Minute, func() (bool, error) {
			var st struct {
				Status       string `json:"status"`
				ErrorMessage string `json:"error_message"`
			}
			if err := a.graph.Get(ctx, "/"+container.ID, creds.AccessToken, url.Values{"fields": {"status,error_message"}}, &st); err != nil {
				return false, err
			}
			switch st.Status {
			case "ERROR", "EXPIRED":
				return false, fmt.Errorf("threads: media processing failed (%s %s)", st.Status, st.ErrorMessage)
			case "IN_PROGRESS":
				return false, nil
			default:
				return true, nil
			}
		})
		if err != nil {
			return platforms.PublishResult{}, err
		}
	}
	var published struct {
		ID string `json:"id"`
	}
	if err := a.graph.Post(ctx, user+"/threads_publish", creds.AccessToken, url.Values{"creation_id": {container.ID}}, &published); err != nil {
		return platforms.PublishResult{}, err
	}
	var link struct {
		Permalink string `json:"permalink"`
	}
	_ = a.graph.Get(ctx, "/"+published.ID, creds.AccessToken, url.Values{"fields": {"permalink"}}, &link)
	return platforms.PublishResult{ExternalID: published.ID, URL: link.Permalink}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	if creds.AccessToken == "" || externalID == "" {
		return platforms.PlatformStats{}, nil
	}
	var out struct {
		Data []struct {
			Name   string `json:"name"`
			Values []struct {
				Value int64 `json:"value"`
			} `json:"values"`
			TotalValue *struct {
				Value int64 `json:"value"`
			} `json:"total_value"`
		} `json:"data"`
	}
	if err := a.graph.Get(ctx, "/"+url.PathEscape(externalID)+"/insights", creds.AccessToken, url.Values{"metric": {"views,likes,replies,reposts"}}, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	var st platforms.PlatformStats
	for _, m := range out.Data {
		var v int64
		if m.TotalValue != nil {
			v = m.TotalValue.Value
		} else if len(m.Values) > 0 {
			v = m.Values[0].Value
		}
		switch m.Name {
		case "views":
			st.Reach = v
		case "likes":
			st.Likes = v
		case "replies":
			st.Comments = v
		case "reposts":
			st.Shares = v
		}
	}
	return st, nil
}
