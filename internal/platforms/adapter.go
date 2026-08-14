package platforms

import (
	"context"
)

type AuthInput struct {
	Token        string
	RefreshToken string
	ExternalRef  string
	Meta         map[string]string
}

type Credentials struct {
	AccessToken  string
	RefreshToken string
	ExternalRef  string
	Meta         map[string]string
}

type NormalizedPost struct {
	Text      string
	MediaURLs []string
	CTAURL    string
	ReplyTo   string
	Meta      map[string]string
}

type PublishResult struct {
	ExternalID string
	URL        string
	Raw        map[string]any
}

type PlatformStats struct {
	Reach    int64
	Likes    int64
	Comments int64
	Shares   int64
	Clicks   int64
	Raw      map[string]any
}

type Adapter interface {
	ID() string
	Connect(ctx context.Context, auth AuthInput) (Credentials, error)
	Publish(ctx context.Context, creds Credentials, post NormalizedPost) (PublishResult, error)
	FetchStats(ctx context.Context, creds Credentials, externalID string) (PlatformStats, error)
}

type EngageKind string

const (
	EngageLike    EngageKind = "like"
	EngageComment EngageKind = "comment"
	EngageFollow  EngageKind = "follow"
	EngageShare   EngageKind = "share"
)

type EngageTarget struct {
	Kind       EngageKind
	ExternalID string
	URL        string
	Text       string
	Meta       map[string]string
}

type EngageResult struct {
	ExternalID string
	URL        string
}

type Engager interface {
	Engage(ctx context.Context, creds Credentials, target EngageTarget) (EngageResult, error)
}

func AsEngager(a Adapter) (Engager, bool) {
	e, ok := a.(Engager)
	return e, ok
}

type Registry struct {
	byID map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{byID: make(map[string]Adapter, len(adapters))}
	for _, a := range adapters {
		r.byID[a.ID()] = a
	}
	return r
}

func (r *Registry) Get(id string) (Adapter, bool) {
	a, ok := r.byID[id]
	return a, ok
}

func (r *Registry) List() []string {
	out := make([]string, 0, len(r.byID))
	for id := range r.byID {
		out = append(out, id)
	}
	return out
}
