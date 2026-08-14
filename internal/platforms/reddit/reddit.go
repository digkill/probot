package reddit

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

// Adapter posts to a subreddit using a Reddit OAuth access token.
// ExternalRef = subreddit name (without r/).
// Meta may include client_id for token refresh later; Connect accepts ready access_token.
type Adapter struct {
	client *http.Client
}

func New() *Adapter {
	return &Adapter{client: &http.Client{Timeout: 45 * time.Second}}
}

func (a *Adapter) ID() string { return "reddit" }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	token := auth.Token
	if token == "" {
		return platforms.Credentials{}, fmt.Errorf("reddit: access token required")
	}
	// Optional: exchange app credentials if username/password provided in meta.
	if auth.Meta["client_id"] != "" && auth.Meta["client_secret"] != "" && auth.Meta["username"] != "" && auth.Meta["password"] != "" {
		t, err := a.passwordGrant(ctx, auth.Meta["client_id"], auth.Meta["client_secret"], auth.Meta["username"], auth.Meta["password"])
		if err != nil {
			return platforms.Credentials{}, err
		}
		token = t
	}
	var me struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := a.getJSON(ctx, token, "https://oauth.reddit.com/api/v1/me", &me); err != nil {
		return platforms.Credentials{}, err
	}
	ref := auth.ExternalRef
	if ref == "" {
		ref = me.Name
	}
	return platforms.Credentials{
		AccessToken: token,
		ExternalRef: strings.TrimPrefix(ref, "r/"),
		Meta: map[string]string{
			"username": me.Name,
			"user_id":  me.ID,
		},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	sub := strings.TrimPrefix(creds.ExternalRef, "r/")
	if sub == "" {
		return platforms.PublishResult{}, fmt.Errorf("reddit: subreddit (external_ref) required")
	}
	if creds.AccessToken == "" {
		return platforms.PublishResult{}, fmt.Errorf("reddit: access token required")
	}
	title := post.Meta["title"]
	body := post.Text
	if title == "" {
		title, body = splitTitleBody(post.Text)
	}
	if post.CTAURL != "" && !strings.Contains(body, post.CTAURL) {
		body = strings.TrimSpace(body + "\n\n" + post.CTAURL)
	}
	form := url.Values{}
	form.Set("sr", sub)
	form.Set("kind", "self")
	form.Set("title", title)
	form.Set("text", body)
	form.Set("api_type", "json")
	form.Set("resubmit", "true")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth.reddit.com/api/submit", strings.NewReader(form.Encode()))
	if err != nil {
		return platforms.PublishResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("User-Agent", "PRobot/0.1 by digkill")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return platforms.PublishResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return platforms.PublishResult{}, fmt.Errorf("reddit submit: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		JSON struct {
			Errors [][]any `json:"errors"`
			Data   struct {
				URL    string `json:"url"`
				Name   string `json:"name"`
				ID     string `json:"id"`
			} `json:"data"`
		} `json:"json"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return platforms.PublishResult{}, fmt.Errorf("reddit submit decode: %w (%s)", err, string(raw))
	}
	if len(out.JSON.Errors) > 0 {
		return platforms.PublishResult{}, fmt.Errorf("reddit submit: %v", out.JSON.Errors)
	}
	return platforms.PublishResult{
		ExternalID: out.JSON.Data.Name,
		URL:        out.JSON.Data.URL,
	}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	if creds.AccessToken == "" || externalID == "" {
		return platforms.PlatformStats{}, nil
	}
	u := "https://oauth.reddit.com/api/info?id=" + url.QueryEscape(externalID)
	var out struct {
		Data struct {
			Children []struct {
				Data struct {
					Score    int64 `json:"score"`
					Ups      int64 `json:"ups"`
					NumComments int64 `json:"num_comments"`
					ViewCount int64 `json:"view_count"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := a.getJSON(ctx, creds.AccessToken, u, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	if len(out.Data.Children) == 0 {
		return platforms.PlatformStats{}, nil
	}
	d := out.Data.Children[0].Data
	return platforms.PlatformStats{
		Reach:    d.ViewCount,
		Likes:    d.Ups,
		Comments: d.NumComments,
		Shares:   0,
	}, nil
}

func (a *Adapter) passwordGrant(ctx context.Context, clientID, clientSecret, username, password string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", username)
	form.Set("password", password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.reddit.com/api/v1/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("User-Agent", "PRobot/0.1 by digkill")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("reddit token: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("reddit token: %s", out.Error)
	}
	return out.AccessToken, nil
}

func (a *Adapter) getJSON(ctx context.Context, token, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "PRobot/0.1 by digkill")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("reddit GET %s: status %d: %s", rawURL, resp.StatusCode, string(raw))
	}
	return json.Unmarshal(raw, dest)
}

func splitTitleBody(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "Update", ""
	}
	parts := strings.SplitN(text, "\n", 2)
	title := strings.TrimSpace(parts[0])
	if len([]rune(title)) > 300 {
		title = string([]rune(title)[:297]) + "..."
	}
	body := ""
	if len(parts) > 1 {
		body = strings.TrimSpace(parts[1])
	}
	return title, body
}
