package crawler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
)

type Runner struct {
	Store  *store.Store
	Client *http.Client
}

func NewRunner(st *store.Store) *Runner {
	return &Runner{
		Store:  st,
		Client: &http.Client{Timeout: 45 * time.Second},
	}
}

func (r *Runner) Run(ctx context.Context, workspaceID uuid.UUID, kind, feedURL, query string) (int, error) {
	return r.RunWatch(ctx, workspaceID, nil, kind, feedURL, query, "mentions")
}

func (r *Runner) RunWatch(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID, kind, feedURL, query, purpose string) (int, error) {
	if purpose == "" {
		purpose = "mentions"
	}
	switch kind {
	case "hn":
		return r.runHN(ctx, workspaceID, brandID, feedURL, query, purpose)
	case "reddit_search":
		return r.runRedditSearch(ctx, workspaceID, brandID, feedURL, query, purpose)
	default:
		return r.runRSS(ctx, workspaceID, brandID, feedURL, query, purpose)
	}
}

func (r *Runner) runRSS(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID, feedURL, query, purpose string) (int, error) {
	if feedURL == "" {
		return 0, nil
	}
	fp := gofeed.NewParser()
	fp.Client = r.Client
	fp.UserAgent = "PRobot/0.1 reputation-watch"
	feed, err := fp.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return 0, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	count := 0
	source := feed.Title
	if source == "" {
		source = "rss"
	}
	prefiltered := strings.Contains(feedURL, "news.google.com") || strings.Contains(feedURL, "bing.com/news")
	for _, item := range feed.Items {
		title := StripHTML(item.Title)
		snippet := StripHTML(item.Description)
		link := item.Link
		if link == "" {
			continue
		}
		hay := strings.ToLower(title + " " + snippet + " " + link)
		if q != "" && !matchAnyTerm(hay, q) && !prefiltered {
			continue
		}
		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}
		found := time.Time{}
		if item.PublishedParsed != nil {
			found = *item.PublishedParsed
		}
		n, err := r.IngestPublic(ctx, workspaceID, brandID, source, link, title, snippet, author, query, purpose, found)
		if err != nil {
			continue
		}
		count += n
	}
	return count, nil
}

func (r *Runner) ingest(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID, source, link, title, snippet, author, query, purpose string, found time.Time) (int, error) {
	return r.IngestPublic(ctx, workspaceID, brandID, source, link, title, snippet, author, query, purpose, found)
}

func (r *Runner) IngestPublic(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID, source, link, title, snippet, author, query, purpose string, found time.Time) (int, error) {
	title = StripHTML(title)
	snippet = StripHTML(snippet)
	sentiment, severity := Classify(title, snippet)
	if !KeepForWatch(purpose, sentiment) {
		return 0, nil
	}
	status := "new"
	if sentiment == "negative" && severity == "high" {
		status = "escalated"
	}
	m := &domain.Mention{
		WorkspaceID: workspaceID,
		BrandID:     brandID,
		Source:      source,
		URL:         link,
		Title:       title,
		Snippet:     trim(snippet, 500),
		Author:      author,
		Status:      status,
		Sentiment:   sentiment,
		Severity:    severity,
		WatchQuery:  query,
		FoundAt:     found,
	}
	sum := sha256.Sum256([]byte(link + "|" + title))
	hash := hex.EncodeToString(sum[:])
	if err := r.Store.UpsertMention(ctx, m, hash); err != nil {
		return 0, err
	}
	return 1, nil
}

func firstTerm(q string) string {
	q = strings.TrimSpace(q)
	q = strings.Split(q, " OR ")[0]
	q = strings.Trim(q, `"'()`)
	return q
}

func matchAnyTerm(hay, query string) bool {
	for _, part := range strings.Split(query, " OR ") {
		part = strings.ToLower(strings.Trim(strings.TrimSpace(part), `"'()`))
		if part != "" && strings.Contains(hay, part) {
			return true
		}
	}
	return query == "" || strings.Contains(hay, strings.ToLower(query))
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
