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

// WorkloadType defines the type of Kubernetes workload
type WorkloadType string

const (
	WorkloadTypeDeployment  WorkloadType = "Deployment"
	WorkloadTypeDaemonSet   WorkloadType = "DaemonSet"
	WorkloadTypeStatefulSet WorkloadType = "StatefulSet"
)

// WorkloadInfo contains workload information
type WorkloadInfo struct {
	Type       WorkloadType
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

// ListWorkloads lists all workloads (Deployments, DaemonSets, StatefulSets) to monitor
func (c *Client) ListWorkloads(ctx context.Context) ([]WorkloadInfo, error) {
	namespace := c.namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	var result []WorkloadInfo

	// List Deployments
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	for _, deploy := range deployments.Items {
		if workload := c.processWorkload(ctx, WorkloadTypeDeployment, deploy.Name, deploy.Namespace, &deploy.Spec.Template.Spec, deploy.Spec.Selector); workload != nil {
			result = append(result, *workload)
		}
	}

	// List DaemonSets
	daemonsets, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list daemonsets: %w", err)
	}
	for _, ds := range daemonsets.Items {
		if workload := c.processWorkload(ctx, WorkloadTypeDaemonSet, ds.Name, ds.Namespace, &ds.Spec.Template.Spec, ds.Spec.Selector); workload != nil {
			result = append(result, *workload)
		}
	}

	// List StatefulSets
	statefulsets, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list statefulsets: %w", err)
	}
	for _, sts := range statefulsets.Items {
		if workload := c.processWorkload(ctx, WorkloadTypeStatefulSet, sts.Name, sts.Namespace, &sts.Spec.Template.Spec, sts.Spec.Selector); workload != nil {
			result = append(result, *workload)
		}
	}

	return result, nil
}

// processWorkload processes a workload and extracts container information
func (c *Client) processWorkload(ctx context.Context, workloadType WorkloadType, name, namespace string, podSpec *corev1.PodSpec, selector *metav1.LabelSelector) *WorkloadInfo {
	// Extract containers with Always pull policy and latest tag
	var containers []ContainerInfo
	for _, container := range podSpec.Containers {
		if container.ImagePullPolicy == corev1.PullAlways {
			tag := extractImageTag(container.Image)
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

	if len(containers) == 0 {
		return nil
	}

	// Get actual running pod info and extract current digest
	if err := c.fillCurrentDigestsFromSelector(ctx, namespace, selector, containers); err != nil {
		logger.Debugf("Warning: unable to get current digest for %s/%s: %v", namespace, name, err)
	}

	return &WorkloadInfo{
		Type:       workloadType,
		Name:       name,
		Namespace:  namespace,
		Containers: containers,
	}
}

// ListDeployments lists all deployments to monitor (deprecated, use ListWorkloads)
func (c *Client) ListDeployments(ctx context.Context) ([]WorkloadInfo, error) {
	return c.ListWorkloads(ctx)
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

// fillCurrentDigestsFromSelector fills container current digest information using label selector
func (c *Client) fillCurrentDigestsFromSelector(ctx context.Context, namespace string, selector *metav1.LabelSelector, containers []ContainerInfo) error {
	// Get pods using label selector
	labelSelector := metav1.FormatLabelSelector(selector)
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found")
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

// UpdateWorkloadImage updates workload image
func (c *Client) UpdateWorkloadImage(ctx context.Context, workloadType WorkloadType, namespace, name, containerName, newImage string) error {
	annotation := map[string]string{
		"kubewatchtower.io/updated-at": time.Now().Format(time.RFC3339),
	}

	switch workloadType {
	case WorkloadTypeDeployment:
		deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get deployment: %w", err)
		}
		if err := updateContainerImage(&deployment.Spec.Template.Spec, containerName, newImage); err != nil {
			return err
		}
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range annotation {
			deployment.Spec.Template.Annotations[k] = v
		}
		_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		return err

	case WorkloadTypeDaemonSet:
		daemonset, err := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get daemonset: %w", err)
		}
		if err := updateContainerImage(&daemonset.Spec.Template.Spec, containerName, newImage); err != nil {
			return err
		}
		if daemonset.Spec.Template.Annotations == nil {
			daemonset.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range annotation {
			daemonset.Spec.Template.Annotations[k] = v
		}
		_, err = c.clientset.AppsV1().DaemonSets(namespace).Update(ctx, daemonset, metav1.UpdateOptions{})
		return err

	case WorkloadTypeStatefulSet:
		statefulset, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get statefulset: %w", err)
		}
		if err := updateContainerImage(&statefulset.Spec.Template.Spec, containerName, newImage); err != nil {
			return err
		}
		if statefulset.Spec.Template.Annotations == nil {
			statefulset.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range annotation {
			statefulset.Spec.Template.Annotations[k] = v
		}
		_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(ctx, statefulset, metav1.UpdateOptions{})
		return err

	default:
		return fmt.Errorf("unsupported workload type: %s", workloadType)
	}
}

// updateContainerImage updates container image in pod spec
func updateContainerImage(podSpec *corev1.PodSpec, containerName, newImage string) error {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == containerName {
			podSpec.Containers[i].Image = newImage
			return nil
		}
	}
	return fmt.Errorf("container %s not found", containerName)
}

// UpdateDeploymentImage updates deployment image (deprecated, use UpdateWorkloadImage)
func (c *Client) UpdateDeploymentImage(ctx context.Context, namespace, deploymentName, containerName, newImage string) error {
	return c.UpdateWorkloadImage(ctx, WorkloadTypeDeployment, namespace, deploymentName, containerName, newImage)
}

// WaitForRollout waits for workload rollout to complete
func (c *Client) WaitForRollout(ctx context.Context, workloadType WorkloadType, namespace, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for rollout")
		case <-ticker.C:
			complete, err := c.isRolloutComplete(ctx, workloadType, namespace, name)
			if err != nil {
				return err
			}
			if complete {
				return nil
			}
		}
	}
}

// isRolloutComplete checks if rollout is complete for different workload types
func (c *Client) isRolloutComplete(ctx context.Context, workloadType WorkloadType, namespace, name string) (bool, error) {
	switch workloadType {
	case WorkloadTypeDeployment:
		deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get deployment: %w", err)
		}
		return isDeploymentRolloutComplete(deployment), nil

	case WorkloadTypeDaemonSet:
		daemonset, err := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get daemonset: %w", err)
		}
		return isDaemonSetRolloutComplete(daemonset), nil

	case WorkloadTypeStatefulSet:
		statefulset, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get statefulset: %w", err)
		}
		return isStatefulSetRolloutComplete(statefulset), nil

	default:
		return false, fmt.Errorf("unsupported workload type: %s", workloadType)
	}
}

// isDeploymentRolloutComplete checks if deployment rollout is complete
func isDeploymentRolloutComplete(deployment *appsv1.Deployment) bool {
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

// isDaemonSetRolloutComplete checks if daemonset rollout is complete
func isDaemonSetRolloutComplete(daemonset *appsv1.DaemonSet) bool {
	if daemonset.Generation <= daemonset.Status.ObservedGeneration {
		if daemonset.Status.UpdatedNumberScheduled == daemonset.Status.DesiredNumberScheduled &&
			daemonset.Status.NumberReady == daemonset.Status.DesiredNumberScheduled &&
			daemonset.Status.NumberAvailable == daemonset.Status.DesiredNumberScheduled {
			return true
		}
	}
	return false
}

// isStatefulSetRolloutComplete checks if statefulset rollout is complete
func isStatefulSetRolloutComplete(statefulset *appsv1.StatefulSet) bool {
	if statefulset.Generation <= statefulset.Status.ObservedGeneration {
		replicas := int32(1)
		if statefulset.Spec.Replicas != nil {
			replicas = *statefulset.Spec.Replicas
		}

		if statefulset.Status.UpdatedReplicas == replicas &&
			statefulset.Status.Replicas == replicas &&
			statefulset.Status.ReadyReplicas == replicas {
			return true
		}
	}
	return false
}

// CleanupOldResources cleans up old resources (ReplicaSets for Deployments, ControllerRevisions for DaemonSets/StatefulSets)
func (c *Client) CleanupOldResources(ctx context.Context, workloadType WorkloadType, namespace, name string) error {
	switch workloadType {
	case WorkloadTypeDeployment:
		return c.cleanupOldReplicaSets(ctx, namespace, name)
	case WorkloadTypeDaemonSet:
		return c.cleanupOldControllerRevisions(ctx, namespace, name)
	case WorkloadTypeStatefulSet:
		return c.cleanupOldControllerRevisions(ctx, namespace, name)
	default:
		return fmt.Errorf("unsupported workload type: %s", workloadType)
	}
}

// cleanupOldReplicaSets cleans up old ReplicaSets for Deployments
func (c *Client) cleanupOldReplicaSets(ctx context.Context, namespace, name string) error {
	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	labelSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	replicaSets, err := c.clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list replicasets: %w", err)
	}

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

// cleanupOldControllerRevisions cleans up old ControllerRevisions for DaemonSets and StatefulSets
func (c *Client) cleanupOldControllerRevisions(ctx context.Context, namespace, name string) error {
	// List all controller revisions
	revisions, err := c.clientset.AppsV1().ControllerRevisions(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list controller revisions: %w", err)
	}

	// Find revisions belonging to this workload, keep only the latest 2
	var workloadRevisions []appsv1.ControllerRevision
	for _, rev := range revisions.Items {
		if ownerName := getOwnerName(rev.OwnerReferences); ownerName == name {
			workloadRevisions = append(workloadRevisions, rev)
		}
	}

	// Sort by revision number (descending)
	if len(workloadRevisions) <= 2 {
		return nil // Keep at least 2 revisions
	}

	// Delete old revisions (keep the latest 2)
	for i := 2; i < len(workloadRevisions); i++ {
		err := c.clientset.AppsV1().ControllerRevisions(namespace).Delete(ctx, workloadRevisions[i].Name, metav1.DeleteOptions{})
		if err != nil {
			logger.Warnf("Failed to delete controller revision %s: %v", workloadRevisions[i].Name, err)
		}
	}

	return nil
}

// getOwnerName gets the owner name from owner references
func getOwnerName(ownerRefs []metav1.OwnerReference) string {
	if len(ownerRefs) > 0 {
		return ownerRefs[0].Name
	}
	return ""
}

// CleanupOldReplicaSets cleans up old ReplicaSets (deprecated, use CleanupOldResources)
func (c *Client) CleanupOldReplicaSets(ctx context.Context, namespace, deploymentName string) error {
	return c.CleanupOldResources(ctx, WorkloadTypeDeployment, namespace, deploymentName)
}
