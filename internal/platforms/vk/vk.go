package vk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/digkill/probot/internal/platforms"
)

type Adapter struct {
	client  *http.Client
	apiVer  string
	baseURL string
}

func New() *Adapter {
	return &Adapter{
		client:  &http.Client{Timeout: 30 * time.Second},
		apiVer:  "5.199",
		baseURL: "https://api.vk.com/method",
	}
}

func (a *Adapter) ID() string { return "vk" }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	if auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("vk: access token required")
	}
	return platforms.Credentials{
		AccessToken: auth.Token,
		ExternalRef: auth.ExternalRef,
		Meta:        auth.Meta,
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	ownerID := creds.ExternalRef
	if ownerID == "" {
		return platforms.PublishResult{}, fmt.Errorf("vk: owner_id/external_ref required")
	}
	message := post.Text
	if post.CTAURL != "" && !strings.Contains(message, post.CTAURL) {
		message = strings.TrimSpace(message + "\n\n" + post.CTAURL)
	}
	q := url.Values{}
	q.Set("access_token", creds.AccessToken)
	q.Set("v", a.apiVer)
	q.Set("owner_id", ownerID)
	q.Set("from_group", "1")
	q.Set("message", message)

	var out struct {
		Response struct {
			PostID int64 `json:"post_id"`
		} `json:"response"`
		Error *struct {
			ErrorCode int    `json:"error_code"`
			ErrorMsg  string `json:"error_msg"`
		} `json:"error"`
	}
	if err := a.call(ctx, "wall.post", q, &out); err != nil {
		return platforms.PublishResult{}, err
	}
	if out.Error != nil {
		return platforms.PublishResult{}, fmt.Errorf("vk wall.post: %s", out.Error.ErrorMsg)
	}
	postURL := fmt.Sprintf("https://vk.com/wall%s_%d", ownerID, out.Response.PostID)
	return platforms.PublishResult{
		ExternalID: fmt.Sprintf("%s_%d", ownerID, out.Response.PostID),
		URL:        postURL,
	}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	q := url.Values{}
	q.Set("access_token", creds.AccessToken)
	q.Set("v", a.apiVer)
	q.Set("posts", externalID)
	var out struct {
		Response []struct {
			Views struct {
				Count int64 `json:"count"`
			} `json:"views"`
			Likes struct {
				Count int64 `json:"count"`
			} `json:"likes"`
			Comments struct {
				Count int64 `json:"count"`
			} `json:"comments"`
			Reposts struct {
				Count int64 `json:"count"`
			} `json:"reposts"`
		} `json:"response"`
		Error *struct {
			ErrorMsg string `json:"error_msg"`
		} `json:"error"`
	}
	if err := a.call(ctx, "wall.getById", q, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	if out.Error != nil {
		return platforms.PlatformStats{}, fmt.Errorf("vk wall.getById: %s", out.Error.ErrorMsg)
	}
	if len(out.Response) == 0 {
		return platforms.PlatformStats{}, nil
	}
	p := out.Response[0]
	return platforms.PlatformStats{
		Reach:    p.Views.Count,
		Likes:    p.Likes.Count,
		Comments: p.Comments.Count,
		Shares:   p.Reposts.Count,
	}, nil
}

func (a *Adapter) call(ctx context.Context, method string, q url.Values, dest any) error {
	u := fmt.Sprintf("%s/%s?%s", a.baseURL, method, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("vk %s: status %d: %s", method, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, dest)
}
