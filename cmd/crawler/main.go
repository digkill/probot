package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/config"
	"github.com/digkill/probot/internal/crawler"
	"github.com/digkill/probot/internal/queue"
	"github.com/digkill/probot/internal/service"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
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

	runner := crawler.NewRunner(st)
	researcher := &service.Researcher{
		Store:  st,
		AI:     ai.NewClient(cfg),
		Runner: runner,
	}
	redisOpt, err := queue.ParseRedisOpt(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}

	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	scheduler := asynq.NewScheduler(redisOpt, nil)
	if _, err := scheduler.Register("@every 30m", queue.NewCrawlDueTask()); err != nil {
		log.Fatalf("schedule crawl: %v", err)
	}
	go func() {
		log.Printf("PRobot crawler scheduler started")
		if err := scheduler.Run(); err != nil {
			log.Printf("crawler scheduler stopped: %v", err)
		}
	}()

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeCrawlSource, func(ctx context.Context, t *asynq.Task) error {
		var p queue.CrawlPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		if p.Kind == "ai_search" && p.BrandID != nil {
			task, err := queue.NewAIResearchTask(queue.AIResearchPayload{
				WorkspaceID: p.WorkspaceID,
				BrandID:     *p.BrandID,
				SourceID:    p.SourceID,
			})
			if err != nil {
				return err
			}
			if _, err := asynqClient.Enqueue(task); err != nil {
				return err
			}
			_ = st.TouchCrawlSource(ctx, p.SourceID)
			log.Printf("crawl %s: enqueued AI research", p.SourceID)
			return nil
		}
		n, err := runner.RunWatch(ctx, p.WorkspaceID, p.BrandID, p.Kind, p.URL, p.Query, p.Purpose)
		_ = st.TouchCrawlSource(ctx, p.SourceID)
		if err != nil {
			return err
		}
		log.Printf("crawl %s: upserted %d mentions", p.SourceID, n)
		return nil
	})
	mux.HandleFunc(queue.TypeCrawlDue, func(ctx context.Context, _ *asynq.Task) error {
		sources, err := st.ListDueCrawlSources(ctx, 40)
		if err != nil {
			return err
		}
		for _, src := range sources {
			task, err := queue.NewCrawlTask(queue.CrawlPayload{
				SourceID:    src.ID,
				WorkspaceID: src.WorkspaceID,
				BrandID:     src.BrandID,
				URL:         src.URL,
				Kind:        src.Kind,
				Query:       src.Query,
				Purpose:     src.Purpose,
			})
			if err != nil {
				log.Printf("crawl due encode %s: %v", src.ID, err)
				continue
			}
			if _, err := asynqClient.Enqueue(task); err != nil {
				log.Printf("crawl due enqueue %s: %v", src.ID, err)
			}
		}
		log.Printf("crawl due: enqueued %d sources", len(sources))
		return nil
	})
	mux.HandleFunc(queue.TypeAIResearch, func(ctx context.Context, t *asynq.Task) error {
		var p queue.AIResearchPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		res, err := researcher.Run(ctx, p.WorkspaceID, p.BrandID)
		if p.SourceID != uuid.Nil {
			_ = st.TouchCrawlSource(ctx, p.SourceID)
		}
		log.Printf("ai research %s: hits=%d mentions=%d agents=%v err=%v", p.BrandID, res.Hits, res.Mentions, res.Agents, res.Errors)
		return err
	})

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 4,
		Queues:      map[string]int{"default": 5},
		RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
			return time.Duration(n) * 20 * time.Second
		},
	})
	log.Printf("PRobot crawler started")
	if err := srv.Run(mux); err != nil {
		log.Fatalf("crawler: %v", err)
	}
}
