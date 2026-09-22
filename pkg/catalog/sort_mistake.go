package catalog

import (
	"time"

	"github.com/testcontainers/testcontainers-go/wait"
)

// SortMistake is the service definition for sort-mistake (Go).
var SortMistake = ServiceDefinition{
	Name:         "sort-mistake",
	DefaultImage: "sort-mistake:latest",
	Aliases:      []string{"sort-mistake"},
	ExposedPorts: []string{"9000/tcp"},
	Env: map[string]string{
		"NV_AUTH_SERVICE_ID":                "SORT_MISTAKE",
		"DB_HOST":                           "mysql:3306",
		"DB_NAME":                           "sort_mistake",
		"DB_USER":                           "root",
		"DB_PASSWORD":                       "root",
		"NV_DB_MAX_CONNS":                   "10",
		"NV_KAFKA_SERVERS":                  "kafka:9092",
		"NV_KAFKA_AUTH_BROKERS":             "kafka:9092",
		"NV_KAFKA_GROUP_ID":                 "sort-mistake-workers",
		"NV_KAFKA_PRODUCER_ENABLE":          "true",
		"NV_KAFKA_CONSUMER_ENABLE":          "false",
		"NV_KAFKA_SORT_MISTAKE_NODES_TOPIC": "dev-sort-mistake-evt-nodes",
		"NV_REDIS_HOST":                     "redis-sort-mistake:6379",
		"NV_REDIS_ENABLE":                   "true",
		"NV_REDIS_COMMON_ENABLE":            "true",
		"NV_REDIS_PASSWORD":                 "",
		"NV_AUTH_API_URL":                   "http://mock-services:8080/global/aaa",
		"NV_INTERNAL_HTTP_URI_SORT":         "http://sort-service:9000/%s/sort/",
		"NV_INTERNAL_HTTP_URI_ZONES":        "http://mock-services:8080/%s/zones/",
		"NV_INTERNAL_HTTP_URI_CORE":         "http://mock-services:8080/%s/core/",
	},
	WaitingFor: wait.ForListeningPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
}
