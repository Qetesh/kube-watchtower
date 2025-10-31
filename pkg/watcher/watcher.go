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
	k8sClient, err := k8s.NewClient(cfg.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	imageChecker, err := registry.NewImageChecker()
	if err != nil {
		return nil, fmt.Errorf("failed to create image checker: %w", err)
	}

	notif := notifier.NewNotifier(cfg.NotificationURL, cfg.NotificationCluster)

	return &Watcher{
		config:       cfg,
		k8sClient:    k8sClient,
		imageChecker: imageChecker,
		notifier:     notif,
	}, nil
}

// Run runs the watcher
func (w *Watcher) Run(ctx context.Context) error {
	logger.Info("Checking all containers (except explicitly disabled with namespace)")

	// Run initial check
	if err := w.check(ctx); err != nil {
		logger.Errorf("Initial check failed: %v", err)
	}

	return nil
}

// check performs one check cycle
func (w *Watcher) check(ctx context.Context) error {
	logger.Debug("Starting image update check...")

	// Reset notifier results for this check cycle
	if w.notifier != nil {
		w.notifier.Reset()
	}

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

			scannedCount++

			logger.Debugf("Checking container: %s/%s/%s (%s)", workload.Namespace, workload.Name, container.Name, workload.Type)
			logger.Debugf("  Image: %s", container.Image)
			logger.Debugf("  Current Digest: %s", container.CurrentDigest)

			// Get registry credentials if imagePullSecrets are defined
			var credentials *registry.RegistryCredentials
			if len(workload.ImagePullSecrets) > 0 {
				logger.Debugf("  ImagePullSecrets found: %v", workload.ImagePullSecrets)
				credentials = w.getCredentialsForImage(ctx, workload.Namespace, workload.ImagePullSecrets, container.Image)
			}

			// Check for updates
			hasUpdate, newDigest, err := w.imageChecker.CheckForUpdate(ctx, container.Image, credentials)
			if err != nil {
				logger.Errorf("Failed to check image update for %s/%s/%s: %v", workload.Namespace, workload.Name, container.Name, err)
				if w.notifier != nil {
					w.notifier.AddResult(container.Image, false, err)
				}
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
				if w.notifier != nil {
					w.notifier.AddResult(container.Image, false, err)
				}
				failedCount++
				continue
			}

			updatedCount++
			logger.Debugf("Update successful: %s/%s/%s", workload.Namespace, workload.Name, container.Name)
			if w.notifier != nil {
				w.notifier.AddResult(container.Image, true, nil)
			}
		}
	}

	// Session done (like watchtower)
	notifyStatus := "no"
	if w.notifier != nil {
		notifyStatus = "yes"
	}
	logger.Infof("Session done Failed=%d Scanned=%d Updated=%d notify=%s", failedCount, scannedCount, updatedCount, notifyStatus)

	// Send summary notification
	if w.notifier != nil {
		w.notifier.SendSummary(scannedCount)
	}

	return nil
}

// updateContainer updates a container in a workload
func (w *Watcher) updateContainer(ctx context.Context, workload k8s.WorkloadInfo, container k8s.ContainerInfo, newDigest string) error {
	// Build new image name
	imageInfo := registry.ParseImage(container.Image)
	newImage := fmt.Sprintf("%s:%s@%s", imageInfo.Repository, imageInfo.Tag, newDigest)

	logger.Debugf("Updating image: %s -> %s", container.Image, newImage)

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
