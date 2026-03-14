package main

import (
	"context"
	"log/slog"
	"notif-api/internal/config"
	"notif-api/internal/db"
	"notif-api/internal/logging"
	"notif-api/internal/publisher"
	"notif-api/internal/shutdown"
	"os"
	"sync"
)

func main() {
	logging.Init()
	cfg := config.LoadConfig()

	database, err := db.NewDatabaseConnection(cfg)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	outboxDB := db.NewOutboxDB(database)
	notificationDB := db.NewNotificationDB(database)

	outboxPublisher, err := publisher.NewOutboxPublisher(
		cfg.RedisAddr,
		cfg.RedisPassword,
		outboxDB,
		notificationDB,
		3,
		100,
		10,
	)
	if err != nil {
		slog.Error("failed to create outbox publisher", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		runErrCh <- outboxPublisher.Run(ctx)
	}()

	shutdownDone := make(chan struct{})
	go func() {
		shutdown.GracefulShutdown(ctx, &workerWG, cancel,
			func() {
				if err := outboxPublisher.Close(); err != nil {
					slog.Warn("error closing outbox publisher", "error", err)
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
		slog.Error("outbox publisher error", "error", err)
	}

	cancel()
	<-shutdownDone

	if err != nil {
		os.Exit(1)
	}

	slog.Info("outbox publisher shutdown completed")
}
