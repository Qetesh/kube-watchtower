package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qetesh/kubewatchtower/pkg/logger"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client Kubernetes client wrapper
type Client struct {
	clientset *kubernetes.Clientset
	namespace string
}

// NewClient creates a new Kubernetes client
func NewClient(namespace string) (*Client, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &Client{
		clientset: clientset,
		namespace: namespace,
	}, nil
}

// getKubeConfig gets Kubernetes configuration
func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fallback to kubeconfig file
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return kubeConfig.ClientConfig()
}

// DeploymentInfo contains deployment information
type DeploymentInfo struct {
	Name       string
	Namespace  string
	Containers []ContainerInfo
}

// ContainerInfo contains container information
type ContainerInfo struct {
	Name            string
	Image           string
	ImagePullPolicy corev1.PullPolicy
	CurrentDigest   string // Current running container image digest
	Tag             string // Image tag
}

// ListDeployments lists all deployments to monitor
func (c *Client) ListDeployments(ctx context.Context) ([]DeploymentInfo, error) {
	namespace := c.namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	var result []DeploymentInfo
	for _, deploy := range deployments.Items {
		// Only process containers with Always pull policy
		var containers []ContainerInfo
		for _, container := range deploy.Spec.Template.Spec.Containers {
			if container.ImagePullPolicy == corev1.PullAlways {
				// Extract image tag
				tag := extractImageTag(container.Image)

				// Only monitor latest tag images
				if tag != "latest" {
					continue
				}

				containers = append(containers, ContainerInfo{
					Name:            container.Name,
					Image:           container.Image,
					ImagePullPolicy: container.ImagePullPolicy,
					Tag:             tag,
				})
			}
		}

		if len(containers) > 0 {
			// Get actual running pod info and extract current digest
			if err := c.fillCurrentDigests(ctx, &deploy, containers); err != nil {
				// Log warning but continue processing
				logger.Debugf("Warning: unable to get current digest for %s/%s: %v", deploy.Namespace, deploy.Name, err)
			}

			result = append(result, DeploymentInfo{
				Name:       deploy.Name,
				Namespace:  deploy.Namespace,
				Containers: containers,
			})
		}
	}

	return result, nil
}

// extractImageTag extracts tag from image string
func extractImageTag(image string) string {
	// Remove digest part if exists
	if idx := strings.Index(image, "@"); idx != -1 {
		image = image[:idx]
	}

	// Extract tag
	parts := strings.Split(image, ":")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}

	return "latest" // Default tag
}

// extractDigestFromImageID extracts digest from imageID
// imageID format: docker-pullable://nginx@sha256:abc123...
// or: docker.io/library/nginx@sha256:abc123...
func extractDigestFromImageID(imageID string) string {
	// Find @ symbol
	idx := strings.Index(imageID, "@")
	if idx == -1 {
		return ""
	}

	// Return part after @ (sha256:...)
	return imageID[idx+1:]
}

// fillCurrentDigests fills container current digest information
func (c *Client) fillCurrentDigests(ctx context.Context, deploy *appsv1.Deployment, containers []ContainerInfo) error {
	// Get pods for this deployment
	labelSelector := metav1.FormatLabelSelector(deploy.Spec.Selector)
	pods, err := c.clientset.CoreV1().Pods(deploy.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for deployment")
	}

	// Use first running pod
	var selectedPod *corev1.Pod
	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodRunning {
			selectedPod = &pods.Items[i]
			break
		}
	}

	if selectedPod == nil {
		return fmt.Errorf("no running pods found")
	}

	// Create container name to status mapping
	containerStatusMap := make(map[string]string)
	for _, status := range selectedPod.Status.ContainerStatuses {
		containerStatusMap[status.Name] = status.ImageID
	}

	// Fill digest information
	for i := range containers {
		if imageID, ok := containerStatusMap[containers[i].Name]; ok {
			containers[i].CurrentDigest = extractDigestFromImageID(imageID)
		}
	}

	return nil
}

// UpdateDeploymentImage updates deployment image
func (c *Client) UpdateDeploymentImage(ctx context.Context, namespace, deploymentName, containerName, newImage string) error {
	// Get current deployment
	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Update container image
	updated := false
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == containerName {
			deployment.Spec.Template.Spec.Containers[i].Image = newImage
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("container %s not found in deployment", containerName)
	}

	// Add annotation to trigger rollout
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubewatchtower.io/updated-at"] = time.Now().Format(time.RFC3339)

	// Update deployment
	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

// WaitForRollout waits for deployment rollout to complete
func (c *Client) WaitForRollout(ctx context.Context, namespace, deploymentName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for rollout")
		case <-ticker.C:
			deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get deployment: %w", err)
			}

			if isRolloutComplete(deployment) {
				return nil
			}
		}
	}
}

// isRolloutComplete checks if rollout is complete
func isRolloutComplete(deployment *appsv1.Deployment) bool {
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		replicas := int32(1)
		if deployment.Spec.Replicas != nil {
			replicas = *deployment.Spec.Replicas
		}

		if deployment.Status.UpdatedReplicas == replicas &&
			deployment.Status.Replicas == replicas &&
			deployment.Status.AvailableReplicas == replicas {
			return true
		}
	}
	return false
}

// CleanupOldReplicaSets cleans up old ReplicaSets
func (c *Client) CleanupOldReplicaSets(ctx context.Context, namespace, deploymentName string) error {
	// Get deployment
	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Get all ReplicaSets
	labelSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	replicaSets, err := c.clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list replicasets: %w", err)
	}

	// Delete old ReplicaSets with 0 replicas
	for _, rs := range replicaSets.Items {
		if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 0 {
			err := c.clientset.AppsV1().ReplicaSets(namespace).Delete(ctx, rs.Name, metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("failed to delete replicaset: %w", err)
			}
		}
	}

	return nil
}
