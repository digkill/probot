package ai

import "testing"

func TestNormalizeProvider(t *testing.T) {
	if NormalizeProvider("Claude") != "anthropic" {
		t.Fatal("claude")
	}
	if NormalizeProvider("xai") != "grok" {
		t.Fatal("xai")
	}
	if NormalizeProvider("google") != "gemini" {
		t.Fatal("google")
	}
}

func TestParseFindings(t *testing.T) {
	raw := "```json\n{\"findings\":[{\"title\":\"Scam report\",\"url\":\"https://example.com/r\",\"snippet\":\"They never refund\",\"sentiment\":\"negative\",\"severity\":\"high\"}]}\n```"
	got := ParseFindings(raw)
	if len(got) != 1 || got[0].Title != "Scam report" || got[0].Sentiment != "negative" {
		t.Fatalf("%+v", got)
	}
}

func TestParseFindingsEmpty(t *testing.T) {
	if len(ParseFindings(`{"findings":[]}`)) != 0 {
		t.Fatal("expected empty")
	}
}

func TestPresetClaude(t *testing.T) {
	p := Preset("anthropic")
	if p.APIKeyEnv != "ANTHROPIC_API_KEY" || p.Compat != "anthropic" {
		t.Fatalf("%+v", p)
	}
}
