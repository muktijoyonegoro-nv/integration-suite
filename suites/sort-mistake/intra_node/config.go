package intra_node

import (
	"integration-suite/pkg/catalog"
	"integration-suite/suites/common"
)

/*
	Test: Sort Mistake - Intra Node (Intra-Hub Node CRUD Lifecycle Pipeline)

	Validates end-to-end intra-hub node management and asynchronous event propagation:
	1. Create Node: Calls the live sort-mistake REST API (POST /1.0/intra/hubs/{hubId}/nodes)
	   authenticated via WireMock AAA, verifies SortNodeEvents (NODE_EVENT_CREATED)
	   published to Kafka, and confirms downstream sort-service synchronizes the node
	   into its MySQL database (sort_service.intra_hub_nodes) and Redis cache.
	2. Update Node: Creates a prerequisite node, calls the live sort-mistake REST API
	   (PATCH /1.0/intra/hubs/{hubId}/nodes/{nodeId}), verifies NODE_EVENT_UPDATED
	   published to Kafka, and asserts that sort-service updates its DB record and Redis cache.
	3. Delete Node: Creates a prerequisite node, calls the live sort-mistake REST API
	   (DELETE /1.0/intra/hubs/{hubId}/nodes/{nodeId}), verifies NODE_EVENT_DELETED
	   published to Kafka, and asserts that sort-service and sort-mistake evict the node
	   from both MySQL databases and Redis cache.
*/

const (
	SortMistakeNodesTopic           = "dev-sort-mistake-evt-nodes"
	HubEventsTopic                  = "hub-events-dev-proto-topic"
	ShipmentFailedParcelUpdateTopic = "dev-hub-evt-shipment-failed-parcel-update"
)

var scenarioConfig = common.ScenarioConfig{
	Name:        "sort-mistake/intra_node",
	NetworkName: "sort-mistake-intra-node-net",
	MySQL: common.MySQLConfig{
		Databases:   []string{"sort_mistake", "sort_service"},
		CheckTables: []string{"sort_mistake.intra_hub_nodes", "sort_service.intra_hub_nodes"},
	},
	Kafka: common.KafkaConfig{
		Topics: []string{
			SortMistakeNodesTopic,
			HubEventsTopic,
			ShipmentFailedParcelUpdateTopic,
		},
	},
	RedisInstances: []string{"redis-sort-mistake", "redis-sort"},
	WireMock:       true,
	Services: []common.ServiceConfig{
		{
			Def:         catalog.SortMistake,
			ExposePort:  "9000/tcp",
			EndpointKey: "sort-mistake",
		},
		{
			Def:               catalog.SortService,
			DumpLogsOnFailure: true,
		},
	},
}
