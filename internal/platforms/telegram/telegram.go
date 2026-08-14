package telegram

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
	client *http.Client
}

func New() *Adapter {
	return &Adapter{client: &http.Client{Timeout: 30 * time.Second}}
}

func (a *Adapter) ID() string { return "telegram" }

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	if auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("telegram: token required")
	}
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
			ID       int64  `json:"id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := a.get(ctx, auth.Token, "getMe", nil, &out); err != nil {
		return platforms.Credentials{}, err
	}
	if !out.OK {
		return platforms.Credentials{}, fmt.Errorf("telegram getMe: %s", out.Description)
	}
	ref := auth.ExternalRef
	if ref == "" {
		ref = out.Result.Username
	}
	return platforms.Credentials{
		AccessToken: auth.Token,
		ExternalRef: ref,
		Meta:        map[string]string{"bot_username": out.Result.Username},
	}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	chatID := creds.ExternalRef
	if chatID == "" {
		return platforms.PublishResult{}, fmt.Errorf("telegram: chat_id/external_ref required")
	}
	text := post.Text
	if post.CTAURL != "" && !strings.Contains(text, post.CTAURL) {
		text = strings.TrimSpace(text + "\n\n" + post.CTAURL)
	}
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "false")

	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"chat"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := a.postForm(ctx, creds.AccessToken, "sendMessage", form, &out); err != nil {
		return platforms.PublishResult{}, err
	}
	if !out.OK {
		return platforms.PublishResult{}, fmt.Errorf("telegram sendMessage: %s", out.Description)
	}
	msgURL := fmt.Sprintf("https://t.me/%s/%d", out.Result.Chat.Username, out.Result.MessageID)
	if out.Result.Chat.Username == "" {
		msgURL = fmt.Sprintf("https://t.me/c/%d/%d", out.Result.Chat.ID, out.Result.MessageID)
	}
	return platforms.PublishResult{
		ExternalID: fmt.Sprintf("%d", out.Result.MessageID),
		URL:        msgURL,
	}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	_ = ctx
	_ = creds
	_ = externalID
	return platforms.PlatformStats{}, nil
}

func (a *Adapter) get(ctx context.Context, token, method string, q url.Values, dest any) error {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s", token, method)
	if q != nil {
		u += "?" + q.Encode()
	}
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
		return fmt.Errorf("telegram %s: status %d: %s", method, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, dest)
}

func (a *Adapter) postForm(ctx context.Context, token, method string, form url.Values, dest any) error {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s", token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram %s: status %d: %s", method, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, dest)
}
