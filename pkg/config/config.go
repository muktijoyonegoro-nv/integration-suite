package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultPodmanBin is the fallback binary path when not configured.
const DefaultPodmanBin = "/opt/podman/bin/podman"

// Config represents root settings for the integration test harness.
type Config struct {
	Podman PodmanConfig `yaml:"podman"`
}

// PodmanConfig holds container runtime configurations.
type PodmanConfig struct {
	Bin    string            `yaml:"bin"`
	Images map[string]string `yaml:"images"`
}

// DefaultConfig provides sensible fallback settings if config.yaml is absent.
// Note: Only public backing infrastructure images are kept in config.yaml / DefaultConfig.
// Internal application service images (sort-mistake, sort-service) are configured via .env.
func DefaultConfig() *Config {
	return &Config{
		Podman: PodmanConfig{
			Bin: DefaultPodmanBin,
			Images: map[string]string{
				"mysql":    "mysql:8.0",
				"kafka":    "confluentinc/confluent-local:7.6.0",
				"redis":    "redis:7-alpine",
				"wiremock": "docker.io/wiremock/wiremock:3.5.2",
				"flyway":   "docker.io/flyway/flyway:11-alpine",
			},
		},
	}
}

// FindConfigFile searches for config.yaml starting from the current directory and walking upwards.
func FindConfigFile() string {
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		return envPath
	}

	dir, err := os.Getwd()
	if err != nil {
		return "config.yaml"
	}

	for {
		candidate := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "config.yaml"
}

// FindEnvFile searches for .env starting from the current directory and walking upwards.
func FindEnvFile() string {
	if envPath := os.Getenv("ENV_PATH"); envPath != "" {
		return envPath
	}

	dir, err := os.Getwd()
	if err != nil {
		return ".env"
	}

	for {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// LoadEnv loads environment variables from a .env file if present.
// Existing environment variables already present in the process are not overwritten.
func LoadEnv(envPath string) error {
	if envPath == "" {
		return nil
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

// Load loads the configuration file and .env file.
// If config.yaml is not found, default configuration is returned.
func Load() (*Config, error) {
	// Auto-discover and load .env if present
	if envFile := FindEnvFile(); envFile != "" {
		_ = LoadEnv(envFile)
	}

	configPath := FindConfigFile()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed reading config file at %s: %w", configPath, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed parsing config yaml at %s: %w", configPath, err)
	}

	if cfg.Podman.Bin == "" {
		cfg.Podman.Bin = DefaultPodmanBin
	}

	if cfg.Podman.Images == nil {
		cfg.Podman.Images = make(map[string]string)
	}

	return cfg, nil
}

// ServiceNameToEnvKey converts a kebab-case service name to uppercase SNAKE_CASE.
// e.g. "sort-mistake" -> "SORT_MISTAKE"
func ServiceNameToEnvKey(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// GetPodmanBin returns the configured Podman binary path, checking PODMAN_BIN env, config, or default.
func (c *Config) GetPodmanBin() string {
	if envBin := os.Getenv("PODMAN_BIN"); envBin != "" {
		return envBin
	}
	if c != nil && c.Podman.Bin != "" {
		return c.Podman.Bin
	}
	return DefaultPodmanBin
}

// GetLocalImage resolves an image tag for a local application service.
// It checks the corresponding environment variable (e.g. SORT_MISTAKE_IMAGE),
// falling back to defaultValue (or <serviceName>:latest if defaultValue is empty).
func (c *Config) GetLocalImage(serviceName string, defaultValue string) string {
	envKey := fmt.Sprintf("%s_IMAGE", ServiceNameToEnvKey(serviceName))
	if envVal := os.Getenv(envKey); envVal != "" {
		return envVal
	}
	if defaultValue != "" {
		return defaultValue
	}
	return fmt.Sprintf("%s:latest", serviceName)
}

// GetLocalRepoDir resolves the local repository directory path for a service.
// It checks the corresponding environment variable (e.g. SORT_MISTAKE_DIR),
// falling back to defaultDir (or ../<serviceName> if defaultDir is empty).
func (c *Config) GetLocalRepoDir(serviceName string, defaultDir string) string {
	envKey := fmt.Sprintf("%s_DIR", ServiceNameToEnvKey(serviceName))
	if envVal := os.Getenv(envKey); envVal != "" {
		return envVal
	}
	if defaultDir != "" {
		return defaultDir
	}
	return fmt.Sprintf("../%s", serviceName)
}

// GetImage retrieves an image tag by service key.
// It checks:
// 1. Podman.Images in config.yaml (for public backing infrastructure)
// 2. <SERVICE>_IMAGE environment variable (for local services from .env)
// 3. Fallback to defaultValue.
func (c *Config) GetImage(key string, defaultValue string) string {
	if c != nil && c.Podman.Images != nil {
		if img, exists := c.Podman.Images[key]; exists && img != "" {
			return img
		}
	}
	envKey := fmt.Sprintf("%s_IMAGE", ServiceNameToEnvKey(key))
	if envVal := os.Getenv(envKey); envVal != "" {
		return envVal
	}
	return defaultValue
}
