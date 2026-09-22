package env

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
)

// Environment holds the runtime state, client connection pools, and containers for a test scenario.
type Environment struct {
	Network *testcontainers.DockerNetwork

	// Backing Containers
	MySQLContainer    testcontainers.Container
	KafkaContainer    *kafka.KafkaContainer
	WireMockContainer testcontainers.Container
	RedisContainers   map[string]testcontainers.Container

	// Companion App Containers
	AppContainers []testcontainers.Container

	// Host Mapped Endpoints (Dynamic Ephemeral Ports)
	MySQLHostDSN    string
	KafkaBrokerAddr string
	WireMockHostURL string
	RedisAddrs      map[string]string

	// Verified Client Pools
	DB           *sql.DB
	RedisClients map[string]*redis.Client
}

// RedisClient returns the active client for a named Redis instance (e.g. "redis-sort-mistake").
func (e *Environment) RedisClient(alias string) *redis.Client {
	if e.RedisClients == nil {
		return nil
	}
	return e.RedisClients[alias]
}

// Teardown cleanly terminates all containers, prunes dangling managed containers, and removes the network.
func (e *Environment) Teardown(ctx context.Context) error {
	var errs []string

	// Use a fresh detached timeout context so cleanup succeeds even if the caller context was cancelled
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if e.DB != nil {
		_ = e.DB.Close()
	}
	for _, client := range e.RedisClients {
		if client != nil {
			_ = client.Close()
		}
	}

	for _, app := range e.AppContainers {
		if app != nil {
			if err := app.Terminate(cleanupCtx); err != nil {
				errs = append(errs, fmt.Sprintf("app container: %v", err))
			}
		}
	}

	for alias, redisC := range e.RedisContainers {
		if redisC != nil {
			if err := redisC.Terminate(cleanupCtx); err != nil {
				errs = append(errs, fmt.Sprintf("redis (%s): %v", alias, err))
			}
		}
	}

	if e.MySQLContainer != nil {
		if err := e.MySQLContainer.Terminate(cleanupCtx); err != nil {
			errs = append(errs, fmt.Sprintf("mysql: %v", err))
		}
	}

	if e.KafkaContainer != nil {
		if err := e.KafkaContainer.Terminate(cleanupCtx); err != nil {
			errs = append(errs, fmt.Sprintf("kafka: %v", err))
		}
	}

	if e.WireMockContainer != nil {
		if err := e.WireMockContainer.Terminate(cleanupCtx); err != nil {
			errs = append(errs, fmt.Sprintf("wiremock: %v", err))
		}
	}

	if e.Network != nil {
		if err := e.Network.Remove(cleanupCtx); err != nil {
			errs = append(errs, fmt.Sprintf("network: %v", err))
		}
	}

	// Safety net: prune any dangling managed containers (never touches images)
	if err := PruneManagedContainers(cleanupCtx); err != nil {
		errs = append(errs, fmt.Sprintf("prune managed containers: %v", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardown errors: %s", strings.Join(errs, "; "))
	}
	return nil
}
