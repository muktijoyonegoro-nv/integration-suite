package common

import (
	"time"

	"integration-suite/pkg/catalog"
)

// ScenarioConfig declares the infrastructure and services required for a test scenario.
type ScenarioConfig struct {
	Name           string          // e.g. "sort-mistake/intra_node"
	NetworkName    string          // Optional; if empty, defaults to sanitized Name + "-net"
	StartupTimeout time.Duration   // Optional; if 0, defaults to 5 minutes
	MySQL          MySQLConfig
	Kafka          KafkaConfig
	RedisInstances []string        // Container aliases, e.g. ["redis-sort-mistake", "redis-sort"]
	WireMock       bool            // Whether WireMock should be booted
	Services       []ServiceConfig // Application services from pkg/catalog to launch
}

// MySQLConfig holds scenario-specific MySQL settings.
type MySQLConfig struct {
	Databases   []string // Databases to ensure exist
	CheckTables []string // Tables (e.g. "sort_mistake.intra_hub_nodes") to verify in connectivity checks
}

// KafkaConfig holds scenario-specific Kafka settings.
type KafkaConfig struct {
	Topics []string // Topics to pre-provision
}

// ServiceConfig defines how a catalog service is run within a scenario.
type ServiceConfig struct {
	Def               catalog.ServiceDefinition
	ImageOverride     string // Optional manual image override (defaults to env or catalog default)
	ExposePort        string // Port to expose (e.g. "9000/tcp") to generate a dynamic host endpoint
	EndpointKey       string // Key for Scenario.Endpoint(key) (defaults to Def.Name)
	DumpLogsOnFailure bool   // If true, dumps container stdout/stderr if the test fails
}
