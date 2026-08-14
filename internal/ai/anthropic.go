package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/digkill/probot/internal/domain"
)

func (c *Client) runAnthropic(ctx context.Context, agent domain.AIAgent, preset ProviderPreset, input RunInput) (RunOutput, error) {
	apiKey := c.resolveKey(agent, preset)
	if apiKey == "" {
		return RunOutput{}, fmt.Errorf("AI API key not configured (%s)", preset.APIKeyEnv)
	}
	model := agent.Model
	if model == "" {
		model = preset.Model
	}
	payload := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"system":     agent.SystemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": c.userContent(input)},
		},
	}
	body, _ := json.Marshal(payload)
	base := agent.BaseURL
	if base == "" {
		base = preset.BaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return RunOutput{}, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return RunOutput{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return RunOutput{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return RunOutput{}, err
	}
	var b strings.Builder
	for _, part := range parsed.Content {
		if part.Type == "text" || part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return RunOutput{Text: b.String(), TokensIn: parsed.Usage.InputTokens, TokensOut: parsed.Usage.OutputTokens}, nil
}
