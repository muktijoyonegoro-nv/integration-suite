package env

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
)

const (
	LabelManaged = "harness.managed"
	LabelSuite   = "harness.suite"
	LabelService = "harness.service"
	ManagedValue = "true"
	SuiteValue   = "integration-suite"
)

// DefaultLabels returns standard metadata labels attached to test containers.
func DefaultLabels(serviceName string) map[string]string {
	labels := map[string]string{
		LabelManaged: ManagedValue,
		LabelSuite:   SuiteValue,
	}
	if serviceName != "" {
		labels[LabelService] = serviceName
	}
	return labels
}

// PruneManagedContainers discovers and force-removes all containers with label harness.managed=true.
// SAFETY: This only operates on containers and will NEVER prune, untag, or delete any images.
func PruneManagedContainers(ctx context.Context) error {
	SetupPodmanEnvironment()

	// Use detached context so pruning succeeds even if the caller context was cancelled
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli, err := testcontainers.NewDockerClientWithOpts(cleanupCtx)
	if err != nil {
		return fmt.Errorf("failed creating docker client for pruning: %w", err)
	}
	defer cli.Close()

	filters := make(client.Filters).Add("label", fmt.Sprintf("%s=%s", LabelManaged, ManagedValue))
	list, err := cli.ContainerList(cleanupCtx, client.ContainerListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("failed listing managed containers: %w", err)
	}

	for _, c := range list.Items {
		_, _ = cli.ContainerRemove(cleanupCtx, c.ID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
	}

	return nil
}
