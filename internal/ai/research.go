package ai

import (
	"encoding/json"
	"regexp"
	"strings"
)

type Finding struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Snippet   string `json:"snippet"`
	Author    string `json:"author"`
	Source    string `json:"source"`
	Sentiment string `json:"sentiment"`
	Severity  string `json:"severity"`
}

var jsonFence = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

func ResearchPrompt(brandName, site, watch string) string {
	return `You are a brand reputation researcher. Using the web search snippets in context (and live search if your model has it), find public reviews, complaints, scam reports, and notable mentions for this brand.

Brand: ` + brandName + `
Site: ` + site + `
Watch terms: ` + watch + `

Return ONLY JSON:
{"findings":[{"title":"","url":"","snippet":"","author":"","source":"","sentiment":"negative|positive|mixed|unknown","severity":"high|medium|low|none"}]}

Rules:
- Prefer negative reviews, complaints, refund/scam threads
- Use real URLs from the snippets when present; do not invent domains
- Snippet: 1-2 sentences of the actual complaint
- Max 12 findings
- If nothing relevant, return {"findings":[]}`
}

func ParseFindings(text string) []Finding {
	raw := strings.TrimSpace(text)
	if m := jsonFence.FindStringSubmatch(raw); len(m) > 1 {
		raw = m[1]
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	var out struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	var keep []Finding
	for _, f := range out.Findings {
		f.Title = strings.TrimSpace(f.Title)
		f.URL = strings.TrimSpace(f.URL)
		f.Snippet = strings.TrimSpace(f.Snippet)
		if f.Title == "" && f.Snippet == "" {
			continue
		}
		if f.URL == "" {
			f.URL = "https://research.local/" + strings.ReplaceAll(strings.ToLower(f.Title), " ", "-")
		}
		if f.Sentiment == "" {
			f.Sentiment = "unknown"
		}
		if f.Severity == "" {
			f.Severity = "none"
		}
		keep = append(keep, f)
	}
	return keep
}
