package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (r *Runner) runHN(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID, feedURL, query, purpose string) (int, error) {
	q := firstTerm(query)
	if q == "" {
		return 0, nil
	}
	u := "https://hn.algolia.com/api/v1/search?query=" + url.QueryEscape(q) + "&hitsPerPage=30"
	comments := strings.Contains(feedURL, "tags=comment")
	if comments {
		u += "&tags=comment"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "PRobot/0.1 reputation-watch")
	resp, err := r.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("hn search: %d %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Hits []struct {
			ObjectID    string `json:"objectID"`
			Title       string `json:"title"`
			StoryTitle  string `json:"story_title"`
			CommentText string `json:"comment_text"`
			URL         string `json:"url"`
			Author      string `json:"author"`
			CreatedAt   string `json:"created_at"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	count := 0
	source := "Hacker News"
	if comments {
		source = "Hacker News comments"
	}
	for _, h := range out.Hits {
		title := h.Title
		if title == "" {
			title = h.StoryTitle
		}
		if title == "" {
			title = "HN mention"
		}
		link := h.URL
		if comments || link == "" {
			link = "https://news.ycombinator.com/item?id=" + h.ObjectID
		}
		snippet := h.CommentText
		if snippet == "" {
			snippet = title
		}
		found := time.Time{}
		if t, err := time.Parse(time.RFC3339, h.CreatedAt); err == nil {
			found = t
		}
		n, err := r.ingest(ctx, workspaceID, brandID, source, link, title, snippet, h.Author, query, purpose, found)
		if err != nil {
			continue
		}
		count += n
	}
	return count, nil
}
