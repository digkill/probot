package main

import (
	"context"
	"log"
	"net"
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
	"github.com/digkill/probot/internal/service"
	"github.com/digkill/probot/internal/store"
	tgclient "github.com/digkill/probot/internal/telegram"
	"github.com/hibiken/asynq"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, cancelRoot := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelRoot()
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

	var telegramManager *tgclient.AccountManager
	var telegramService *service.TelegramService
	if cfg.TelegramClient.Enabled {
		tc := cfg.TelegramClient
		repo, err := store.NewTelegramStore(st.Pool, cfg.EncryptionKey)
		if err != nil {
			log.Fatal("telegram: invalid storage configuration")
		}
		factory, err := tgclient.NewFactory(tgclient.Config{AppID: tc.AppID, AppHash: tc.AppHash, EncryptionKey: cfg.EncryptionKey, RPCRate: tc.RPCRate, MaxAccounts: tc.MaxAccounts, AuthTTL: tc.AuthTTL}, repo, repo, log.Default())
		if err != nil {
			log.Fatal("telegram: invalid client configuration")
		}
		telegramManager, err = tgclient.NewAccountManager(ctx, repo, factory, log.Default(), tc.MaxAccounts)
		if err != nil {
			log.Fatal("telegram: invalid manager configuration")
		}
		telegramService, err = service.NewTelegramService(repo, telegramManager)
		if err != nil {
			log.Fatal("telegram: invalid service configuration")
		}
		if err := telegramManager.Restore(ctx); err != nil {
			log.Print("component=telegram operation=restore status=failed")
		}
	}
	srv := httpapi.NewServer(cfg, st, registry, asynqClient, telegramService)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("PRobot API listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	if telegramManager != nil {
		if err := telegramManager.Stop(shutdownCtx); err != nil {
			log.Print("component=telegram operation=shutdown status=timeout")
		}
	}
}
