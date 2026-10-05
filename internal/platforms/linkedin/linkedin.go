package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/platforms/metagraph"
)

// apiVersion is LinkedIn's monthly API version (YYYYMM); each one is supported
// for about a year, so bump it when LinkedIn starts rejecting it.
const apiVersion = "202604"

const commentaryLimit = 3000

// Adapter publishes via the LinkedIn Posts API. Token: OAuth access token with
// w_member_social (personal) or w_organization_social (company page).
// ExternalRef: organization ID / URN for a company page; empty = the member.
type Adapter struct {
	client *http.Client
	base   string
}

func New() *Adapter {
	return &Adapter{client: &http.Client{Timeout: 45 * time.Second}, base: "https://api.linkedin.com"}
}

func (a *Adapter) ID() string { return "linkedin" }

var digits = regexp.MustCompile(`^\d+$`)

func (a *Adapter) Connect(ctx context.Context, auth platforms.AuthInput) (platforms.Credentials, error) {
	if auth.Token == "" {
		return platforms.Credentials{}, fmt.Errorf("linkedin: access token required")
	}
	ref := strings.TrimSpace(auth.ExternalRef)
	meta := map[string]string{}
	switch {
	case ref == "":
		var me struct {
			Sub  string `json:"sub"`
			Name string `json:"name"`
		}
		if _, err := a.do(ctx, http.MethodGet, "/v2/userinfo", auth.Token, nil, &me); err != nil {
			return platforms.Credentials{}, err
		}
		if me.Sub == "" {
			return platforms.Credentials{}, fmt.Errorf("linkedin: could not resolve member id (token needs openid+profile scopes, or pass an organization id)")
		}
		ref = "urn:li:person:" + me.Sub
		meta["name"] = me.Name
	case digits.MatchString(ref):
		ref = "urn:li:organization:" + ref
	case strings.HasPrefix(ref, "urn:li:person:"), strings.HasPrefix(ref, "urn:li:organization:"):
	default:
		return platforms.Credentials{}, fmt.Errorf("linkedin: external_ref must be an organization id or urn:li:person/organization URN")
	}
	return platforms.Credentials{AccessToken: auth.Token, ExternalRef: ref, Meta: meta}, nil
}

func (a *Adapter) Publish(ctx context.Context, creds platforms.Credentials, post platforms.NormalizedPost) (platforms.PublishResult, error) {
	if creds.AccessToken == "" || creds.ExternalRef == "" {
		return platforms.PublishResult{}, fmt.Errorf("linkedin: author and token required")
	}
	text := strings.TrimSpace(post.Text)
	body := map[string]any{
		"author":     creds.ExternalRef,
		"visibility": "PUBLIC",
		"distribution": map[string]any{
			"feedDistribution":               "MAIN_FEED",
			"targetEntities":                 []any{},
			"thirdPartyDistributionChannels": []any{},
		},
		"lifecycleState":            "PUBLISHED",
		"isReshareDisabledByAuthor": false,
	}
	if post.CTAURL != "" {
		title := post.Meta["title"]
		if title == "" {
			title = strings.SplitN(text, "\n", 2)[0]
		}
		body["content"] = map[string]any{"article": map[string]any{
			"source": post.CTAURL,
			"title":  metagraph.TruncateRunes(strings.TrimSpace(title), 200),
		}}
	}
	// Escaping can add characters, so trim the raw text with headroom first.
	body["commentary"] = EscapeLittleText(metagraph.TruncateRunes(text, commentaryLimit-300))
	hdr, err := a.do(ctx, http.MethodPost, "/rest/posts", creds.AccessToken, body, nil)
	if err != nil {
		return platforms.PublishResult{}, err
	}
	urn := hdr.Get("x-restli-id")
	if urn == "" {
		return platforms.PublishResult{}, fmt.Errorf("linkedin: post created but no id returned")
	}
	return platforms.PublishResult{ExternalID: urn, URL: "https://www.linkedin.com/feed/update/" + urn + "/"}, nil
}

func (a *Adapter) FetchStats(ctx context.Context, creds platforms.Credentials, externalID string) (platforms.PlatformStats, error) {
	if creds.AccessToken == "" || externalID == "" {
		return platforms.PlatformStats{}, nil
	}
	var out struct {
		LikesSummary struct {
			TotalLikes int64 `json:"totalLikes"`
		} `json:"likesSummary"`
		CommentsSummary struct {
			AggregatedTotalComments int64 `json:"aggregatedTotalComments"`
		} `json:"commentsSummary"`
	}
	if _, err := a.do(ctx, http.MethodGet, "/rest/socialActions/"+url.PathEscape(externalID), creds.AccessToken, nil, &out); err != nil {
		return platforms.PlatformStats{}, err
	}
	return platforms.PlatformStats{Likes: out.LikesSummary.TotalLikes, Comments: out.CommentsSummary.AggregatedTotalComments}, nil
}

var littleTextReserved = strings.NewReplacer(
	`\`, `\\`, `|`, `\|`, `{`, `\{`, `}`, `\}`, `@`, `\@`, `[`, `\[`, `]`, `\]`,
	`(`, `\(`, `)`, `\)`, `<`, `\<`, `>`, `\>`, `#`, `\#`, `*`, `\*`, `_`, `\_`, `~`, `\~`,
)

// EscapeLittleText escapes characters reserved by LinkedIn's "little text"
// commentary format; unescaped ones make the API reject or mangle the post.
func EscapeLittleText(s string) string { return littleTextReserved.Replace(s) }

func (a *Adapter) do(ctx context.Context, method, path, token string, payload, dest any) (http.Header, error) {
	var rd io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	if strings.HasPrefix(path, "/rest/") {
		req.Header.Set("LinkedIn-Version", apiVersion)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = metagraph.TruncateRunes(string(raw), 300)
		}
		return nil, fmt.Errorf("linkedin: status %d: %s", resp.StatusCode, e.Message)
	}
	if dest != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, dest); err != nil {
			return nil, fmt.Errorf("linkedin: bad response: %w", err)
		}
	}
	return resp.Header, nil
}
