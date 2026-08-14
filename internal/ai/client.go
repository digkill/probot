package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/digkill/probot/internal/config"
	"github.com/digkill/probot/internal/domain"
)

type Client struct {
	cfg    *config.Config
	client *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg:    cfg,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

type ProviderStatus struct {
	ProviderPreset
	Configured bool `json:"configured"`
}

func (c *Client) ProviderStatus() []ProviderStatus {
	var out []ProviderStatus
	for _, p := range Presets() {
		out = append(out, ProviderStatus{ProviderPreset: p, Configured: c.cfg.ResolveAPIKey(p.APIKeyEnv) != ""})
	}
	return out
}

type RunInput struct {
	Prompt     string         `json:"prompt"`
	Context    map[string]any `json:"context,omitempty"`
	LiveSearch bool           `json:"live_search,omitempty"`
}

type RunOutput struct {
	Text      string         `json:"text"`
	ImageURL  string         `json:"image_url,omitempty"`
	Raw       map[string]any `json:"raw,omitempty"`
	TokensIn  int            `json:"tokens_in"`
	TokensOut int            `json:"tokens_out"`
}

func (c *Client) Run(ctx context.Context, agent domain.AIAgent, input RunInput) (RunOutput, error) {
	if agent.Role == domain.AgentImage {
		return c.runImage(ctx, agent, input)
	}
	preset := Preset(agent.Provider)
	switch preset.Compat {
	case "anthropic":
		return c.runAnthropic(ctx, agent, preset, input)
	case "gemini":
		return c.runGemini(ctx, agent, preset, input)
	default:
		return c.runOpenAICompat(ctx, agent, preset, input)
	}
}

func (c *Client) resolveKey(agent domain.AIAgent, preset ProviderPreset) string {
	env := agent.APIKeyEnv
	if env == "" {
		env = preset.APIKeyEnv
	}
	return c.cfg.ResolveAPIKey(env)
}

func (c *Client) userContent(input RunInput) string {
	parts := []string{input.Prompt}
	if len(input.Context) > 0 {
		b, _ := json.Marshal(input.Context)
		parts = append(parts, "Context JSON:\n"+string(b))
	}
	return strings.Join(parts, "\n\n")
}

func (c *Client) runOpenAICompat(ctx context.Context, agent domain.AIAgent, preset ProviderPreset, input RunInput) (RunOutput, error) {
	apiKey := c.resolveKey(agent, preset)
	if apiKey == "" {
		return RunOutput{}, fmt.Errorf("AI API key not configured (%s)", preset.APIKeyEnv)
	}
	base := agent.BaseURL
	if base == "" {
		base = preset.BaseURL
	}
	model := agent.Model
	if model == "" {
		model = preset.Model
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": agent.SystemPrompt},
			{"role": "user", "content": c.userContent(input)},
		},
	}
	if input.LiveSearch && NormalizeProvider(agent.Provider) == "grok" {
		payload["search_parameters"] = map[string]any{"mode": "on", "return_citations": true}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return RunOutput{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return RunOutput{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return RunOutput{}, fmt.Errorf("%s status %d: %s", preset.ID, resp.StatusCode, string(raw))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return RunOutput{}, err
	}
	text := ""
	if len(parsed.Choices) > 0 {
		text = parsed.Choices[0].Message.Content
	}
	return RunOutput{Text: text, TokensIn: parsed.Usage.PromptTokens, TokensOut: parsed.Usage.CompletionTokens}, nil
}

func (c *Client) runImage(ctx context.Context, agent domain.AIAgent, input RunInput) (RunOutput, error) {
	envName := agent.APIKeyEnv
	if envName == "" || envName == "OPENAI_API_KEY" {
		envName = "IMAGE_API_KEY"
	}
	apiKey := c.cfg.ResolveAPIKey(envName)
	if apiKey == "" {
		apiKey = c.cfg.ResolveAPIKey("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return RunOutput{}, fmt.Errorf("image API key not configured")
	}
	base := agent.BaseURL
	if base == "" {
		base = c.cfg.ImageBaseURL
	}
	model := agent.Model
	if model == "" {
		model = c.cfg.ImageModel
	}
	prompt := input.Prompt
	if agent.SystemPrompt != "" {
		prompt = agent.SystemPrompt + "\n\n" + prompt
	}
	payload := map[string]any{
		"model":  model,
		"prompt": prompt,
		"n":      1,
		"size":   "1024x1024",
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return RunOutput{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return RunOutput{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return RunOutput{}, fmt.Errorf("image status %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return RunOutput{}, err
	}
	url := ""
	if len(parsed.Data) > 0 {
		url = parsed.Data[0].URL
	}
	return RunOutput{Text: "image generated", ImageURL: url}, nil
}
