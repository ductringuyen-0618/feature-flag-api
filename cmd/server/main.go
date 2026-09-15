package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ductringuyen-0618/feature-flag-api/internal/config"
	"github.com/ductringuyen-0618/feature-flag-api/internal/httpapi"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/cached"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/postgres"
	redissync "github.com/ductringuyen-0618/feature-flag-api/internal/sync/redis"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	var syncClient *redissync.Sync
	syncClient, err = redissync.New(ctx, cfg.RedisURL)
	if err != nil {
		slog.Warn("redis unavailable; running without pub/sub", "err", err)
		syncClient = nil
	} else {
		defer syncClient.Close()
	}

	svc := cached.New(db, syncClient, cfg.ReloadInterval)
	if err := svc.Start(ctx); err != nil {
		slog.Error("service start", "err", err)
		os.Exit(1)
	}

	handler := httpapi.New(svc)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
