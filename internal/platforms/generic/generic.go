package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/digkill/probot/internal/platforms"
)

type WebhookConfig struct {
	URL     string
	Headers map[string]string
}

type ManualPackage struct {
	Text      string   `json:"text"`
	MediaURLs []string `json:"media_urls"`
	CTAURL    string   `json:"cta_url"`
	Hint      string   `json:"hint"`
}

type Publisher struct {
	client *http.Client
}

func NewPublisher() *Publisher {
	return &Publisher{client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Publisher) PublishWebhook(ctx context.Context, cfg WebhookConfig, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	if cfg.URL == "" {
		return platforms.PublishResult{}, fmt.Errorf("webhook url required")
	}
	payload := map[string]any{
		"text":       post.Text,
		"media_urls": post.MediaURLs,
		"cta_url":    post.CTAURL,
		"meta":       post.Meta,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return platforms.PublishResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return platforms.PublishResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return platforms.PublishResult{}, fmt.Errorf("webhook status %d: %s", resp.StatusCode, string(raw))
	}
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	extID, _ := parsed["id"].(string)
	extURL, _ := parsed["url"].(string)
	return platforms.PublishResult{ExternalID: extID, URL: extURL, Raw: parsed}, nil
}

func (p *Publisher) PrepareManual(post platforms.NormalizedPost) ManualPackage {
	return ManualPackage{
		Text:      post.Text,
		MediaURLs: post.MediaURLs,
		CTAURL:    post.CTAURL,
		Hint:      "Publish manually, then confirm with external URL via API",
	}
}
