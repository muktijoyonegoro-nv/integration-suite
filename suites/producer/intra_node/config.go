package intra_node

import (
	"integration-suite/pkg/catalog"
	"integration-suite/suites/common"
)

/*
	Partial Integration Test: sort-mistake Producer & Contract Suite

	Validates sort-mistake container in complete isolation from downstream consumers:
	1. REST API invocation (POST / PATCH / DELETE) authenticated via WireMock.
	2. State verification in sort_mistake MySQL database and Redis cache.
	3. Kafka event publication on topic dev-sort-mistake-evt-nodes.
	4. Strict contract verification (schema invariants, wire compatibility, enum values)
	   using pkg/contract/sortmistake.

	Resource Impact:
	- sort-service JVM container is COMPLETELY OMITTED, reducing RAM usage by ~1GB+
	  and startup latency by ~30s.
*/

const (
	SortMistakeNodesTopic = "dev-sort-mistake-evt-nodes"
)

var scenarioConfig = common.ScenarioConfig{
	Name:        "producer/intra_node",
	NetworkName: "producer-intra-node-net",
	MySQL: common.MySQLConfig{
		Databases:   []string{"sort_mistake"},
		CheckTables: []string{"sort_mistake.intra_hub_nodes"},
	},
	Kafka: common.KafkaConfig{
		Topics: []string{
			SortMistakeNodesTopic,
		},
	},
	RedisInstances: []string{"redis-sort-mistake"},
	WireMock:       true,
	Services: []common.ServiceConfig{
		{
			Def:               catalog.SortMistake,
			ExposePort:        "9000/tcp",
			EndpointKey:       "sort-mistake",
			DumpLogsOnFailure: true,
		},
	},
}
