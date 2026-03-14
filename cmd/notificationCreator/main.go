package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"notif-api/cmd/notificationCreator/handler"
	_ "notif-api/docs"
	"notif-api/internal/config"
	"notif-api/internal/db"
	"notif-api/internal/handlers"
	"notif-api/internal/logging"
	"notif-api/internal/middleware"
	"notif-api/internal/shutdown"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// @title Notification API
// @version 1.0
// @description Batch notification management and async processing API.
// @BasePath /
func main() {
	logging.Init()
	cfg := config.LoadConfig()

	database, err := db.NewDatabaseConnection(cfg)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	notificationDB := db.NewNotificationDB(database)
	outboxDB := db.NewOutboxDB(database)

	notificationHandler := handlers.NewNotificationHandlerWithOutbox(notificationDB, nil, outboxDB)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CorrelationID())

	handler.SetupRoutes(router, notificationHandler)

	server := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
	}

	ctx, cancel := context.WithCancel(context.Background())

	shutdownDone := make(chan struct{})
	go func() {
		shutdown.GracefulShutdown(ctx, nil, cancel,
			func() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer shutdownCancel()
				if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
					slog.Warn("error shutting down API server", "error", err)
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

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting API server", "port", cfg.ServerPort)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	if err := <-errCh; err != nil {
		slog.Error("failed to start or run server", "error", err)
		cancel()
		<-shutdownDone
		os.Exit(1)
	}

	cancel()
	<-shutdownDone
	slog.Info("API server shutdown completed")
}
