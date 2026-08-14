package bluesky

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

func (a *Adapter) Engage(ctx context.Context, creds platforms.Credentials, target platforms.EngageTarget) (platforms.EngageResult, error) {
	if creds.AccessToken == "" || creds.ExternalRef == "" {
		return platforms.EngageResult{}, fmt.Errorf("bluesky: session credentials required")
	}
	uri := target.ExternalID
	if uri == "" {
		uri = target.URL
	}
	cid := ""
	if target.Meta != nil {
		cid = target.Meta["cid"]
	}
	if cid == "" && strings.HasPrefix(uri, "at://") {
		var posts struct {
			Posts []struct {
				URI string `json:"uri"`
				CID string `json:"cid"`
			} `json:"posts"`
		}
		if err := a.getJSON(ctx, creds.AccessToken, "app.bsky.feed.getPosts?uris="+url.QueryEscape(uri), &posts); err == nil && len(posts.Posts) > 0 {
			cid = posts.Posts[0].CID
			uri = posts.Posts[0].URI
		}
	}
	switch target.Kind {
	case platforms.EngageLike:
		if uri == "" || cid == "" {
			return platforms.EngageResult{}, fmt.Errorf("bluesky: post uri and cid required to like")
		}
		payload := map[string]any{
			"repo":       creds.ExternalRef,
			"collection": "app.bsky.feed.like",
			"record": map[string]any{
				"$type": "app.bsky.feed.like",
				"subject": map[string]string{
					"uri": uri,
					"cid": cid,
				},
				"createdAt": time.Now().UTC().Format(time.RFC3339),
			},
		}
		var out struct {
			URI string `json:"uri"`
		}
		if err := a.postJSON(ctx, creds.AccessToken, "com.atproto.repo.createRecord", payload, &out); err != nil {
			return platforms.EngageResult{}, err
		}
		return platforms.EngageResult{ExternalID: out.URI, URL: target.URL}, nil
	case platforms.EngageComment:
		if uri == "" || cid == "" {
			return platforms.EngageResult{}, fmt.Errorf("bluesky: post uri and cid required to reply")
		}
		text := target.Text
		if len([]rune(text)) > 300 {
			text = string([]rune(text)[:297]) + "..."
		}
		payload := map[string]any{
			"repo":       creds.ExternalRef,
			"collection": "app.bsky.feed.post",
			"record": map[string]any{
				"$type":     "app.bsky.feed.post",
				"text":      text,
				"createdAt": time.Now().UTC().Format(time.RFC3339),
				"reply": map[string]any{
					"root":   map[string]string{"uri": uri, "cid": cid},
					"parent": map[string]string{"uri": uri, "cid": cid},
				},
			},
		}
		var out struct {
			URI string `json:"uri"`
		}
		if err := a.postJSON(ctx, creds.AccessToken, "com.atproto.repo.createRecord", payload, &out); err != nil {
			return platforms.EngageResult{}, err
		}
		return platforms.EngageResult{ExternalID: out.URI, URL: target.URL}, nil
	default:
		return platforms.EngageResult{}, fmt.Errorf("bluesky: %s not supported via API (manual)", target.Kind)
	}
}

func (a *Adapter) getJSON(ctx context.Context, bearer, method string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/"+method, nil)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("bluesky GET %s: status %d: %s", method, resp.StatusCode, string(raw))
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(raw, dest)
}
