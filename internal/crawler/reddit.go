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

func (r *Runner) runRedditSearch(ctx context.Context, workspaceID uuid.UUID, brandID *uuid.UUID, feedURL, query, purpose string) (int, error) {
	q := firstTerm(query)
	if q == "" {
		return 0, nil
	}
	search := q + " (review OR scam OR complaint OR проблема OR отзыв OR развод OR жалоба)"
	u := "https://www.reddit.com/search.json?q=" + url.QueryEscape(search) + "&sort=new&limit=25"
	comments := strings.Contains(feedURL, "type=comment")
	if comments {
		u += "&type=comment"
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
		return 0, fmt.Errorf("reddit search: %d %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data struct {
			Children []struct {
				Data struct {
					Title     string  `json:"title"`
					LinkTitle string  `json:"link_title"`
					Selftext  string  `json:"selftext"`
					Body      string  `json:"body"`
					Permalink string  `json:"permalink"`
					Author    string  `json:"author"`
					Created   float64 `json:"created_utc"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	count := 0
	source := "Reddit"
	if comments {
		source = "Reddit comments"
	}
	for _, c := range out.Data.Children {
		d := c.Data
		link := d.Permalink
		if link == "" {
			continue
		}
		if link[0] == '/' {
			link = "https://www.reddit.com" + link
		}
		title := d.Title
		if title == "" {
			title = d.LinkTitle
		}
		snippet := d.Selftext
		if snippet == "" {
			snippet = d.Body
		}
		found := time.Time{}
		if d.Created > 0 {
			found = time.Unix(int64(d.Created), 0)
		}
		n, err := r.ingest(ctx, workspaceID, brandID, source, link, title, snippet, d.Author, query, purpose, found)
		if err != nil {
			continue
		}
		count += n
	}
	return count, nil
}
