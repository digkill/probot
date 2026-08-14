package telegram

import (
	"context"
	"fmt"
	"net/url"

	"github.com/digkill/probot/internal/platforms"
)

func (a *Adapter) Engage(ctx context.Context, creds platforms.Credentials, target platforms.EngageTarget) (platforms.EngageResult, error) {
	if target.Kind != platforms.EngageComment {
		return platforms.EngageResult{}, fmt.Errorf("telegram: only comments (replies) are supported")
	}
	chatID := creds.ExternalRef
	if chatID == "" {
		return platforms.EngageResult{}, fmt.Errorf("telegram: chat_id required")
	}
	replyTo := target.ExternalID
	if replyTo == "" {
		return platforms.EngageResult{}, fmt.Errorf("telegram: reply_to message_id required")
	}
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", target.Text)
	form.Set("reply_to_message_id", replyTo)
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := a.postForm(ctx, creds.AccessToken, "sendMessage", form, &out); err != nil {
		return platforms.EngageResult{}, err
	}
	if !out.OK {
		return platforms.EngageResult{}, fmt.Errorf("telegram reply: %s", out.Description)
	}
	return platforms.EngageResult{
		ExternalID: fmt.Sprintf("%d", out.Result.MessageID),
		URL:        target.URL,
	}, nil
}
