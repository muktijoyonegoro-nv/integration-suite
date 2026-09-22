package catalog

import (
	"github.com/testcontainers/testcontainers-go/wait"
)

// ServiceDefinition represents a declarative container specification for an application service.
type ServiceDefinition struct {
	Name         string
	DefaultImage string
	Aliases      []string
	ExposedPorts []string
	Env          map[string]string
	WaitingFor   wait.Strategy
}
