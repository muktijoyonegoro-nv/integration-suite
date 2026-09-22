package common

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"integration-suite/pkg/catalog"
	"integration-suite/pkg/env"
	"integration-suite/pkg/testutil"
)

var (
	activeScenario *Scenario
	scenarioMu     sync.Mutex
)

// Scenario encapsulates the runtime environment, active services, and endpoints for a test scenario.
type Scenario struct {
	Config    ScenarioConfig
	Env       *env.Environment
	Endpoints map[string]string
	Apps      map[string]*catalog.AppContainer

	mu sync.RWMutex
}

// Endpoint returns the dynamic mapped host URL (e.g. "http://127.0.0.1:54321") for a service key.
func (s *Scenario) Endpoint(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Endpoints[key]
}

// DB returns the shared MySQL connection pool.
func (s *Scenario) DB() *sql.DB {
	return s.Env.DB
}

// KafkaBroker returns the host-mapped Kafka broker address.
func (s *Scenario) KafkaBroker() string {
	return s.Env.KafkaBrokerAddr
}

// WireMockURL returns the host-mapped WireMock URL.
func (s *Scenario) WireMockURL() string {
	return s.Env.WireMockHostURL
}

// RedisClient returns the active client for a named Redis instance (e.g. "redis-sort-mistake").
func (s *Scenario) RedisClient(alias string) *redis.Client {
	return s.Env.RedisClient(alias)
}

// TruncateTables clears table rows across configured databases for clean test isolation.
func (s *Scenario) TruncateTables(ctx context.Context) error {
	if s.Env.DB != nil {
		return testutil.TruncateTables(ctx, s.Env.DB)
	}
	return nil
}

// FlushRedis flushes all configured Redis instances.
func (s *Scenario) FlushRedis(ctx context.Context) error {
	for alias := range s.Env.RedisClients {
		if client := s.RedisClient(alias); client != nil {
			if err := testutil.FlushRedis(ctx, client); err != nil {
				return fmt.Errorf("failed flushing redis [%s]: %w", alias, err)
			}
		}
	}
	return nil
}

// DumpLogs dumps stdout/stderr logs for any service marked with DumpLogsOnFailure.
func (s *Scenario) DumpLogs() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, svcCfg := range s.Config.Services {
		if svcCfg.DumpLogsOnFailure {
			if app, ok := s.Apps[svcCfg.Def.Name]; ok && app != nil {
				fmt.Printf("\n>>> [Harness] Test failed! Dumping logs for service [%s]:\n", svcCfg.Def.Name)
				if r, err := app.Container.Logs(ctx); err == nil {
					if logBytes, err := io.ReadAll(r); err == nil {
						fmt.Println(string(logBytes))
					}
				}
			}
		}
	}
}

// Teardown cleanly terminates all application containers, backing containers, and network.
func (s *Scenario) Teardown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.Env.Teardown(ctx)
}

// Setup provisions the scenario environment, launches application containers, runs connectivity checks,
// and registers automatic teardown with log dumping via t.Cleanup.
func Setup(t *testing.T, cfg ScenarioConfig) *Scenario {
	t.Helper()

	scenarioMu.Lock()
	defer scenarioMu.Unlock()

	var s *Scenario
	if activeScenario != nil && activeScenario.Config.Name == cfg.Name {
		s = activeScenario
	} else {
		timeout := cfg.StartupTimeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		netName := cfg.NetworkName
		if netName == "" {
			netName = strings.ReplaceAll(cfg.Name, "/", "-") + "-net"
		}

		builder := env.NewBuilder().WithNetwork(netName)

		if len(cfg.MySQL.Databases) > 0 {
			builder = builder.WithMySQL(env.MySQLConfig{
				Databases:  cfg.MySQL.Databases,
				Migrations: cfg.MySQL.Migrations,
			})
		}

		if len(cfg.Kafka.Topics) > 0 {
			builder = builder.WithKafka(env.KafkaConfig{
				Topics: cfg.Kafka.Topics,
			})
		}

		for _, redisAlias := range cfg.RedisInstances {
			builder = builder.WithRedisInstance(redisAlias)
		}

		if cfg.WireMock {
			builder = builder.WithWireMock()
		}

		scenarioEnv, err := builder.Build(ctx)
		require.NoError(t, err, "failed to build scenario backing infrastructure for [%s]", cfg.Name)

		s = &Scenario{
			Config:    cfg,
			Env:       scenarioEnv,
			Endpoints: make(map[string]string),
			Apps:      make(map[string]*catalog.AppContainer),
		}

		// Launch application containers
		for _, svcCfg := range cfg.Services {
			app, err := catalog.StartService(ctx, scenarioEnv, svcCfg.Def, svcCfg.ImageOverride)
			if err != nil {
				_ = scenarioEnv.Teardown(ctx)
				require.NoError(t, err, "failed to start service container [%s]", svcCfg.Def.Name)
			}
			s.Apps[svcCfg.Def.Name] = app

			endpointKey := svcCfg.EndpointKey
			if endpointKey == "" {
				endpointKey = svcCfg.Def.Name
			}

			if svcCfg.ExposePort != "" {
				endpoint, err := app.HostEndpoint(ctx, svcCfg.ExposePort)
				if err != nil {
					_ = scenarioEnv.Teardown(ctx)
					require.NoError(t, err, "failed resolving host endpoint for [%s] port [%s]", svcCfg.Def.Name, svcCfg.ExposePort)
				}
				s.Endpoints[endpointKey] = endpoint
			}
		}

		activeScenario = s
	}

	// Register automatic teardown and failure diagnostics
	t.Cleanup(func() {
		scenarioMu.Lock()
		defer scenarioMu.Unlock()

		if t.Failed() {
			s.DumpLogs()
		}

		_ = s.Teardown()
		activeScenario = nil
	})

	// Run connectivity checks as an automatic subtest
	t.Run("Connectivity Checks", func(t *testing.T) {
		AssertConnectivity(t, s)
	})

	return s
}
