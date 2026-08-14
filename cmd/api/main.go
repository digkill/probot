package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/digkill/probot/internal/catalog"
	"github.com/digkill/probot/internal/config"
	httpapi "github.com/digkill/probot/internal/http"
	"github.com/digkill/probot/internal/platformreg"
	"github.com/digkill/probot/internal/queue"
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

	if err := catalog.SeedPlatforms(ctx, st); err != nil {
		log.Fatalf("seed platforms: %v", err)
	}

	redisOpt, err := queue.ParseRedisOpt(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	registry := platformreg.Default()

	srv := httpapi.NewServer(cfg, st, registry, asynqClient)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("PRobot API listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
