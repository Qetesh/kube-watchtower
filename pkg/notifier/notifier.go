package notifier

import (
	"fmt"
	"strings"

	"github.com/containrrr/shoutrrr"
	"github.com/qetesh/kube-watchtower/pkg/logger"
)

// UpdateResult stores the result of an update operation
type UpdateResult struct {
	Image   string
	Success bool
	Error   error
}

// Notifier handles sending notifications
type Notifier struct {
	url         string
	clusterName string
	enabled     bool
	results     []UpdateResult
}

// NewNotifier creates a new notifier
func NewNotifier(url, clusterName string) *Notifier {
	enabled := url != ""
	if enabled {
		logger.Infof("Using notifications: %s", extractServiceType(url))
	}
	return &Notifier{
		url:         url,
		clusterName: clusterName,
		enabled:     enabled,
		results:     make([]UpdateResult, 0),
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

// AddResult adds an update result
func (n *Notifier) AddResult(image string, success bool, err error) {
	if !n.enabled {
		return
	}
	n.results = append(n.results, UpdateResult{
		Image:   image,
		Success: success,
		Error:   err,
	})
}

// SendSummary sends a summary notification of all updates
func (n *Notifier) SendSummary(totalCount int) {
	if !n.enabled {
		return
	}

	// If no updates were attempted, don't send notification
	if len(n.results) == 0 {
		return
	}

	message := n.buildSummaryMessage(totalCount)
	n.send(message)
}

// buildSummaryMessage builds the summary notification message
func (n *Notifier) buildSummaryMessage(totalCount int) string {
	var sb strings.Builder

	// Title
	sb.WriteString(fmt.Sprintf("☸️ kube-watchtower updates on %s\n\n", n.clusterName))

	// Separate successful and failed updates
	var successList []string
	var failList []string

	for _, result := range n.results {
		if result.Success {
			successList = append(successList, result.Image)
		} else {
			failList = append(failList, result.Image)
		}
	}

	// Successful updates
	if len(successList) > 0 {
		sb.WriteString("✅ Updated successfully:\n")
		for _, image := range successList {
			sb.WriteString(fmt.Sprintf("- %s\n", image))
		}
		sb.WriteString("\n")
	}

	// Failed updates
	if len(failList) > 0 {
		sb.WriteString("❌ Failed to update:\n")
		for _, image := range failList {
			sb.WriteString(fmt.Sprintf("- %s\n", image))
		}
		sb.WriteString("\n")
	}

	// Summary
	successCount := len(successList)
	sb.WriteString(fmt.Sprintf("Updated: %d/%d", successCount, totalCount))

	return sb.String()
}

// send sends notification
func (n *Notifier) send(message string) {
	err := shoutrrr.Send(n.url, message)
	if err != nil {
		logger.Warnf("Failed to send notification: %v", err)
	}
}

// Reset clears all stored results
func (n *Notifier) Reset() {
	n.results = make([]UpdateResult, 0)
}
