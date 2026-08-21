package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/utkarsh/claim-identification/internal/claims"
	"github.com/utkarsh/claim-identification/internal/config"
	"github.com/utkarsh/claim-identification/internal/httpapi"
	"github.com/utkarsh/claim-identification/internal/seed"
	"github.com/utkarsh/claim-identification/internal/store"
	"github.com/utkarsh/claim-identification/internal/store/memory"
	"github.com/utkarsh/claim-identification/internal/store/postgres"
	"github.com/utkarsh/claim-identification/internal/workflow"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Error("close store", "error", err)
		}
	}()

	if _, err := seed.Load(ctx, st, seed.Options{
		File:             cfg.SeedFile,
		MediaDir:         cfg.MediaDir,
		PublicBaseURL:    cfg.PublicBaseURL,
		RewriteMediaURLs: cfg.RewriteMediaURLs,
	}, log); err != nil {
		if !errors.Is(err, seed.ErrNoSeedFile) {
			return err
		}
		log.Warn("no seed file, starting with an empty catalogue", "file", cfg.SeedFile)
	}

	detector := claims.New(claims.WithStructuredFields(cfg.IncludeStructuredClaims))

	engine := workflow.New(st, detector, workflow.Config{
		Workers:   cfg.Workers,
		QueueSize: cfg.QueueSize,
		Timeout:   cfg.WorkflowTimeout,
	}, log)
	engine.Start()

	api := httpapi.New(httpapi.Config{
		Engine:   engine,
		Store:    st,
		Logger:   log,
		MediaDir: cfg.MediaDir,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.Addr, "store", storeKind(cfg))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http server shutdown", "error", err)
	}
	if err := engine.Shutdown(shutdownCtx); err != nil {
		log.Error("workflow engine shutdown", "error", err)
	}

	log.Info("shutdown complete")
	return nil
}

func openStore(ctx context.Context, cfg config.Config, log *slog.Logger) (store.Store, error) {
	if !cfg.UsesPostgres() {
		log.Warn("DATABASE_URL not set, using in-memory store (data is lost on restart)")
		return memory.New(), nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	st, err := postgres.Connect(connectCtx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	log.Info("connected to postgres")
	return st, nil
}

func storeKind(cfg config.Config) string {
	if cfg.UsesPostgres() {
		return "postgres"
	}
	return "memory"
}
