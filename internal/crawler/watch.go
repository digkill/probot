package crawler

import (
	"net/url"
	"strings"

	"github.com/digkill/probot/internal/domain"
)

func googleNewsRSS(q, hl, gl, ceid string) string {
	return "https://news.google.com/rss/search?q=" + url.QueryEscape(q) + "&hl=" + hl + "&gl=" + gl + "&ceid=" + ceid
}

func ReviewWatchSources(brand domain.Brand) []domain.CrawlSource {
	watch := BrandWatchQuery(brand.Name, brand.Slug, brand.CanonicalURL)
	newsQ := NewsSearchQuery(watch)
	interval := 1800
	bid := brand.ID
	src := func(kind, sourceURL string) domain.CrawlSource {
		return domain.CrawlSource{
			WorkspaceID: brand.WorkspaceID,
			BrandID:     &bid,
			Kind:        kind,
			Purpose:     "reviews",
			Query:       watch,
			IntervalSec: interval,
			Enabled:     true,
			URL:         sourceURL,
		}
	}
	out := []domain.CrawlSource{
		src("news", googleNewsRSS(newsQ, "ru", "RU", "RU:ru")),
		src("news", googleNewsRSS(newsQ, "en-US", "US", "US:en")),
		src("news", "https://www.bing.com/news/search?q="+url.QueryEscape(newsQ)+"&format=rss"),
		src("hn", "https://hn.algolia.com/api/v1/search"),
		src("hn", "https://hn.algolia.com/api/v1/search?tags=comment"),
		src("reddit_search", "https://www.reddit.com/search.json"),
		src("reddit_search", "https://www.reddit.com/search.json?type=comment"),
		src("ai_search", "ai://research"),
	}
	if host := hostOf(brand.CanonicalURL); host != "" && host != "localhost" && host != "example.com" {
		siteQ := "site:" + host + " (отзыв OR жалоба OR scam OR review OR complaint OR \"не работает\")"
		out = append(out,
			src("news", googleNewsRSS(siteQ, "ru", "RU", "RU:ru")),
			src("news", googleNewsRSS("site:youtube.com ("+watch+") (review OR scam OR жалоба OR отзыв)", "en-US", "US", "US:en")),
		)
	}
	return out
}

func BrandWatchQuery(name, slug, canonical string) string {
	parts := []string{strings.TrimSpace(name)}
	if slug != "" && !strings.EqualFold(slug, name) {
		parts = append(parts, slug)
	}
	if host := hostOf(canonical); host != "" && host != "localhost" {
		parts = append(parts, host)
		bare := strings.TrimPrefix(host, "www.")
		if bare != host {
			parts = append(parts, bare)
		}
	}
	seen := map[string]bool{}
	var uniq []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		uniq = append(uniq, p)
	}
	return strings.Join(uniq, " OR ")
}

func NewsSearchQuery(watch string) string {
	watch = strings.TrimSpace(watch)
	if watch == "" {
		return ""
	}
	return "(" + watch + ") (жалоба OR scam OR review OR \"не работает\" OR terrible OR refund OR мошен OR развод OR complaint)"
}

func hostOf(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
