package queue

import (
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/google/uuid"
)

const (
	TypePublishPublication = "publish:publication"
	TypeCrawlSource        = "crawl:source"
	TypeAIRun              = "ai:run"
	TypePollAllStats       = "stats:poll_all"
	TypeFetchPubStats      = "stats:publication"
	TypeEngageTask         = "engage:task"
	TypeCrawlDue           = "crawl:due"
	TypeAIResearch         = "ai:research"
)

type PublishPayload struct {
	PublicationID uuid.UUID `json:"publication_id"`
}

type CrawlPayload struct {
	SourceID    uuid.UUID  `json:"source_id"`
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	BrandID     *uuid.UUID `json:"brand_id,omitempty"`
	URL         string     `json:"url"`
	Kind        string     `json:"kind"`
	Query       string     `json:"query"`
	Purpose     string     `json:"purpose,omitempty"`
}

type AIRunPayload struct {
	RunID uuid.UUID `json:"run_id"`
}

func NewPublishTask(publicationID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(PublishPayload{PublicationID: publicationID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypePublishPublication, payload), nil
}

func NewCrawlTask(p CrawlPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeCrawlSource, payload), nil
}

func NewPollAllStatsTask() *asynq.Task {
	return asynq.NewTask(TypePollAllStats, []byte(`{}`))
}

func NewFetchPubStatsTask(publicationID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(PublishPayload{PublicationID: publicationID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeFetchPubStats, payload), nil
}

type EngagePayload struct {
	TaskID uuid.UUID `json:"task_id"`
}

func NewEngageTask(taskID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(EngagePayload{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeEngageTask, payload), nil
}

func NewCrawlDueTask() *asynq.Task {
	return asynq.NewTask(TypeCrawlDue, []byte(`{}`))
}

type AIResearchPayload struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	BrandID     uuid.UUID `json:"brand_id"`
	SourceID    uuid.UUID `json:"source_id,omitempty"`
}

func NewAIResearchTask(p AIResearchPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeAIResearch, payload), nil
}

func ParseRedisOpt(redisURL string) (asynq.RedisClientOpt, error) {
	// asynq accepts host:port; parse redis://localhost:6379/0 simply
	opt := asynq.RedisClientOpt{Addr: "localhost:6379"}
	if redisURL == "" {
		return opt, nil
	}
	// Minimal parse: redis://host:port/db
	u := redisURL
	if len(u) > 8 && u[:8] == "redis://" {
		u = u[8:]
	}
	host := u
	if i := indexByte(u, '/'); i >= 0 {
		host = u[:i]
	}
	if host != "" {
		opt.Addr = host
	}
	return opt, nil
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
