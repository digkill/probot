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

func (c *Client) runGemini(ctx context.Context, agent domain.AIAgent, preset ProviderPreset, input RunInput) (RunOutput, error) {
	apiKey := c.resolveKey(agent, preset)
	if apiKey == "" {
		return RunOutput{}, fmt.Errorf("AI API key not configured (%s)", preset.APIKeyEnv)
	}
	model := agent.Model
	if model == "" {
		model = preset.Model
	}
	base := agent.BaseURL
	if base == "" {
		base = preset.BaseURL
	}
	payload := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": c.userContent(input)}}},
		},
		"generationConfig": map[string]any{"maxOutputTokens": 4096},
	}
	if agent.SystemPrompt != "" {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]string{{"text": agent.SystemPrompt}},
		}
	}
	if input.LiveSearch {
		payload["tools"] = []map[string]any{{"google_search": map[string]any{}}}
	}
	body, _ := json.Marshal(payload)
	u := strings.TrimRight(base, "/") + "/models/" + model + ":generateContent?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return RunOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return RunOutput{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 && input.LiveSearch {
		input.LiveSearch = false
		return c.runGemini(ctx, agent, preset, input)
	}
	if resp.StatusCode >= 300 {
		return RunOutput{}, fmt.Errorf("gemini status %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return RunOutput{}, err
	}
	var b strings.Builder
	if len(parsed.Candidates) > 0 {
		for _, part := range parsed.Candidates[0].Content.Parts {
			b.WriteString(part.Text)
		}
	}
	return RunOutput{
		Text:      b.String(),
		TokensIn:  parsed.UsageMetadata.PromptTokenCount,
		TokensOut: parsed.UsageMetadata.CandidatesTokenCount,
	}, nil
}
