package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/digkill/probot/internal/platforms"
)

func (a *Adapter) Engage(ctx context.Context, creds platforms.Credentials, target platforms.EngageTarget) (platforms.EngageResult, error) {
	if creds.AccessToken == "" {
		return platforms.EngageResult{}, fmt.Errorf("reddit: access token required")
	}
	id := normalizeRedditID(target.ExternalID, target.URL)
	switch target.Kind {
	case platforms.EngageLike:
		form := url.Values{}
		form.Set("id", id)
		form.Set("dir", "1")
		if err := a.postAPI(ctx, creds.AccessToken, "https://oauth.reddit.com/api/vote", form, nil); err != nil {
			return platforms.EngageResult{}, err
		}
		return platforms.EngageResult{ExternalID: id, URL: target.URL}, nil
	case platforms.EngageComment:
		if strings.TrimSpace(target.Text) == "" {
			return platforms.EngageResult{}, fmt.Errorf("reddit: comment text required")
		}
		form := url.Values{}
		form.Set("parent", id)
		form.Set("text", target.Text)
		form.Set("api_type", "json")
		var out struct {
			JSON struct {
				Errors [][]any `json:"errors"`
				Data   struct {
					Things []struct {
						Data struct {
							Name    string `json:"name"`
							Permalink string `json:"permalink"`
						} `json:"data"`
					} `json:"things"`
				} `json:"data"`
			} `json:"json"`
		}
		if err := a.postAPI(ctx, creds.AccessToken, "https://oauth.reddit.com/api/comment", form, &out); err != nil {
			return platforms.EngageResult{}, err
		}
		if len(out.JSON.Errors) > 0 {
			return platforms.EngageResult{}, fmt.Errorf("reddit comment: %v", out.JSON.Errors)
		}
		res := platforms.EngageResult{ExternalID: id, URL: target.URL}
		if len(out.JSON.Data.Things) > 0 {
			res.ExternalID = out.JSON.Data.Things[0].Data.Name
			if out.JSON.Data.Things[0].Data.Permalink != "" {
				res.URL = "https://reddit.com" + out.JSON.Data.Things[0].Data.Permalink
			}
		}
		return res, nil
	default:
		return platforms.EngageResult{}, fmt.Errorf("reddit: %s not supported via API (manual)", target.Kind)
	}
}

func (a *Adapter) postAPI(ctx context.Context, token, rawURL string, form url.Values, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "PRobot/0.1 by digkill")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("reddit %s: status %d: %s", rawURL, resp.StatusCode, string(raw))
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(raw, dest)
}

func normalizeRedditID(externalID, rawURL string) string {
	id := strings.TrimSpace(externalID)
	if strings.HasPrefix(id, "t3_") || strings.HasPrefix(id, "t1_") {
		return id
	}
	if id != "" {
		return "t3_" + strings.TrimPrefix(id, "/")
	}
	// /r/sub/comments/abc123/...
	parts := strings.Split(rawURL, "/comments/")
	if len(parts) == 2 {
		idPart := strings.Split(parts[1], "/")[0]
		if idPart != "" {
			return "t3_" + idPart
		}
	}
	return id
}
