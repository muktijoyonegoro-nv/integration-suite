package intra_node

import (
	"integration-suite/pkg/catalog"
	"integration-suite/suites/common"
)

/*
	Partial Integration Test: sort-service Consumer & Event Ingestion Suite

	Validates sort-service container in complete isolation from upstream producers:
	1. Direct Kafka publication of contract-compliant SortNodeEvents messages (CREATED,
	   UPDATED, DELETED) using pkg/contract/fixtures and testutil.PublishSortNodeEvents.
	2. Asynchronous consumption and event processing by sort-service Kafka Streams topology.
	3. Verification of state synchronization into sort_service MySQL database and redis-sort cache.
	4. Verification of consumer idempotency and edge cases (e.g. repeated events, delete nonexistent).

	Resource Impact:
	- sort-mistake Go container, redis-sort-mistake, and sort_mistake database are OMITTED,
	  allowing targeted debugging of sort-service event ingestion.
*/

const (
	SortMistakeNodesTopic = "dev-sort-mistake-evt-nodes"
)

var scenarioConfig = common.ScenarioConfig{
	Name:        "consumer/intra_node",
	NetworkName: "consumer-intra-node-net",
	MySQL: common.MySQLConfig{
		Databases:   []string{"sort_service"},
		CheckTables: []string{"sort_service.intra_hub_nodes"},
	},
	Kafka: common.KafkaConfig{
		Topics: []string{
			SortMistakeNodesTopic,
		},
	},
	RedisInstances: []string{"redis-sort"},
	WireMock:       true,
	Services: []common.ServiceConfig{
		{
			Def:               catalog.SortService,
			DumpLogsOnFailure: true,
		},
	},
}
