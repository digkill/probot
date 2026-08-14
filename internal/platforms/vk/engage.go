package vk

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/digkill/probot/internal/platforms"
)

func (a *Adapter) Engage(ctx context.Context, creds platforms.Credentials, target platforms.EngageTarget) (platforms.EngageResult, error) {
	if creds.AccessToken == "" {
		return platforms.EngageResult{}, fmt.Errorf("vk: access token required")
	}
	ownerID, itemID, err := parseVKPost(target.ExternalID, target.URL)
	if err != nil {
		return platforms.EngageResult{}, err
	}
	switch target.Kind {
	case platforms.EngageLike:
		q := url.Values{}
		q.Set("access_token", creds.AccessToken)
		q.Set("v", a.apiVer)
		q.Set("type", "post")
		q.Set("owner_id", ownerID)
		q.Set("item_id", itemID)
		var out struct {
			Error *struct{ ErrorMsg string `json:"error_msg"` } `json:"error"`
		}
		if err := a.call(ctx, "likes.add", q, &out); err != nil {
			return platforms.EngageResult{}, err
		}
		if out.Error != nil {
			return platforms.EngageResult{}, fmt.Errorf("vk likes.add: %s", out.Error.ErrorMsg)
		}
		return platforms.EngageResult{ExternalID: ownerID + "_" + itemID, URL: target.URL}, nil
	case platforms.EngageComment:
		if strings.TrimSpace(target.Text) == "" {
			return platforms.EngageResult{}, fmt.Errorf("vk: comment text required")
		}
		q := url.Values{}
		q.Set("access_token", creds.AccessToken)
		q.Set("v", a.apiVer)
		q.Set("owner_id", ownerID)
		q.Set("post_id", itemID)
		q.Set("message", target.Text)
		var out struct {
			Response struct {
				CommentID int64 `json:"comment_id"`
			} `json:"response"`
			Error *struct{ ErrorMsg string `json:"error_msg"` } `json:"error"`
		}
		if err := a.call(ctx, "wall.createComment", q, &out); err != nil {
			return platforms.EngageResult{}, err
		}
		if out.Error != nil {
			return platforms.EngageResult{}, fmt.Errorf("vk wall.createComment: %s", out.Error.ErrorMsg)
		}
		return platforms.EngageResult{
			ExternalID: fmt.Sprintf("%d", out.Response.CommentID),
			URL:        fmt.Sprintf("https://vk.com/wall%s_%s?reply=%d", ownerID, itemID, out.Response.CommentID),
		}, nil
	default:
		return platforms.EngageResult{}, fmt.Errorf("vk: %s not supported via API (manual)", target.Kind)
	}
}

func parseVKPost(externalID, rawURL string) (ownerID, itemID string, err error) {
	id := strings.TrimPrefix(externalID, "wall")
	if strings.Contains(id, "_") {
		parts := strings.SplitN(id, "_", 2)
		return parts[0], parts[1], nil
	}
	if i := strings.Index(rawURL, "wall"); i >= 0 {
		rest := rawURL[i+4:]
		rest = strings.Split(rest, "?")[0]
		parts := strings.SplitN(rest, "_", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}
	return "", "", fmt.Errorf("vk: cannot parse owner_id/item_id from %q", externalID)
}
