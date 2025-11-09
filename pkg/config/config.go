package config

import (
	"os"
	"strings"
	"time"
)

// Config stores application configuration
type Config struct {

	// Notification URL (shoutrrr format) (default: "")
	NotificationURL string

	// Notification cluster name (default: "kubernetes")
	NotificationCluster string

	// Kubernetes disable namespaces (comma separated) (default: "")
	DisableNamespaces []string

	// Kubernetes enable namespaces (comma separated) (default: "")
	EnableNamespaces []string

	// Log level (default: info)
	LogLevel string

	// Dry-run mode (default: false)
	DryRun bool
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	config := &Config{
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		NotificationURL:     getEnv("NOTIFICATION_URL", ""),
		NotificationCluster: getEnv("NOTIFICATION_CLUSTER", "kubernetes"),
		DryRun:              getEnvBool("DRY_RUN", false),
	}

	// Parse disabled namespaces list
	disableNamespacesStr := getEnv("DISABLE_NAMESPACES", "")
	if disableNamespacesStr != "" {
		config.DisableNamespaces = strings.Split(disableNamespacesStr, ",")
		for i := range config.DisableNamespaces {
			config.DisableNamespaces[i] = strings.TrimSpace(config.DisableNamespaces[i])
		}
	}

	// Parse enabled namespaces list
	enableNamespacesStr := getEnv("ENABLE_NAMESPACES", "")
	if enableNamespacesStr != "" {
		config.EnableNamespaces = strings.Split(enableNamespacesStr, ",")
		for i := range config.EnableNamespaces {
			config.EnableNamespaces[i] = strings.TrimSpace(config.EnableNamespaces[i])
		}
	}
	return config
}

// IsNamespaceDisabled checks if a namespace is disabled
func (c *Config) IsNamespaceDisabled(namespace string) bool {
	for _, disabled := range c.DisableNamespaces {
		if disabled == namespace {
			return true
		}
	}
	return false
}

// getEnv gets environment variable, returns default if not exists
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool gets boolean environment variable
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

// getEnvDuration gets duration environment variable
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return duration
}
