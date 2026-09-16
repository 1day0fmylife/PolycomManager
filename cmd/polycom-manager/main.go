package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"polycom-manager/internal/app"
	"polycom-manager/internal/config"
)

var version = "dev"

func main() {
	cfg := config.Load()
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("version", version)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- a.Run() }()
	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
	case err := <-errCh:
		if err != nil {
			log.Error("server stopped", "error", err)
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()
	if err := a.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown failed", "error", err)
	}
}
