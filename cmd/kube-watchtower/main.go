package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/qetesh/kube-watchtower/pkg/config"
	"github.com/qetesh/kube-watchtower/pkg/logger"
	"github.com/qetesh/kube-watchtower/pkg/watcher"
)

var version = "dev"

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize logger
	if err := logger.Init(cfg.LogLevel); err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	// Print version
	logger.Infof("kube-watchtower %s", version)

	// Debug configuration
	logger.Debugf("Configuration loaded: DisableNamespaces=%v",
		cfg.DisableNamespaces)

	// Create watcher
	w, err := watcher.NewWatcher(cfg)
	if err != nil {
		logger.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Signal handler goroutine
	done := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			logger.Infof("Received signal %v, shutting down...", sig)
			cancel()
		case <-ctx.Done():
			// Context cancelled by other means, exit gracefully
		}
		close(done)
	}()

	// Run watcher
	if err := w.Run(ctx); err != nil && err != context.Canceled {
		cancel()
		signal.Stop(sigCh)
		logger.Fatalf("Watcher failed: %v", err)
	}

	// Cancel context and wait for signal handler to exit
	cancel()
	signal.Stop(sigCh)
	<-done

	logger.Info("kube-watchtower stopped")
}
