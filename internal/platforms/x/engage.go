package x

import (
	"context"
	"fmt"
	"strings"

	"github.com/digkill/probot/internal/platforms"
)

func (a *Adapter) Engage(ctx context.Context, creds platforms.Credentials, target platforms.EngageTarget) (platforms.EngageResult, error) {
	if creds.AccessToken == "" {
		return platforms.EngageResult{}, fmt.Errorf("x: access token required")
	}
	tweetID := extractXStatusID(target.ExternalID, target.URL)
	if tweetID == "" && target.Kind != platforms.EngageFollow {
		return platforms.EngageResult{}, fmt.Errorf("x: tweet id required")
	}
	userID := creds.Meta["user_id"]
	switch target.Kind {
	case platforms.EngageLike:
		if userID == "" {
			return platforms.EngageResult{}, fmt.Errorf("x: user_id missing on credentials")
		}
		path := "/users/" + userID + "/likes"
		if err := a.do(ctx, "POST", path, creds.AccessToken, map[string]any{"tweet_id": tweetID}, nil); err != nil {
			return platforms.EngageResult{}, err
		}
		return platforms.EngageResult{ExternalID: tweetID, URL: target.URL}, nil
	case platforms.EngageComment:
		res, err := a.Publish(ctx, creds, platforms.NormalizedPost{Text: target.Text, ReplyTo: tweetID})
		if err != nil {
			return platforms.EngageResult{}, err
		}
		return platforms.EngageResult{ExternalID: res.ExternalID, URL: res.URL}, nil
	case platforms.EngageFollow:
		targetUser := firstNonEmpty(target.ExternalID, target.Meta["user_id"])
		if userID == "" || targetUser == "" {
			return platforms.EngageResult{}, fmt.Errorf("x: follower and target user id required")
		}
		path := "/users/" + userID + "/following"
		if err := a.do(ctx, "POST", path, creds.AccessToken, map[string]any{"target_user_id": targetUser}, nil); err != nil {
			return platforms.EngageResult{}, err
		}
		return platforms.EngageResult{ExternalID: targetUser, URL: target.URL}, nil
	default:
		return platforms.EngageResult{}, fmt.Errorf("x: %s not supported via API (manual)", target.Kind)
	}
}

func extractXStatusID(externalID, rawURL string) string {
	if externalID != "" && !strings.Contains(externalID, "/") {
		return externalID
	}
	parts := strings.Split(rawURL, "/status/")
	if len(parts) == 2 {
		return strings.Split(strings.Split(parts[1], "?")[0], "/")[0]
	}
	return strings.TrimSpace(externalID)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
