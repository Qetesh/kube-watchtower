package config

import (
	"os"
	"strings"
	"time"
)

// Config stores application configuration
type Config struct {
	// Schedule in cron format
	Schedule string

	// Enable cleanup of old resources
	Cleanup bool

	// List of disabled containers (comma separated)
	DisableContainers []string

	// Notification URL (shoutrrr format)
	NotificationURL string

	// Notification cluster name
	NotificationCluster string

	// Check interval (used when Schedule is not specified)
	CheckInterval time.Duration

	// Kubernetes namespace (empty means all namespaces)
	Namespace string

	// Run once and exit
	RunOnce  bool
	LogLevel string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	config := &Config{
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		Schedule:            getEnv("SCHEDULE", ""),
		Cleanup:             getEnvBool("CLEANUP", true),
		NotificationURL:     getEnv("NOTIFICATION_URL", ""),
		NotificationCluster: getEnv("NOTIFICATION_CLUSTER", "kubernetes"),
		CheckInterval:       getEnvDuration("CHECK_INTERVAL", 5*time.Minute),
		Namespace:           getEnv("NAMESPACE", ""),
		RunOnce:             getEnvBool("RUN_ONCE", false),
	}

	// Parse disabled containers list
	disableContainersStr := getEnv("DISABLE_CONTAINERS", "")
	if disableContainersStr != "" {
		config.DisableContainers = strings.Split(disableContainersStr, ",")
		for i := range config.DisableContainers {
			config.DisableContainers[i] = strings.TrimSpace(config.DisableContainers[i])
		}
	}

	return config
}

// IsContainerDisabled checks if a container is disabled
func (c *Config) IsContainerDisabled(containerName string) bool {
	for _, disabled := range c.DisableContainers {
		if disabled == containerName {
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
