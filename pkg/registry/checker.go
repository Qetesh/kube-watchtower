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
// Correctly handles registry ports (e.g., registry.example.com:5000/myimage:latest)
func ParseImage(image string) *ImageInfo {
	info := &ImageInfo{
		Tag: "latest",
	}

	// Separate digest (e.g., image@sha256:abc123)
	if idx := strings.Index(image, "@"); idx != -1 {
		info.Digest = image[idx+1:]
		image = image[:idx]
	}

	// Find tag: the last ":" that appears after the last "/"
	// This correctly distinguishes registry port from image tag
	// e.g., "registry.example.com:5000/myimage:latest" → tag is "latest"
	// e.g., "registry.example.com:5000/myimage" → no tag, default to "latest"
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")

	if lastColon > lastSlash {
		info.Repository = image[:lastColon]
		info.Tag = image[lastColon+1:]
	} else {
		info.Repository = image
	}

	return info
}

// RegistryCredentials contains registry authentication credentials
type RegistryCredentials struct {
	Registry string
	Username string
	Password string
}

// GetRemoteDigest fetches the current remote digest for an image from the registry
func (ic *ImageChecker) GetRemoteDigest(ctx context.Context, image string, credentials *RegistryCredentials) (string, error) {
	imageInfo := ParseImage(image)

	remoteDigest, err := ic.getRemoteDigest(ctx, imageInfo, credentials)
	if err != nil {
		return "", fmt.Errorf("failed to get remote digest: %w", err)
	}

	return remoteDigest, nil
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
