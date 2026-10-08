// Command api runs the atelier HTTP API.
//
// Usage:
//
//	api                 serve HTTP (applies migrations first when AUTO_MIGRATE=true)
//	api migrate         apply pending migrations
//	api rollback        roll back the most recent migration
//	api seed            load reference catalog data (garments, measurement fields, fit rules)
//	api seed-dev        load development-only demo content (refused in production)
//	api bootstrap-owner create the first owner account from BOOTSTRAP_OWNER_* variables
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/seed"
	"github.com/kuyamcliff/tailor-website/backend/internal/server"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
	"github.com/kuyamcliff/tailor-website/backend/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("service", "atelier-api", "env", string(cfg.Env))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "migrate":
		return db.Migrate(ctx, pool, migrations.FS, log)
	case "rollback":
		return db.Rollback(ctx, pool, migrations.FS, log)
	case "seed":
		return seed.Reference(ctx, pool)
	case "seed-dev":
		if cfg.IsProduction() {
			return errors.New("seed-dev is refused in production")
		}
		if err := seed.Reference(ctx, pool); err != nil {
			return err
		}
		store, err := uploads.NewStorage(cfg.Storage)
		if err != nil {
			return err
		}
		return seed.Development(ctx, pool, store)
	case "bootstrap-owner":
		return seed.BootstrapOwner(ctx, pool, cfg.BootstrapOwnerEmail, cfg.BootstrapOwnerPassword, cfg.BootstrapOwnerName)
	case "serve":
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}

	if cfg.AutoMigrate {
		if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
			return err
		}
	}

	app, err := server.New(ctx, cfg, pool, log)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // uploads
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 16,
	}
	app.StartBackground(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}
