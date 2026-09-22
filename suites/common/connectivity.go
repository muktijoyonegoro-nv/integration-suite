package common

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-suite/pkg/testutil"
)

// AssertConnectivity runs sanity checks against all configured backing infrastructure and services.
func AssertConnectivity(t *testing.T, s *Scenario) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. MySQL Checks
	if len(s.Config.MySQL.Databases) > 0 {
		t.Run("MySQL Connectivity and Schemas", func(t *testing.T) {
			require.NotNil(t, s.DB(), "MySQL connection pool must not be nil")
			var result int
			err := s.DB().QueryRowContext(ctx, "SELECT 1").Scan(&result)
			require.NoError(t, err, "MySQL ping query (SELECT 1) failed")
			assert.Equal(t, 1, result)

			for _, table := range s.Config.MySQL.CheckTables {
				var count int
				query := fmt.Sprintf("SELECT count(*) FROM %s", table)
				err := s.DB().QueryRowContext(ctx, query).Scan(&count)
				require.NoError(t, err, "table check failed for: %s", table)
			}
		})
	}

	// 2. Redis Checks
	for _, alias := range s.Config.RedisInstances {
		aliasName := alias
		t.Run(fmt.Sprintf("Redis (%s) Connectivity", aliasName), func(t *testing.T) {
			client := s.RedisClient(aliasName)
			require.NotNil(t, client, "redis client for [%s] must not be nil", aliasName)
			pong, err := client.Ping(ctx).Result()
			require.NoError(t, err, "redis ping failed for [%s]", aliasName)
			assert.Equal(t, "PONG", pong)
		})
	}

	// 3. Kafka Checks
	if len(s.Config.Kafka.Topics) > 0 || s.KafkaBroker() != "" {
		t.Run("Kafka Broker Connectivity", func(t *testing.T) {
			require.NotEmpty(t, s.KafkaBroker(), "Kafka broker address must not be empty")
			err := testutil.EnsureTopic(s.KafkaBroker(), "harness-connectivity-check", 1, 1)
			require.NoError(t, err, "failed to create connectivity check topic on Kafka")
		})
	}

	// 4. WireMock Checks
	if s.Config.WireMock {
		t.Run("WireMock Auth Mock Connectivity", func(t *testing.T) {
			require.NotEmpty(t, s.WireMockURL(), "WireMock URL must not be empty")
			resp, err := http.Get(s.WireMockURL() + "/__admin/health")
			require.NoError(t, err, "WireMock health endpoint failed")
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}

	// 5. Application Services Checks
	for _, svc := range s.Config.Services {
		key := svc.EndpointKey
		if key == "" {
			key = svc.Def.Name
		}
		if svc.ExposePort != "" {
			svcKey := key
			t.Run(fmt.Sprintf("Service (%s) Endpoint Check", svcKey), func(t *testing.T) {
				endpoint := s.Endpoint(svcKey)
				require.NotEmpty(t, endpoint, "endpoint for service [%s] must not be empty", svcKey)
			})
		}
	}
}
