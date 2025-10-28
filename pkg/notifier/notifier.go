package notifier

import (
	"fmt"

	"github.com/containrrr/shoutrrr"
	"github.com/qetesh/kubewatchtower/pkg/logger"
)

// Notifier handles sending notifications
type Notifier struct {
	url     string
	enabled bool
}

// NewNotifier creates a new notifier
func NewNotifier(url string) *Notifier {
	enabled := url != ""
	if enabled {
		logger.Infof("Using notifications: %s", extractServiceType(url))
	}
	return &Notifier{
		url:     url,
		enabled: enabled,
	}
}

// extractServiceType extracts service type from shoutrrr URL
// e.g., "telegram://..." -> "telegram"
func extractServiceType(url string) string {
	if url == "" {
		return "none"
	}
	for i, c := range url {
		if c == ':' {
			return url[:i]
		}
	}
	return "unknown"
}

// NotifyUpdateStarted notifies when update starts
func (n *Notifier) NotifyUpdateStarted(namespace, deployment, container, oldImage, newImage string) {
	if !n.enabled {
		return
	}

	message := fmt.Sprintf(
		"🔄 KubeWatchtower: Update Started\n"+
			"Namespace: %s\n"+
			"Deployment: %s\n"+
			"Container: %s\n"+
			"Old Image: %s\n"+
			"New Image: %s",
		namespace, deployment, container, oldImage, newImage,
	)

	n.send(message)
}

// NotifyUpdateSuccess notifies when update succeeds
func (n *Notifier) NotifyUpdateSuccess(namespace, deployment, container, image string) {
	if !n.enabled {
		return
	}

	message := fmt.Sprintf(
		"✅ KubeWatchtower: Update Successful\n"+
			"Namespace: %s\n"+
			"Deployment: %s\n"+
			"Container: %s\n"+
			"Image: %s",
		namespace, deployment, container, image,
	)

	n.send(message)
}

// NotifyUpdateFailure notifies when update fails
func (n *Notifier) NotifyUpdateFailure(namespace, deployment, container, image string, err error) {
	if !n.enabled {
		return
	}

	message := fmt.Sprintf(
		"❌ KubeWatchtower: Update Failed\n"+
			"Namespace: %s\n"+
			"Deployment: %s\n"+
			"Container: %s\n"+
			"Image: %s\n"+
			"Error: %v",
		namespace, deployment, container, image, err,
	)

	n.send(message)
}

// NotifyCheckCompleted notifies when check is completed
func (n *Notifier) NotifyCheckCompleted(updatedCount, totalCount int) {
	if !n.enabled {
		return
	}

	message := fmt.Sprintf(
		"📊 KubeWatchtower:\n"+
			"Check Completed\n"+
			"Updated: %d\n"+
			"Total: %d",
		updatedCount, totalCount,
	)

	n.send(message)
}

// send sends notification
func (n *Notifier) send(message string) {
	err := shoutrrr.Send(n.url, message)
	if err != nil {
		logger.Warnf("Failed to send notification: %v", err)
	}
}
