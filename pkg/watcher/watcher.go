package watcher

import (
	"context"
	"fmt"
	"strings"
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
	k8sClient, err := k8s.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	imageChecker, err := registry.NewImageChecker()
	if err != nil {
		return nil, fmt.Errorf("failed to create image checker: %w", err)
	}

	notif := notifier.NewNotifier(cfg.NotificationURL, cfg.NotificationCluster, cfg.DryRun)

	return &Watcher{
		config:       cfg,
		k8sClient:    k8sClient,
		imageChecker: imageChecker,
		notifier:     notif,
	}, nil
}

// Run runs the watcher
func (w *Watcher) Run(ctx context.Context) error {
	return w.check(ctx)
}

// workloadKey generates a unique key for a workload
func workloadKey(workload k8s.WorkloadInfo) string {
	return fmt.Sprintf("%s/%s/%s", workload.Type, workload.Namespace, workload.Name)
}

// check performs one check cycle
func (w *Watcher) check(ctx context.Context) error {
	logger.Debug("Starting image update check...")

	// Reset notifier results for this check cycle
	if w.notifier != nil {
		w.notifier.Reset()
	}

	// List all workloads (Deployments, DaemonSets, StatefulSets)
	workloads, err := w.k8sClient.ListWorkloads(ctx, w.config)
	if err != nil {
		return fmt.Errorf("failed to list workloads: %w", err)
	}

	logger.Debugf("Found %d workloads to monitor", len(workloads))

	scannedCount := 0
	failedCount := 0
	updatedCount := 0

	// Track workloads already processed in this cycle (restarted or failed)
	processedWorkloads := make(map[string]bool)
	// Track workloads that failed to restart
	failedWorkloads := make(map[string]bool)

	// Check each workload and restart immediately when update is detected
	for _, workload := range workloads {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			logger.Info("Check interrupted by context cancellation")
			return ctx.Err()
		default:
		}

		key := workloadKey(workload)

		for _, container := range workload.Containers {
			scannedCount++

			logger.Debugf("Checking container: %s/%s/%s (%s)", workload.Namespace, workload.Name, container.Name, workload.Type)
			logger.Debugf("  Image: %s", container.Image)
			logger.Debugf("  Current Digest: %s", container.CurrentDigest)

			// Skip if current digest is empty (unable to get from pod status)
			// This prevents unnecessary restarts when digest cannot be determined
			if container.CurrentDigest == "" {
				logger.Warnf("Skipping %s/%s/%s: unable to get current digest from pod status", workload.Namespace, workload.Name, container.Name)
				continue
			}

			// Get registry credentials if imagePullSecrets are defined
			var credentials *registry.RegistryCredentials
			if len(workload.ImagePullSecrets) > 0 {
				logger.Debugf("  ImagePullSecrets found: %v", workload.ImagePullSecrets)
				credentials = w.getCredentialsForImage(ctx, workload.Namespace, workload.ImagePullSecrets, container.Image)
			}

			// Get remote digest
			newDigest, err := w.imageChecker.GetRemoteDigest(ctx, container.Image, credentials)
			if err != nil {
				logger.Errorf("Failed to check image update for %s/%s/%s: %v", workload.Namespace, workload.Name, container.Name, err)
				if w.notifier != nil {
					w.notifier.AddResult(workload.Namespace, container.Image, false, err)
				}
				failedCount++
				continue
			}

			logger.Debugf("  Remote Digest: %s", newDigest)

			// Compare digests — skip if unchanged
			if container.CurrentDigest == newDigest {
				logger.Debugf("No update needed: %s/%s/%s (digest matches)", workload.Namespace, workload.Name, container.Name)
				continue
			}

			// Log new image found (with safe digest display)
			imageInfo := registry.ParseImage(container.Image)
			digestDisplay := newDigest
			if len(newDigest) > 12 {
				digestDisplay = newDigest[:12]
			}
			logger.Infof("Found new %s:%s image (%s)", imageInfo.Repository, imageInfo.Tag, digestDisplay)

			// Skip if workload already failed to restart in this cycle
			if failedWorkloads[key] {
				logger.Debugf("Workload %s already failed to restart, skipping", key)
				failedCount++
				if w.notifier != nil {
					w.notifier.AddResult(workload.Namespace, container.Image, false, fmt.Errorf("workload restart previously failed"))
				}
				continue
			}

			// Skip if workload was already restarted in this cycle
			if processedWorkloads[key] {
				logger.Debugf("Workload %s already restarted in this cycle, skipping duplicate restart", key)
				updatedCount++
				if w.notifier != nil {
					w.notifier.AddResult(workload.Namespace, container.Image, true, nil)
				}
				continue
			}

			// Restart workload immediately
			if w.config.DryRun {
				logger.Infof("[DRY-RUN] Would restart %s %s/%s", workload.Type, workload.Namespace, workload.Name)
				processedWorkloads[key] = true
				updatedCount++
				if w.notifier != nil {
					w.notifier.AddResult(workload.Namespace, container.Image, true, nil)
				}
			} else {
				if err := w.restartWorkload(ctx, workload); err != nil {
					logger.Errorf("Failed to restart %s %s/%s: %v", workload.Type, workload.Namespace, workload.Name, err)
					processedWorkloads[key] = true
					failedWorkloads[key] = true
					failedCount++
					if w.notifier != nil {
						w.notifier.AddResult(workload.Namespace, container.Image, false, err)
					}
					continue
				}

				processedWorkloads[key] = true
				updatedCount++
				if w.notifier != nil {
					w.notifier.AddResult(workload.Namespace, container.Image, true, nil)
				}
			}
		}
	}

	// Session done (like watchtower)
	if w.config.DryRun {
		logger.Infof("[DRY-RUN] Session done Scanned=%d Detected=%d Failed=%d", scannedCount, updatedCount, failedCount)
	} else {
		logger.Infof("Session done Scanned=%d Updated=%d Failed=%d", scannedCount, updatedCount, failedCount)
	}

	// Send summary notification
	if w.notifier != nil {
		w.notifier.SendSummary(scannedCount)
	}

	return nil
}

// restartWorkload triggers a rolling restart of a workload to pull new images
// Since containers use imagePullPolicy: Always, a rollout restart will pull the latest images
func (w *Watcher) restartWorkload(ctx context.Context, workload k8s.WorkloadInfo) error {
	logger.Infof("Restarting %s %s/%s", workload.Type, workload.Namespace, workload.Name)

	// Trigger rollout restart using the same mechanism as "kubectl rollout restart"
	err := w.k8sClient.RolloutRestart(ctx, workload.Type, workload.Namespace, workload.Name)
	if err != nil {
		return fmt.Errorf("failed to restart %s: %w", workload.Type, err)
	}

	// Wait for rollout to complete
	logger.Infof("Waiting for rollout to complete: %s/%s (%s)", workload.Namespace, workload.Name, workload.Type)
	err = w.k8sClient.WaitForRollout(ctx, workload.Type, workload.Namespace, workload.Name, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("rollout failed: %w", err)
	}

	logger.Infof("Rollout completed: %s/%s (%s)", workload.Namespace, workload.Name, workload.Type)
	return nil
}

// getCredentialsForImage gets the appropriate registry credentials for an image
func (w *Watcher) getCredentialsForImage(ctx context.Context, namespace string, secretNames []string, image string) *registry.RegistryCredentials {
	// Parse image to extract registry
	imageInfo := registry.ParseImage(image)
	imageRegistry := extractRegistry(imageInfo.Repository)

	// Try each secret
	for _, secretName := range secretNames {
		auths, err := w.k8sClient.GetImagePullSecret(ctx, namespace, secretName)
		if err != nil {
			logger.Debugf("Failed to get secret %s: %v", secretName, err)
			continue
		}

		// Find matching registry
		for _, auth := range auths {
			if matchesRegistry(imageRegistry, auth.Registry) {
				logger.Debugf("  Found matching credentials for registry: %s", auth.Registry)
				return &registry.RegistryCredentials{
					Registry: auth.Registry,
					Username: auth.Username,
					Password: auth.Password,
				}
			}
		}
	}

	logger.Debugf("  No matching credentials found for registry: %s", imageRegistry)
	return nil
}

// extractRegistry extracts the registry host from a repository string
func extractRegistry(repository string) string {
	// Docker Hub images don't have registry prefix
	if !strings.Contains(repository, "/") {
		return "index.docker.io"
	}

	// If the first part contains a dot or colon, it's likely a registry
	parts := strings.SplitN(repository, "/", 2)
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
		return parts[0]
	}

	// Otherwise, it's Docker Hub (e.g., library/nginx)
	return "index.docker.io"
}

// matchesRegistry checks if image registry matches secret registry
func matchesRegistry(imageRegistry, secretRegistry string) bool {
	// Normalize registries
	imageRegistry = normalizeRegistry(imageRegistry)
	secretRegistry = normalizeRegistry(secretRegistry)

	// Direct match
	if imageRegistry == secretRegistry {
		return true
	}

	// Docker Hub special cases
	dockerHubRegistries := []string{
		"index.docker.io",
		"docker.io",
		"registry-1.docker.io",
		"registry.hub.docker.com",
	}

	imageIsDockerHub := contains(dockerHubRegistries, imageRegistry)
	secretIsDockerHub := contains(dockerHubRegistries, secretRegistry)

	return imageIsDockerHub && secretIsDockerHub
}

// normalizeRegistry normalizes a registry URL
func normalizeRegistry(registry string) string {
	// Remove https:// or http:// prefix
	registry = strings.TrimPrefix(registry, "https://")
	registry = strings.TrimPrefix(registry, "http://")
	// Remove trailing slash
	registry = strings.TrimSuffix(registry, "/")
	return strings.ToLower(registry)
}

// contains checks if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Close closes the watcher
func (w *Watcher) Close() error {
	if w.imageChecker != nil {
		return w.imageChecker.Close()
	}
	return nil
}
