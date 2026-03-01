package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aarush/uptime-monitor/internal/handler"
	"github.com/aarush/uptime-monitor/internal/monitor"
	"github.com/aarush/uptime-monitor/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	port := env("PORT", "8080")
	dbURL := env("DATABASE_URL", "postgres://uptime:uptime@localhost:5432/uptime?sslmode=disable")
	redisURL := env("REDIS_URL", "redis://localhost:6379")

	pg, err := store.NewPostgresStore(dbURL)
	if err != nil {
		slog.Error("postgres connection failed", "err", err)
		os.Exit(1)
	}
	defer pg.Close()
	slog.Info("connected to postgres")

	cache, err := store.NewCache(redisURL)
	if err != nil {
		slog.Error("redis connection failed", "err", err)
		os.Exit(1)
	}
	defer cache.Close()
	slog.Info("connected to redis")

	checker := monitor.NewChecker()
	scheduler := monitor.NewScheduler(pg, cache, checker)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := scheduler.Start(ctx); err != nil {
		slog.Error("scheduler start failed", "err", err)
		os.Exit(1)
	}

	router := handler.NewRouter(pg, cache, scheduler)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server started", "port", port, "url", "http://localhost:"+port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	scheduler.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "err", err)
	}

	slog.Info("server stopped")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
