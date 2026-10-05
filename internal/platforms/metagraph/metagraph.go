// Package metagraph is a minimal client for Meta Graph-style APIs
// (Facebook, Instagram, Threads), which share request and error shapes.
package metagraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	Name string
	Base string
	HTTP *http.Client
}

func New(name, base string) *Client {
	return &Client{Name: name, Base: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 45 * time.Second}}
}

type apiError struct {
	Error *struct {
		Message      string `json:"message"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
		UserMessage  string `json:"error_user_msg"`
	} `json:"error"`
}

func (c *Client) Get(ctx context.Context, path, token string, params url.Values, dest any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("access_token", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	return c.do(req, dest)
}

func (c *Client) Post(ctx context.Context, path, token string, form url.Values, dest any) error {
	if form == nil {
		form = url.Values{}
	}
	form.Set("access_token", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, dest)
}

// do never includes the request URL in errors: it carries the access token.
func (c *Client) do(req *http.Request, dest any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s: request failed: %w", c.Name, stripURL(err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var ae apiError
	_ = json.Unmarshal(raw, &ae)
	if ae.Error != nil {
		msg := ae.Error.UserMessage
		if msg == "" {
			msg = ae.Error.Message
		}
		return fmt.Errorf("%s: %s (code %d)", c.Name, msg, ae.Error.Code)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: status %d: %s", c.Name, resp.StatusCode, truncate(string(raw), 300))
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("%s: bad response: %w", c.Name, err)
	}
	return nil
}

func stripURL(err error) error {
	if ue, ok := err.(*url.Error); ok {
		return ue.Err
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// WaitReady polls fetch until it reports done, failing on error or timeout.
func WaitReady(ctx context.Context, interval, timeout time.Duration, fetch func() (done bool, err error)) error {
	deadline := time.Now().Add(timeout)
	for {
		done, err := fetch()
		if err != nil || done {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("media is still processing after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// ComposeText appends ctaURL to text if missing and trims the body so the
// result fits limit runes while keeping the link intact.
func ComposeText(text, ctaURL string, limit int) string {
	text = strings.TrimSpace(text)
	suffix := ""
	if ctaURL != "" && !strings.Contains(text, ctaURL) {
		suffix = "\n\n" + ctaURL
	}
	budget := limit - len([]rune(suffix))
	if budget < 1 {
		return TruncateRunes(text, limit)
	}
	return TruncateRunes(text, budget) + suffix
}

func TruncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

func IsVideoURL(u string) bool {
	p := strings.ToLower(u)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	for _, ext := range []string{".mp4", ".mov", ".m4v", ".webm"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}
