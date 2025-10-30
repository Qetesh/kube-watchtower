package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/qetesh/kube-watchtower/pkg/config"
	"github.com/qetesh/kube-watchtower/pkg/k8s"
	"github.com/qetesh/kube-watchtower/pkg/logger"
	"github.com/qetesh/kube-watchtower/pkg/notifier"
	"github.com/qetesh/kube-watchtower/pkg/registry"
)

// Watcher monitors and updates container images
type Watcher struct {
	config       *config.Config
	k8sClient    *k8s.Client
	imageChecker *registry.ImageChecker
	notifier     *notifier.Notifier
}

// NewWatcher creates a new watcher
func NewWatcher(cfg *config.Config) (*Watcher, error) {
	k8sClient, err := k8s.NewClient(cfg.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	imageChecker, err := registry.NewImageChecker()
	if err != nil {
		return nil, fmt.Errorf("failed to create image checker: %w", err)
	}

	notif := notifier.NewNotifier(cfg.NotificationURL)

	return &Watcher{
		config:       cfg,
		k8sClient:    k8sClient,
		imageChecker: imageChecker,
		notifier:     notif,
	}, nil
}

// Run runs the watcher
func (w *Watcher) Run(ctx context.Context) error {
	logger.Info("Checking all containers (except explicitly disabled with label)")

	// Schedule first run
	if !w.config.RunOnce {
		nextRun := calculateNextRun(w.config.CheckInterval)
		logger.Infof("Scheduling first run: %s", nextRun.Format("2006-01-02 15:04:05 MST"))

		duration := time.Until(nextRun)
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60
		logger.Infof("Note that the first check will be performed in %d hours, %d minutes, %d seconds", hours, minutes, seconds)

		// Wait until first run
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(duration):
		}
	}

	// Run initial check
	if err := w.check(ctx); err != nil {
		logger.Errorf("Initial check failed: %v", err)
	}

	if w.config.RunOnce {
		logger.Debug("Run once mode, exiting")
		return nil
	}

	// Use ticker for continuous checking
	ticker := time.NewTicker(w.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Received shutdown signal, stopping watcher")
			return ctx.Err()
		case <-ticker.C:
			if err := w.check(ctx); err != nil {
				logger.Errorf("Check failed: %v", err)
			}
		}
	}
}

// calculateNextRun calculates the next run time
func calculateNextRun(interval time.Duration) time.Time {
	now := time.Now()
	// Round to next midnight
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return midnight
}

// check performs one check cycle
func (w *Watcher) check(ctx context.Context) error {
	logger.Debug("Starting image update check...")

	// List all workloads (Deployments, DaemonSets, StatefulSets)
	workloads, err := w.k8sClient.ListWorkloads(ctx)
	if err != nil {
		return fmt.Errorf("failed to list workloads: %w", err)
	}

	logger.Debugf("Found %d workloads to monitor", len(workloads))

	updatedCount := 0
	failedCount := 0
	scannedCount := 0

	// Check each workload
	for _, workload := range workloads {
		for _, container := range workload.Containers {
			// Check if container is disabled
			if w.config.IsContainerDisabled(container.Name) {
				logger.Debugf("Skipping disabled container: %s/%s/%s (%s)", workload.Namespace, workload.Name, container.Name, workload.Type)
				continue
			}

			// Only monitor latest tag
			if container.Tag != "latest" {
				logger.Debugf("Skipping non-latest tag: %s/%s/%s (tag: %s)", workload.Namespace, workload.Name, container.Name, container.Tag)
				continue
			}

			scannedCount++

			logger.Debugf("Checking container: %s/%s/%s (%s)", workload.Namespace, workload.Name, container.Name, workload.Type)
			logger.Debugf("  Image: %s", container.Image)
			logger.Debugf("  Current Digest: %s", container.CurrentDigest)

			// Check for updates
			hasUpdate, newDigest, err := w.imageChecker.CheckForUpdate(ctx, container.Image, nil)
			if err != nil {
				logger.Errorf("Failed to check image update for %s/%s/%s: %v", workload.Namespace, workload.Name, container.Name, err)
				failedCount++
				continue
			}

			logger.Debugf("  Remote Digest: %s", newDigest)

			// If we have current digest, use it for comparison
			if container.CurrentDigest != "" {
				if container.CurrentDigest == newDigest {
					logger.Debugf("No update needed: %s/%s/%s (digest matches)", workload.Namespace, workload.Name, container.Name)
					continue
				}
				hasUpdate = true
			}

			if !hasUpdate {
				logger.Debugf("No update needed: %s/%s/%s", workload.Namespace, workload.Name, container.Name)
				continue
			}

			// Log new image found (like watchtower)
			imageInfo := registry.ParseImage(container.Image)
			logger.Infof("Found new %s:%s image (%s)", imageInfo.Repository, imageInfo.Tag, newDigest[:12])

			// Perform update
			if err := w.updateContainer(ctx, workload, container, newDigest); err != nil {
				logger.Errorf("Update failed: %v", err)
				w.notifier.NotifyUpdateFailure(workload.Namespace, workload.Name, container.Name, container.Image, err)
				failedCount++
				continue
			}

			updatedCount++
			logger.Debugf("Update successful: %s/%s/%s", workload.Namespace, workload.Name, container.Name)
			w.notifier.NotifyUpdateSuccess(workload.Namespace, workload.Name, container.Name, container.Image)
		}
	}

	// Session done (like watchtower)
	notifyStatus := "no"
	if w.notifier != nil {
		notifyStatus = "yes"
	}
	logger.Infof("Session done Failed=%d Scanned=%d Updated=%d notify=%s", failedCount, scannedCount, updatedCount, notifyStatus)

	if w.notifier != nil {
		w.notifier.NotifyCheckCompleted(updatedCount, scannedCount)
	}

	return nil
}

// updateContainer updates a container in a workload
func (w *Watcher) updateContainer(ctx context.Context, workload k8s.WorkloadInfo, container k8s.ContainerInfo, newDigest string) error {
	// Build new image name
	imageInfo := registry.ParseImage(container.Image)
	newImage := fmt.Sprintf("%s:%s@%s", imageInfo.Repository, imageInfo.Tag, newDigest)

	logger.Debugf("Updating image: %s -> %s", container.Image, newImage)
	w.notifier.NotifyUpdateStarted(workload.Namespace, workload.Name, container.Name, container.Image, newImage)

	// Log stopping container (like watchtower)
	logger.Infof("Stopping /%s/%s (container: %s) with SIGTERM", workload.Namespace, workload.Name, container.Name)

	// Update workload
	err := w.k8sClient.UpdateWorkloadImage(ctx, workload.Type, workload.Namespace, workload.Name, container.Name, newImage)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", workload.Type, err)
	}

	// Log creating container (like watchtower)
	logger.Infof("Creating /%s/%s", workload.Namespace, workload.Name)

	// Wait for rollout to complete
	logger.Debugf("Waiting for rollout to complete: %s/%s (%s)", workload.Namespace, workload.Name, workload.Type)
	err = w.k8sClient.WaitForRollout(ctx, workload.Type, workload.Namespace, workload.Name, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("rollout failed: %w", err)
	}

	// Cleanup old resources
	if w.config.Cleanup {
		logger.Debugf("Cleaning up old resources: %s/%s (%s)", workload.Namespace, workload.Name, workload.Type)
		// Log removing old resources (like watchtower logs "Removing image")
		logger.Infof("Removing old resources for %s/%s", workload.Namespace, workload.Name)
		err = w.k8sClient.CleanupOldResources(ctx, workload.Type, workload.Namespace, workload.Name)
		if err != nil {
			logger.Warnf("Cleanup warning: %v", err)
		}
	}

	logger.Debugf("Update completed: %s/%s/%s (%s)", workload.Namespace, workload.Name, container.Name, workload.Type)
	return nil
}

// Close closes the watcher
func (w *Watcher) Close() error {
	if w.imageChecker != nil {
		return w.imageChecker.Close()
	}
	return nil
}
