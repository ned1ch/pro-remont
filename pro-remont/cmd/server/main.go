package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pro-remont/internal/api"
	"pro-remont/internal/auth"
	"pro-remont/internal/config"
	"pro-remont/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(os.Getenv("LOG_LEVEL")),
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		logger.Error("failed to apply migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("database is ready")

	if cfg.SeedDemo {
		if err := st.Seed(ctx); err != nil {
			logger.Error("failed to seed demo data", "error", err)
			os.Exit(1)
		}
	}

	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.TokenTTL)
	srv := api.New(st.Pool, cfg, tokens, logger)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server started", "addr", cfg.Addr, "web_dir", cfg.WebDir)
		logger.Info("demo accounts",
			"admin", "admin@pro-remont.ru / Admin123!",
			"manager", "manager@pro-remont.ru / Manager123!",
			"master", "master1@pro-remont.ru / Master123!",
			"client", "client@example.ru / Client123!",
		)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		logger.Error("server error", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		_ = httpSrv.Close()
	}
	logger.Info("server stopped")
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
