package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/qetesh/kube-watchtower/pkg/logger"
)

// ImageChecker checks container image updates
type ImageChecker struct{}

// NewImageChecker creates a new image checker
func NewImageChecker() (*ImageChecker, error) {
	return &ImageChecker{}, nil
}

// ImageInfo contains image information
type ImageInfo struct {
	Repository string
	Tag        string
	Digest     string
}

// ParseImage parses image string into ImageInfo
func ParseImage(image string) *ImageInfo {
	info := &ImageInfo{
		Tag: "latest",
	}

	// Separate digest
	parts := strings.Split(image, "@")
	if len(parts) > 1 {
		info.Digest = parts[1]
		image = parts[0]
	}

	// Separate tag
	parts = strings.Split(image, ":")
	info.Repository = parts[0]
	if len(parts) > 1 {
		info.Tag = parts[1]
	}

	return info
}

// RegistryCredentials contains registry authentication credentials
type RegistryCredentials struct {
	Registry string
	Username string
	Password string
}

// CheckForUpdate checks if image has an update
// Returns: hasUpdate (whether there is an update), remoteDigest (remote image digest), error
func (ic *ImageChecker) CheckForUpdate(ctx context.Context, currentImage string, credentials *RegistryCredentials) (bool, string, error) {
	imageInfo := ParseImage(currentImage)

	// Get remote image digest
	remoteDigest, err := ic.getRemoteDigest(ctx, imageInfo, credentials)
	if err != nil {
		return false, "", fmt.Errorf("failed to get remote digest: %w", err)
	}

	// Return remote digest, let caller decide whether to update
	// hasUpdate is always true here, specific comparison logic is in watcher
	return true, remoteDigest, nil
}

// getRemoteDigest gets remote image digest
func (ic *ImageChecker) getRemoteDigest(ctx context.Context, imageInfo *ImageInfo, credentials *RegistryCredentials) (string, error) {
	imageName := fmt.Sprintf("%s:%s", imageInfo.Repository, imageInfo.Tag)

	ref, err := name.ParseReference(imageName)
	if err != nil {
		return "", fmt.Errorf("failed to parse image name %s: %w", imageName, err)
	}

	// Prepare authentication options
	options := []remote.Option{
		remote.WithContext(ctx),
	}

	// Add authentication if credentials are provided
	if credentials != nil && credentials.Username != "" {
		auth := &authn.Basic{
			Username: credentials.Username,
			Password: credentials.Password,
		}
		options = append(options, remote.WithAuth(auth))
		logger.Debugf("Using credentials for registry: %s", credentials.Registry)
	} else {
		// Use default keychain (can read from ~/.docker/config.json)
		options = append(options, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	// Check distribution
	desc, err := remote.Get(ref, options...)
	if err != nil {
		return "", fmt.Errorf("failed to inspect distribution: %w", err)
	}

	return desc.Digest.String(), nil
}

// Close closes the image checker (no-op, kept for interface compatibility)
func (ic *ImageChecker) Close() error {
	return nil
}
