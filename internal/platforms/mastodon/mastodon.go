package mastodon

import (
	"bytes"
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

// Adapter posts to a Mastodon-compatible instance.
// ExternalRef or Meta["instance"] = https://mastodon.social
type Adapter struct {
	client *http.Client
	id     string
}

func New() *Adapter {
	return &Adapter{client: &http.Client{Timeout: 30 * time.Second}, id: "mastodon"}
}

func NewLemmy() *Adapter {
	// Lemmy is not API-compatible with Mastodon statuses; keep slug for catalog
	// and fail clearly until a dedicated adapter exists. Still register as mastodon
	// for fediverse instances that speak Mastodon API under custom slug via Meta.
	return &Adapter{client: &http.Client{Timeout: 30 * time.Second}, id: "mastodon"}
}

func (a *Adapter) ID() string { return a.id }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	instance := strings.TrimRight(firstNonEmpty(auth.ExternalRef, auth.Meta["instance"]), "/")
	if instance == "" || auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("mastodon: instance URL and access token required")
	}
	if !strings.HasPrefix(instance, "http") {
		instance = "https://" + instance
	}
	var acct struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		URL      string `json:"url"`
	}
	if err := a.do(ctx, http.MethodGet, instance+"/api/v1/accounts/verify_credentials", auth.Token, nil, &acct); err != nil {
		return platforms.Credentials{}, err
	}
	return platforms.Credentials{
		AccessToken: auth.Token,
		ExternalRef: instance,
		Meta: map[string]string{
			"instance": instance,
			"username": acct.Username,
			"user_id":  acct.ID,
			"profile":  acct.URL,
		},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	instance := strings.TrimRight(firstNonEmpty(creds.ExternalRef, creds.Meta["instance"]), "/")
	if instance == "" || creds.AccessToken == "" {
		return platforms.PublishResult{}, fmt.Errorf("mastodon: instance and token required")
	}
	text := post.Text
	if post.CTAURL != "" && !strings.Contains(text, post.CTAURL) {
		text = strings.TrimSpace(text + "\n\n" + post.CTAURL)
	}
	form := url.Values{}
	form.Set("status", text)
	form.Set("visibility", "public")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, instance+"/api/v1/statuses", strings.NewReader(form.Encode()))
	if err != nil {
		return platforms.PublishResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return platforms.PublishResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return platforms.PublishResult{}, fmt.Errorf("mastodon status: %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return platforms.PublishResult{}, err
	}
	return platforms.PublishResult{ExternalID: out.ID, URL: out.URL}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	instance := strings.TrimRight(firstNonEmpty(creds.ExternalRef, creds.Meta["instance"]), "/")
	if instance == "" || creds.AccessToken == "" || externalID == "" {
		return platforms.PlatformStats{}, nil
	}
	var out struct {
		ReblogsCount    int64 `json:"reblogs_count"`
		FavouritesCount int64 `json:"favourites_count"`
		RepliesCount    int64 `json:"replies_count"`
	}
	if err := a.do(ctx, http.MethodGet, instance+"/api/v1/statuses/"+externalID, creds.AccessToken, nil, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	return platforms.PlatformStats{
		Likes:    out.FavouritesCount,
		Comments: out.RepliesCount,
		Shares:   out.ReblogsCount,
	}, nil
}

func (a *Adapter) do(ctx context.Context, method, rawURL, token string, payload any, dest any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
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
		return fmt.Errorf("mastodon %s: %d: %s", rawURL, resp.StatusCode, string(raw))
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(raw, dest)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
