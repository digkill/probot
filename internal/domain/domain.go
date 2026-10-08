package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

type PublicationStatus string

const (
	PubDraft               PublicationStatus = "draft"
	PubScheduled           PublicationStatus = "scheduled"
	PubPublishing          PublicationStatus = "publishing"
	PubLive                PublicationStatus = "live"
	PubFailed              PublicationStatus = "failed"
	PubNeedsManualConfirm  PublicationStatus = "needs_manual_confirm"
)

type AgentRole string

const (
	AgentContent          AgentRole = "content"
	AgentPlatformAdapt    AgentRole = "platform_adapt"
	AgentImage            AgentRole = "image"
	AgentAnalyticsAdvisor AgentRole = "analytics_advisor"
	AgentReputation       AgentRole = "reputation"
	AgentResearch         AgentRole = "research"
)

type Workspace struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
}

type Brand struct {
	ID             uuid.UUID `json:"id"`
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	CanonicalURL   string    `json:"canonical_url"`
	ToneOfVoice    string    `json:"tone_of_voice"`
	ForbiddenWords []string  `json:"forbidden_words"`
	CTADefault     string    `json:"cta_default"`
	CreatedAt      time.Time `json:"created_at"`
}

type PlatformDefinition struct {
	ID          uuid.UUID       `json:"id"`
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	BaseURL     string          `json:"base_url"`
	PublishMode string          `json:"publish_mode"`
	CharLimit   *int            `json:"char_limit,omitempty"`
	Tags        []string        `json:"tags"`
	AudienceFit float64         `json:"audience_fit"`
	Effort      string          `json:"effort"`
	Risk        string          `json:"risk"`
	Lang        string          `json:"lang"`
	Region      string          `json:"region"`
	Status      string          `json:"status"`
	Meta        json.RawMessage `json:"meta"`
}

type CustomPlatform struct {
	ID           uuid.UUID       `json:"id"`
	WorkspaceID  uuid.UUID       `json:"workspace_id"`
	Name         string          `json:"name"`
	Slug         string          `json:"slug"`
	PublishMode  string          `json:"publish_mode"`
	WebhookURL   string          `json:"webhook_url"`
	HTTPTemplate json.RawMessage `json:"http_template"`
}

type Channel struct {
	ID               uuid.UUID  `json:"id"`
	WorkspaceID      uuid.UUID  `json:"workspace_id"`
	BrandID          uuid.UUID  `json:"brand_id"`
	PlatformDefID    *uuid.UUID `json:"platform_def_id,omitempty"`
	CustomPlatformID *uuid.UUID `json:"custom_platform_id,omitempty"`
	Name             string     `json:"name"`
	ExternalRef      string     `json:"external_ref"`
	Health           string     `json:"health"`
}

type Campaign struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	BrandID     uuid.UUID `json:"brand_id"`
	Name        string    `json:"name"`
	Playbook    string    `json:"playbook"`
	Status      string    `json:"status"`
}

type ContentPiece struct {
	ID           uuid.UUID `json:"id"`
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	BrandID      uuid.UUID `json:"brand_id"`
	CampaignID   *uuid.UUID `json:"campaign_id,omitempty"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	CTAURL       string    `json:"cta_url"`
	MediaURLs    []string  `json:"media_urls"`
	UTMSource    string    `json:"utm_source"`
	UTMMedium    string    `json:"utm_medium"`
	UTMCampaign  string    `json:"utm_campaign"`
	Status       string    `json:"status"`
}

type Publication struct {
	ID             uuid.UUID         `json:"id"`
	WorkspaceID    uuid.UUID         `json:"workspace_id"`
	ContentID      uuid.UUID         `json:"content_id"`
	ChannelID      uuid.UUID         `json:"channel_id"`
	CampaignID     *uuid.UUID        `json:"campaign_id,omitempty"`
	Status         PublicationStatus `json:"status"`
	BodyOverride   string            `json:"body_override"`
	ScheduledAt    *time.Time        `json:"scheduled_at,omitempty"`
	PublishedAt    *time.Time        `json:"published_at,omitempty"`
	ExternalID     string            `json:"external_id"`
	ExternalURL    string            `json:"external_url"`
	IdempotencyKey string            `json:"idempotency_key"`
	ErrorMessage   string            `json:"error_message"`
	SortOrder      int               `json:"sort_order"`
}

type CrossLink struct {
	ID                 uuid.UUID  `json:"id"`
	WorkspaceID        uuid.UUID  `json:"workspace_id"`
	CampaignID         *uuid.UUID `json:"campaign_id,omitempty"`
	FromPublicationID  *uuid.UUID `json:"from_publication_id,omitempty"`
	ToPublicationID    *uuid.UUID `json:"to_publication_id,omitempty"`
	ToURL              string     `json:"to_url"`
	Kind               string     `json:"kind"`
}

type Mention struct {
	ID          uuid.UUID  `json:"id"`
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	BrandID     *uuid.UUID `json:"brand_id,omitempty"`
	CampaignID  *uuid.UUID `json:"campaign_id,omitempty"`
	Source      string     `json:"source"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	Snippet     string     `json:"snippet"`
	Author      string     `json:"author"`
	Status      string     `json:"status"`
	Sentiment   string     `json:"sentiment"`
	Severity    string     `json:"severity"`
	WatchQuery  string     `json:"watch_query,omitempty"`
	DraftReply  string     `json:"draft_reply,omitempty"`
	FoundAt     time.Time  `json:"found_at"`
}

type CrawlSource struct {
	ID           uuid.UUID  `json:"id"`
	WorkspaceID  uuid.UUID  `json:"workspace_id"`
	BrandID      *uuid.UUID `json:"brand_id,omitempty"`
	Kind         string     `json:"kind"`
	URL          string     `json:"url"`
	Query        string     `json:"query"`
	Purpose      string     `json:"purpose"`
	IntervalSec  int        `json:"interval_sec"`
	Enabled      bool       `json:"enabled"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
}

type AIAgent struct {
	ID           uuid.UUID       `json:"id"`
	WorkspaceID  uuid.UUID       `json:"workspace_id"`
	Name         string          `json:"name"`
	Role         AgentRole       `json:"role"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	BaseURL      string          `json:"base_url"`
	APIKeyEnv    string          `json:"api_key_env"`
	SystemPrompt string          `json:"system_prompt"`
	Params       json.RawMessage `json:"params"`
	Enabled      bool            `json:"enabled"`
}

type AIRun struct {
	ID           uuid.UUID       `json:"id"`
	WorkspaceID  uuid.UUID       `json:"workspace_id"`
	AgentID      uuid.UUID       `json:"agent_id"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output"`
	Status       string          `json:"status"`
	TokensIn     int             `json:"tokens_in"`
	TokensOut    int             `json:"tokens_out"`
	ErrorMessage string          `json:"error_message,omitempty"`
}

type GraphNode struct {
	ID    string         `json:"id"`
	Type  string         `json:"type"`
	Label string         `json:"label"`
	Meta  map[string]any `json:"meta,omitempty"`
}

type GraphEdge struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Weight float64 `json:"weight"`
}

type CampaignGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type EngagementKind string

const (
	EngageLike         EngagementKind = "like"
	EngageComment      EngagementKind = "comment"
	EngageFollow       EngagementKind = "follow"
	EngageShare        EngagementKind = "share"
	EngageLinkExchange EngagementKind = "link_exchange"
)

type EngagementStatus string

const (
	EngageDraft     EngagementStatus = "draft"
	EngagePending   EngagementStatus = "pending_approval"
	EngageApproved  EngagementStatus = "approved"
	EngageExecuting EngagementStatus = "executing"
	EngageDone      EngagementStatus = "done"
	EngageFailed    EngagementStatus = "failed"
	EngageSkipped   EngagementStatus = "skipped"
	EngageManual    EngagementStatus = "needs_manual"
)

type EngagementTask struct {
	ID               uuid.UUID         `json:"id"`
	WorkspaceID      uuid.UUID         `json:"workspace_id"`
	BrandID          *uuid.UUID        `json:"brand_id,omitempty"`
	ChannelID        uuid.UUID         `json:"channel_id"`
	PartnerID        *uuid.UUID        `json:"partner_id,omitempty"`
	MentionID        *uuid.UUID        `json:"mention_id,omitempty"`
	Kind             EngagementKind    `json:"kind"`
	Status           EngagementStatus  `json:"status"`
	TargetURL        string            `json:"target_url"`
	TargetExternalID string            `json:"target_external_id"`
	TargetTitle      string            `json:"target_title"`
	DraftText        string            `json:"draft_text"`
	ResultURL        string            `json:"result_url"`
	Points           int               `json:"points"`
	ErrorMessage     string            `json:"error_message"`
	ScheduledAt      *time.Time        `json:"scheduled_at,omitempty"`
	ExecutedAt       *time.Time        `json:"executed_at,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
}

type LinkPartner struct {
	ID          uuid.UUID  `json:"id"`
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	BrandID     *uuid.UUID `json:"brand_id,omitempty"`
	Name        string     `json:"name"`
	Platform    string     `json:"platform"`
	TheirURL    string     `json:"their_url"`
	OurURL      string     `json:"our_url"`
	Contact     string     `json:"contact"`
	Status      string     `json:"status"`
	Notes       string     `json:"notes"`
	CreatedAt   time.Time  `json:"created_at"`
}

type KarmaSummary struct {
	TotalPoints     int            `json:"total_points"`
	TodayPoints     int            `json:"today_points"`
	ByKind          map[string]int `json:"by_kind"`
	TodayByKind     map[string]int `json:"today_by_kind"`
	TodayTaskCounts map[string]int `json:"today_task_counts"`
}

type Contact struct {
	ID          uuid.UUID         `json:"id"`
	WorkspaceID uuid.UUID         `json:"workspace_id"`
	Email       string            `json:"email"`
	Phone       string            `json:"phone"`
	Name        string            `json:"name"`
	Tags        []string          `json:"tags"`
	Status      string            `json:"status"`
	Attributes  map[string]string `json:"attributes"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
