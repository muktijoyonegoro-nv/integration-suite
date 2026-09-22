package catalog

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"

	"integration-suite/pkg/config"
	"integration-suite/pkg/env"
)

// AppContainer wraps a running catalog service container.
type AppContainer struct {
	Container testcontainers.Container
	Def       ServiceDefinition
}

// StartService launches a catalog service attached to the test environment network with random host ports.
func StartService(ctx context.Context, e *env.Environment, def ServiceDefinition, imageOverride string) (*AppContainer, error) {
	imageName := def.DefaultImage
	if imageOverride != "" {
		imageName = imageOverride
	} else if cfg, err := config.Load(); err == nil && cfg != nil {
		imageName = cfg.GetLocalImage(def.Name, def.DefaultImage)
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        imageName,
			ExposedPorts: def.ExposedPorts,
			Env:          def.Env,
			Labels:       env.DefaultLabels(def.Name),
			Networks:     []string{e.Network.Name},
			NetworkAliases: map[string][]string{
				e.Network.Name: def.Aliases,
			},
			WaitingFor: def.WaitingFor,
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed starting catalog service [%s]: %w", def.Name, err)
	}

	e.AppContainers = append(e.AppContainers, c)
	return &AppContainer{Container: c, Def: def}, nil
}

// HostEndpoint resolves the dynamic, random host endpoint for the specified container port.
func (a *AppContainer) HostEndpoint(ctx context.Context, internalPort string) (string, error) {
	mappedPort, err := a.Container.MappedPort(ctx, internalPort)
	if err != nil {
		return "", fmt.Errorf("failed resolving mapped port for %s: %w", internalPort, err)
	}
	return fmt.Sprintf("http://127.0.0.1:%s", mappedPort.Port()), nil
}

// Terminate terminates the running container.
func (a *AppContainer) Terminate(ctx context.Context) error {
	return a.Container.Terminate(ctx)
}
