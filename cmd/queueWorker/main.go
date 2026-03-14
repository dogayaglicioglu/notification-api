package main

import (
	"context"
	"log/slog"
	"net/http"
	"notif-api/cmd/queueWorker/handler"
	"notif-api/internal/config"
	"notif-api/internal/db"
	"notif-api/internal/logging"
	"notif-api/internal/provider"
	"notif-api/internal/shutdown"
	"notif-api/internal/worker"
	"os"
	"sync"
	"time"
)

func main() {
	logging.Init()
	cfg := config.LoadConfig()

	database, err := db.NewDatabaseConnection(cfg)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	notificationDB := db.NewNotificationDB(database)

	if cfg.ExternalProviderURL == "" {
		slog.Error("missing required environment variable", "name", "EXTERNAL_PROVIDER_URL")
		os.Exit(1)
	}
	notifProvider := provider.NewWebhookProvider(cfg.ExternalProviderURL)
	slog.Info("external provider configured", "url", cfg.ExternalProviderURL)

	queueWorker, err := worker.NewRedisQueueWorker(cfg.RedisAddr, cfg.RedisPassword, notificationDB, notifProvider, cfg.WorkerConcurrency, cfg.WorkerMaxRetry)
	if err != nil {
		slog.Error("failed to create queue worker", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	handler.SetupRoutes(mux)

	metricsAddr := ":" + cfg.MetricsPort
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: mux,
	}
	go func() {
		slog.Info("starting metrics server", "address", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		runErrCh <- queueWorker.Run(ctx)
	}()

	shutdownDone := make(chan struct{})
	go func() {
		shutdown.GracefulShutdown(ctx, &workerWG, cancel,
			func() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer shutdownCancel()
				if err := metricsServer.Shutdown(shutdownCtx); err != nil {
					slog.Warn("error shutting down metrics server", "error", err)
				}
			},
			func() {
				if err := queueWorker.Close(); err != nil {
					slog.Warn("error closing redis queue worker connection", "error", err)
				}
			},
			func() {
				if err := database.Close(); err != nil {
					slog.Warn("error closing database", "error", err)
				}
			},
		)
		close(shutdownDone)
	}()

	err = <-runErrCh
	if err != nil {
		slog.Error("worker terminated with error", "error", err)
	}

	cancel()
	<-shutdownDone

	if err != nil {
		os.Exit(1)
	}

	slog.Info("worker stopped gracefully")
}
