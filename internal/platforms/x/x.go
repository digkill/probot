package x

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/digkill/probot/internal/platforms"
)

// Adapter uses X API v2 with a user OAuth2 access token (Bearer).
type Adapter struct {
	client  *http.Client
	baseURL string
}

func New() *Adapter {
	return &Adapter{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://api.x.com/2",
	}
}

func (a *Adapter) ID() string { return "x" }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	if auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("x: access token required")
	}
	var out struct {
		Data struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"data"`
		Detail string `json:"detail"`
		Title  string `json:"title"`
	}
	if err := a.do(ctx, http.MethodGet, "/users/me", auth.Token, nil, &out); err != nil {
		return platforms.Credentials{}, err
	}
	if out.Data.ID == "" {
		msg := out.Detail
		if msg == "" {
			msg = out.Title
		}
		if msg == "" {
			msg = "users/me failed"
		}
		return platforms.Credentials{}, fmt.Errorf("x: %s", msg)
	}
	return platforms.Credentials{
		AccessToken:  auth.Token,
		RefreshToken: auth.RefreshToken,
		ExternalRef:  out.Data.Username,
		Meta: map[string]string{
			"user_id":  out.Data.ID,
			"username": out.Data.Username,
			"name":     out.Data.Name,
		},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	if creds.AccessToken == "" {
		return platforms.PublishResult{}, fmt.Errorf("x: access token required")
	}
	text := post.Text
	if post.CTAURL != "" && !strings.Contains(text, post.CTAURL) {
		text = strings.TrimSpace(text + "\n\n" + post.CTAURL)
	}
	text = truncateRunes(text, 280)

	payload := map[string]any{"text": text}
	if post.ReplyTo != "" {
		payload["reply"] = map[string]string{"in_reply_to_tweet_id": post.ReplyTo}
	}

	var out struct {
		Data struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"data"`
		Detail string `json:"detail"`
		Title  string `json:"title"`
	}
	if err := a.do(ctx, http.MethodPost, "/tweets", creds.AccessToken, payload, &out); err != nil {
		return platforms.PublishResult{}, err
	}
	if out.Data.ID == "" {
		msg := out.Detail
		if msg == "" {
			msg = out.Title
		}
		if msg == "" {
			msg = "tweet create failed"
		}
		return platforms.PublishResult{}, fmt.Errorf("x: %s", msg)
	}
	username := creds.Meta["username"]
	if username == "" {
		username = creds.ExternalRef
	}
	tweetURL := fmt.Sprintf("https://x.com/i/web/status/%s", out.Data.ID)
	if username != "" {
		tweetURL = fmt.Sprintf("https://x.com/%s/status/%s", username, out.Data.ID)
	}
	return platforms.PublishResult{
		ExternalID: out.Data.ID,
		URL:        tweetURL,
	}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	if creds.AccessToken == "" || externalID == "" {
		return platforms.PlatformStats{}, nil
	}
	path := fmt.Sprintf("/tweets/%s?tweet.fields=public_metrics", externalID)
	var out struct {
		Data struct {
			PublicMetrics struct {
				LikeCount    int64 `json:"like_count"`
				ReplyCount   int64 `json:"reply_count"`
				RetweetCount int64 `json:"retweet_count"`
				QuoteCount   int64 `json:"quote_count"`
				Impression   int64 `json:"impression_count"`
			} `json:"public_metrics"`
		} `json:"data"`
	}
	if err := a.do(ctx, http.MethodGet, path, creds.AccessToken, nil, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	m := out.Data.PublicMetrics
	return platforms.PlatformStats{
		Reach:    m.Impression,
		Likes:    m.LikeCount,
		Comments: m.ReplyCount,
		Shares:   m.RetweetCount + m.QuoteCount,
	}, nil
}

func (a *Adapter) do(ctx context.Context, method, path, token string, payload any, dest any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("x %s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if dest == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dest)
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
