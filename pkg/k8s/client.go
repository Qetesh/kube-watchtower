package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qetesh/kube-watchtower/pkg/logger"
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
}

// NewClient creates a new Kubernetes client
func NewClient() (*Client, error) {
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
	Type             WorkloadType
	Name             string
	Namespace        string
	Containers       []ContainerInfo
	ImagePullSecrets []string // Names of image pull secrets
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
func (c *Client) ListWorkloads(ctx context.Context, excludeNamespaces []string) ([]WorkloadInfo, error) {
	// Always list all namespaces
	namespace := corev1.NamespaceAll

	var result []WorkloadInfo

	// List Deployments
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	for _, deploy := range deployments.Items {
		// Only process deployments with available replicas
		if deploy.Status.AvailableReplicas <= 0 {
			logger.Debugf("Skipping deployment: %s/%s (available replicas: %d)", deploy.Namespace, deploy.Name, deploy.Status.AvailableReplicas)
			continue
		}
		if workload := c.processWorkload(ctx, WorkloadTypeDeployment, deploy.Name, deploy.Namespace, &deploy.Spec.Template.Spec, deploy.Spec.Selector, excludeNamespaces); workload != nil {
			result = append(result, *workload)
		}
	}

	// List DaemonSets
	daemonsets, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list daemonsets: %w", err)
	}
	for _, ds := range daemonsets.Items {
		// Only process daemonsets with available replicas
		if ds.Status.NumberAvailable <= 0 {
			logger.Debugf("Skipping daemonset: %s/%s (available replicas: %d)", ds.Namespace, ds.Name, ds.Status.NumberAvailable)
			continue
		}
		if workload := c.processWorkload(ctx, WorkloadTypeDaemonSet, ds.Name, ds.Namespace, &ds.Spec.Template.Spec, ds.Spec.Selector, excludeNamespaces); workload != nil {
			result = append(result, *workload)
		}
	}

	// List StatefulSets
	statefulsets, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list statefulsets: %w", err)
	}
	for _, sts := range statefulsets.Items {
		// Only process statefulsets with available replicas
		if sts.Status.AvailableReplicas <= 0 {
			logger.Debugf("Skipping statefulset: %s/%s (available replicas: %d)", sts.Namespace, sts.Name, sts.Status.AvailableReplicas)
			continue
		}
		if workload := c.processWorkload(ctx, WorkloadTypeStatefulSet, sts.Name, sts.Namespace, &sts.Spec.Template.Spec, sts.Spec.Selector, excludeNamespaces); workload != nil {
			result = append(result, *workload)
		}
	}

	return result, nil
}

// processWorkload processes a workload and extracts container information
func (c *Client) processWorkload(ctx context.Context, workloadType WorkloadType, name, namespace string, podSpec *corev1.PodSpec, selector *metav1.LabelSelector, excludeNamespaces []string) *WorkloadInfo {
	// Check if namespace is disabled
	for _, excludeNs := range excludeNamespaces {
		if excludeNs != "" && excludeNs == namespace {
			logger.Debugf("Skipping disabled namespace: %s", namespace)
			return nil
		}
	}

	// Extract containers with Always pull policy
	var containers []ContainerInfo
	for _, container := range podSpec.Containers {
		if container.ImagePullPolicy == corev1.PullAlways {
			tag := extractImageTag(container.Image)

			containers = append(containers, ContainerInfo{
				Name:            container.Name,
				Image:           container.Image,
				ImagePullPolicy: container.ImagePullPolicy,
				Tag:             tag,
			})
		} else {
			logger.Debugf("Skipping container: %s/%s (image pull policy: %s)", namespace, name, container.ImagePullPolicy)
		}
	}

	if len(containers) == 0 {
		return nil
	}

	// Get actual running pod info and extract current digest
	if err := c.fillCurrentDigestsFromSelector(ctx, namespace, selector, containers); err != nil {
		logger.Debugf("Warning: unable to get current digest for %s/%s: %v", namespace, name, err)
	}

	// Extract ImagePullSecrets
	imagePullSecrets := make([]string, 0, len(podSpec.ImagePullSecrets))
	for _, secret := range podSpec.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, secret.Name)
	}

	return &WorkloadInfo{
		Type:             workloadType,
		Name:             name,
		Namespace:        namespace,
		Containers:       containers,
		ImagePullSecrets: imagePullSecrets,
	}
}

// ListDeployments lists all deployments to monitor (deprecated, use ListWorkloads)
func (c *Client) ListDeployments(ctx context.Context) ([]WorkloadInfo, error) {
	return c.ListWorkloads(ctx, nil)
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
		"kube-watchtower.io/updated-at": time.Now().Format(time.RFC3339),
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

// getOwnerName gets the owner name from owner references
func getOwnerName(ownerRefs []metav1.OwnerReference) string {
	if len(ownerRefs) > 0 {
		return ownerRefs[0].Name
	}
	return ""
}

// DockerConfigJSON represents the structure of .dockerconfigjson
type DockerConfigJSON struct {
	Auths map[string]DockerAuthConfig `json:"auths"`
}

// DockerAuthConfig represents authentication configuration for a registry
type DockerAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

// RegistryAuth contains registry authentication information
type RegistryAuth struct {
	Registry string
	Username string
	Password string
}

// GetImagePullSecret retrieves and parses an image pull secret
func (c *Client) GetImagePullSecret(ctx context.Context, namespace, secretName string) ([]RegistryAuth, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	// Check if this is a docker config secret
	if secret.Type != corev1.SecretTypeDockerConfigJson {
		return nil, fmt.Errorf("secret %s is not a docker config secret (type: %s)", secretName, secret.Type)
	}

	// Parse .dockerconfigjson
	dockerConfigData, ok := secret.Data[corev1.DockerConfigJsonKey]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain .dockerconfigjson", secretName)
	}

	var dockerConfig DockerConfigJSON
	if err := json.Unmarshal(dockerConfigData, &dockerConfig); err != nil {
		return nil, fmt.Errorf("failed to parse docker config: %w", err)
	}

	// Extract auth information
	var auths []RegistryAuth
	for registry, authConfig := range dockerConfig.Auths {
		username := authConfig.Username
		password := authConfig.Password

		// If auth field is present but username/password are not, decode auth
		if authConfig.Auth != "" && (username == "" || password == "") {
			decoded, err := base64.StdEncoding.DecodeString(authConfig.Auth)
			if err != nil {
				logger.Warnf("Failed to decode auth for registry %s: %v", registry, err)
				continue
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				username = parts[0]
				password = parts[1]
			}
		}

		auths = append(auths, RegistryAuth{
			Registry: registry,
			Username: username,
			Password: password,
		})
	}

	return auths, nil
}
