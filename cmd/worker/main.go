package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/config"
	"github.com/digkill/probot/internal/crawler"
	"github.com/digkill/probot/internal/platformreg"
	"github.com/digkill/probot/internal/platforms/generic"
	"github.com/digkill/probot/internal/queue"
	"github.com/digkill/probot/internal/service"
	"github.com/digkill/probot/internal/store"
	"github.com/hibiken/asynq"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	registry := platformreg.Default()
	publisher := &service.Publisher{
		Store:         st,
		Registry:      registry,
		Generic:       generic.NewPublisher(),
		EncKey:        []byte(cfg.EncryptionKey),
		PublicBaseURL: cfg.PublicBaseURL,
	}
	stats := &service.StatsPoller{
		Store:    st,
		Registry: registry,
		EncKey:   []byte(cfg.EncryptionKey),
	}

	redisOpt, err := queue.ParseRedisOpt(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}

	scheduler := asynq.NewScheduler(redisOpt, nil)
	if _, err := scheduler.Register("@every 15m", queue.NewPollAllStatsTask()); err != nil {
		log.Fatalf("schedule stats: %v", err)
	}
	go func() {
		log.Printf("PRobot scheduler started")
		if err := scheduler.Run(); err != nil {
			log.Printf("scheduler stopped: %v", err)
		}
	}()

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypePublishPublication, func(ctx context.Context, t *asynq.Task) error {
		var p queue.PublishPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		log.Printf("publishing %s", p.PublicationID)
		return publisher.PublishPublication(ctx, p.PublicationID)
	})
	mux.HandleFunc(queue.TypePollAllStats, func(ctx context.Context, _ *asynq.Task) error {
		n, err := stats.PollAll(ctx)
		log.Printf("stats poll: updated %d publications", n)
		return err
	})
	mux.HandleFunc(queue.TypeFetchPubStats, func(ctx context.Context, t *asynq.Task) error {
		var p queue.PublishPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		return stats.FetchOne(ctx, p.PublicationID)
	})
	engager := &service.Engager{
		Store:    st,
		Registry: registry,
		EncKey:   []byte(cfg.EncryptionKey),
	}
	mux.HandleFunc(queue.TypeEngageTask, func(ctx context.Context, t *asynq.Task) error {
		var p queue.EngagePayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		log.Printf("engage %s", p.TaskID)
		return engager.Execute(ctx, p.TaskID)
	})
	researcher := &service.Researcher{
		Store:  st,
		AI:     ai.NewClient(cfg),
		Runner: crawler.NewRunner(st),
	}
	mux.HandleFunc(queue.TypeAIResearch, func(ctx context.Context, t *asynq.Task) error {
		var p queue.AIResearchPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		res, err := researcher.Run(ctx, p.WorkspaceID, p.BrandID)
		log.Printf("ai research %s: hits=%d mentions=%d agents=%v errors=%v", p.BrandID, res.Hits, res.Mentions, res.Agents, res.Errors)
		return err
	})

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 8,
		Queues:      map[string]int{"default": 10},
		RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
			return time.Duration(n) * 15 * time.Second
		},
	})
	log.Printf("PRobot worker started")
	if err := srv.Run(mux); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
