package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/digkill/probot/internal/platforms"
)

type Adapter struct {
	client  *http.Client
	baseURL string
}

func New() *Adapter {
	return &Adapter{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://bsky.social/xrpc",
	}
}

func (a *Adapter) ID() string { return "bluesky" }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	identifier := auth.ExternalRef
	if identifier == "" {
		identifier = auth.Meta["handle"]
	}
	if identifier == "" || auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("bluesky: handle and app password required")
	}
	payload := map[string]string{
		"identifier": identifier,
		"password":   auth.Token,
	}
	var out struct {
		AccessJwt string `json:"accessJwt"`
		Did       string `json:"did"`
		Handle    string `json:"handle"`
	}
	if err := a.postJSON(ctx, "", "com.atproto.server.createSession", payload, &out); err != nil {
		return platforms.Credentials{}, err
	}
	return platforms.Credentials{
		AccessToken: out.AccessJwt,
		ExternalRef: out.Did,
		Meta: map[string]string{
			"handle": out.Handle,
			"did":    out.Did,
		},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	if creds.AccessToken == "" || creds.ExternalRef == "" {
		return platforms.PublishResult{}, fmt.Errorf("bluesky: session credentials required")
	}
	text := post.Text
	if post.CTAURL != "" && !strings.Contains(text, post.CTAURL) {
		text = strings.TrimSpace(text + "\n\n" + post.CTAURL)
	}
	if len([]rune(text)) > 300 {
		text = string([]rune(text)[:297]) + "..."
	}
	record := map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	payload := map[string]any{
		"repo":       creds.ExternalRef,
		"collection": "app.bsky.feed.post",
		"record":     record,
	}
	var out struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := a.postJSON(ctx, creds.AccessToken, "com.atproto.repo.createRecord", payload, &out); err != nil {
		return platforms.PublishResult{}, err
	}
	handle := creds.Meta["handle"]
	postURL := out.URI
	if handle != "" {
		// at://did/app.bsky.feed.post/rkey
		parts := strings.Split(out.URI, "/")
		rkey := parts[len(parts)-1]
		postURL = fmt.Sprintf("https://bsky.app/profile/%s/post/%s", handle, rkey)
	}
	return platforms.PublishResult{
		ExternalID: out.URI,
		URL:        postURL,
		Raw:        map[string]any{"cid": out.CID},
	}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	_ = ctx
	_ = creds
	_ = externalID
	return platforms.PlatformStats{}, nil
}

func (a *Adapter) postJSON(ctx context.Context, bearer, method string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("bluesky %s: status %d: %s", method, resp.StatusCode, string(raw))
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(raw, dest)
}
