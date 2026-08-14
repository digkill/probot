package ai

import "strings"

type ProviderPreset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKeyEnv string `json:"api_key_env"`
	Compat    string `json:"compat"` // openai | anthropic | gemini
}

func Presets() []ProviderPreset {
	return []ProviderPreset{
		{ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini", APIKeyEnv: "OPENAI_API_KEY", Compat: "openai"},
		{ID: "anthropic", Name: "Claude (Anthropic)", BaseURL: "https://api.anthropic.com", Model: "claude-sonnet-4-5", APIKeyEnv: "ANTHROPIC_API_KEY", Compat: "anthropic"},
		{ID: "grok", Name: "Grok (xAI)", BaseURL: "https://api.x.ai/v1", Model: "grok-3", APIKeyEnv: "GROK_API_KEY", Compat: "openai"},
		{ID: "gemini", Name: "Gemini (Google)", BaseURL: "https://generativelanguage.googleapis.com/v1beta", Model: "gemini-2.5-flash", APIKeyEnv: "GEMINI_API_KEY", Compat: "gemini"},
	}
}

func NormalizeProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "claude", "anthropic":
		return "anthropic"
	case "grok", "xai", "x.ai":
		return "grok"
	case "gemini", "google":
		return "gemini"
	case "openai", "chatgpt":
		return "openai"
	default:
		if p == "" {
			return "openai"
		}
		return strings.ToLower(p)
	}
}

func Preset(provider string) ProviderPreset {
	id := NormalizeProvider(provider)
	for _, p := range Presets() {
		if p.ID == id {
			return p
		}
	}
	return Presets()[0]
}
