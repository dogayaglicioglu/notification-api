package shutdown

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func GracefulShutdown(ctx context.Context, wg *sync.WaitGroup, cancel context.CancelFunc, cleanupActions ...func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	select {
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig.String())
	case <-ctx.Done():
		slog.Info("shutdown requested from context")
	}

	cancel()

	if wg != nil {
		wg.Wait()
	}

	for _, action := range cleanupActions {
		if action != nil {
			action()
		}
	}
}
