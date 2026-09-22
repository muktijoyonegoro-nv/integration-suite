package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-suite/pkg/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/non/existent/path/config.yaml")
	t.Setenv("ENV_PATH", "/non/existent/path/.env")
	t.Setenv("PODMAN_BIN", "")
	t.Setenv("SORT_MISTAKE_IMAGE", "")
	t.Setenv("SORT_SERVICE_IMAGE", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "/opt/podman/bin/podman", cfg.GetPodmanBin())
	assert.Equal(t, "mysql:8.0", cfg.GetImage("mysql", "fallback"))
	assert.Equal(t, "docker.io/wiremock/wiremock:3.5.2", cfg.GetImage("wiremock", "fallback"))
	assert.Equal(t, "custom-fallback", cfg.GetImage("nonexistent", "custom-fallback"))

	// Verify local images are NOT in Podman.Images by default
	_, hasSortMistake := cfg.Podman.Images["sort-mistake"]
	assert.False(t, hasSortMistake, "sort-mistake must not be in public Podman.Images")
	_, hasSortService := cfg.Podman.Images["sort-service"]
	assert.False(t, hasSortService, "sort-service must not be in public Podman.Images")
}

func TestLoad_FromFile(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
podman:
  bin: /custom/bin/podman
  images:
    mysql: "custom-mysql:9.0"
    redis: "redis:alpine-custom"
`
	err := os.WriteFile(configFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_PATH", configFile)
	t.Setenv("ENV_PATH", "/non/existent/path/.env")
	t.Setenv("PODMAN_BIN", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "/custom/bin/podman", cfg.GetPodmanBin())
	assert.Equal(t, "custom-mysql:9.0", cfg.GetImage("mysql", "mysql:8.0"))
	assert.Equal(t, "redis:alpine-custom", cfg.GetImage("redis", "redis:7-alpine"))
	// Keys not in custom file should fall back to default or argument
	assert.Equal(t, "confluentinc/confluent-local:7.6.0", cfg.GetImage("kafka", "fallback"))
}

func TestLoad_EnvOverrideBin(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/non/existent/path/config.yaml")
	t.Setenv("ENV_PATH", "/non/existent/path/.env")
	t.Setenv("PODMAN_BIN", "/override/bin/podman")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "/override/bin/podman", cfg.GetPodmanBin())
}

func TestLoad_DotEnvIntegration(t *testing.T) {
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")

	envContent := `
# Local test environment
SORT_MISTAKE_DIR=/custom/path/to/sort-mistake
SORT_SERVICE_DIR=/custom/path/to/sort-service
SORT_MISTAKE_IMAGE=custom-sort-mistake:v1
SORT_SERVICE_IMAGE=custom-sort-service:v2
`
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_PATH", "/non/existent/path/config.yaml")
	t.Setenv("ENV_PATH", envFile)
	t.Setenv("SORT_MISTAKE_DIR", "")
	t.Setenv("SORT_SERVICE_DIR", "")
	t.Setenv("SORT_MISTAKE_IMAGE", "")
	t.Setenv("SORT_SERVICE_IMAGE", "")

	cfg, err := config.Load()
	require.NoError(t, err)

	// Verify local images resolved from .env
	assert.Equal(t, "custom-sort-mistake:v1", cfg.GetLocalImage("sort-mistake", "default:latest"))
	assert.Equal(t, "custom-sort-service:v2", cfg.GetLocalImage("sort-service", "default:latest"))
	assert.Equal(t, "custom-sort-mistake:v1", cfg.GetImage("sort-mistake", "default:latest"))
	assert.Equal(t, "custom-sort-service:v2", cfg.GetImage("sort-service", "default:latest"))

	// Verify local repo directories resolved from .env
	assert.Equal(t, "/custom/path/to/sort-mistake", cfg.GetLocalRepoDir("sort-mistake", "../sort-mistake"))
	assert.Equal(t, "/custom/path/to/sort-service", cfg.GetLocalRepoDir("sort-service", "../sort-service"))
}

func TestGetLocalImage_Defaults(t *testing.T) {
	t.Setenv("SORT_MISTAKE_IMAGE", "")
	t.Setenv("SORT_NEW_SVC_IMAGE", "")

	cfg := config.DefaultConfig()

	assert.Equal(t, "sort-mistake:latest", cfg.GetLocalImage("sort-mistake", "sort-mistake:latest"))
	assert.Equal(t, "sort-new-svc:latest", cfg.GetLocalImage("sort-new-svc", ""))
	assert.Equal(t, "../sort-new-svc", cfg.GetLocalRepoDir("sort-new-svc", ""))
}
