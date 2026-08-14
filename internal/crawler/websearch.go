package crawler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type WebHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

var ddgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://[^"]+)"[^>]*>([^<]{3,180})</a>`)

func SearchWeb(ctx context.Context, client *http.Client, brand, site string) []WebHit {
	if client == nil {
		client = http.DefaultClient
	}
	queries := []string{
		brand + ` отзыв жалоба`,
		brand + ` review scam complaint`,
		`"` + brand + `" "не работает"`,
	}
	if site != "" && site != "https://example.com" {
		queries = append(queries, `site:`+hostOf(site)+` review OR жалоба OR scam`)
	}
	seen := map[string]bool{}
	var hits []WebHit
	for _, q := range queries {
		for _, h := range searchDDG(ctx, client, q) {
			key := strings.ToLower(h.URL)
			if h.URL == "" || seen[key] || strings.Contains(key, "duckduckgo.com") {
				continue
			}
			seen[key] = true
			hits = append(hits, h)
			if len(hits) >= 20 {
				return hits
			}
		}
	}
	return hits
}

func searchDDG(ctx context.Context, client *http.Client, q string) []WebHit {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "PRobot/0.1 reputation-watch")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	html := string(raw)
	var hits []WebHit
	for _, m := range ddgLinkRe.FindAllStringSubmatch(html, 30) {
		link := strings.TrimSpace(m[1])
		title := StripHTML(m[2])
		if title == "" || strings.Contains(link, "duckduckgo.com") {
			continue
		}
		hits = append(hits, WebHit{Title: title, URL: link, Snippet: title})
	}
	return hits
}
