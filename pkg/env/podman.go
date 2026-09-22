package env

import (
	"os"
	"os/exec"
	"strings"

	"integration-suite/pkg/config"
)

// SetupPodmanEnvironment configures DOCKER_HOST and disables Ryuk for rootless Podman machines.
func SetupPodmanEnvironment() {
	_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	if os.Getenv("DOCKER_HOST") != "" {
		return
	}

	podmanBin := config.DefaultPodmanBin
	if cfg, err := config.Load(); err == nil && cfg != nil {
		podmanBin = cfg.GetPodmanBin()
	} else if envBin := os.Getenv("PODMAN_BIN"); envBin != "" {
		podmanBin = envBin
	}

	if _, err := os.Stat(podmanBin); err != nil {
		if resolved, err := exec.LookPath("podman"); err == nil {
			podmanBin = resolved
		}
	}

	out, err := exec.Command(podmanBin, "machine", "inspect", "--format", "{{.ConnectionInfo.PodmanSocket.Path}}").Output()
	if err == nil {
		socketPath := strings.TrimSpace(string(out))
		if socketPath != "" {
			_ = os.Setenv("DOCKER_HOST", "unix://"+socketPath)
		}
	}
}
