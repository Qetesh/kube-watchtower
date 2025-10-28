package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/qetesh/kubewatchtower/pkg/config"
	"github.com/qetesh/kubewatchtower/pkg/logger"
	"github.com/qetesh/kubewatchtower/pkg/watcher"
)

const version = "1.0.0"

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize logger
	if err := logger.Init(cfg.LogLevel); err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	// Print version (like watchtower)
	logger.Infof("KubeWatchtower %s", version)

	// Debug configuration
	logger.Debugf("Configuration loaded: Namespace=%s, CheckInterval=%v, Cleanup=%v",
		cfg.Namespace, cfg.CheckInterval, cfg.Cleanup)

	// Create watcher
	w, err := watcher.NewWatcher(cfg)
	if err != nil {
		logger.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Run watcher
	if err := w.Run(ctx); err != nil && err != context.Canceled {
		logger.Fatalf("Watcher failed: %v", err)
	}

	logger.Info("KubeWatchtower stopped")
}
