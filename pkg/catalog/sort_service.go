package catalog

import (
	"time"

	"github.com/testcontainers/testcontainers-go/wait"
)

// SortService is the service definition for sort-service (Java/Play).
var SortService = ServiceDefinition{
	Name:         "sort-service",
	DefaultImage: "sort-service:latest",
	Aliases:      []string{"sort-service"},
	ExposedPorts: []string{"9000/tcp"},
	Env: map[string]string{
		"DB_USER":                           "root",
		"DB_PASSWORD":                       "root",
		"DB_URI":                            "jdbc:mysql://mysql:3306/sort_service?characterEncoding=UTF-8",
		"NV_SERVICE_NAME":                   "SORT_SERVICE",
		"NV_REDIS_COMMON_ENABLE":            "true",
		"NV_REDIS_HOST":                     "redis-sort:6379",
		"NV_REDIS_PASSWORD":                 "",
		"NV_KAFKA_SERVERS":                           "kafka:9092",
		"NV_KAFKA_SORT_MISTAKE_NODES_TOPIC":          "dev-sort-mistake-evt-nodes",
		"NV_KAFKA_STREAMS_ENABLE_CONSUMER":           "true",
		"NV_KAFKA_STREAMS_APPLICATION_ID_CONFIG":     "sort",
		"NV_KAFKA_STREAMS_AUTO_OFFSET_RESET_CONFIG":  "earliest",
		"NV_KAFKA_STREAMS_REPLICATION_FACTOR_CONFIG": "1",
		"NV_KAFKA_STREAMS_NUM_STREAM_THREADS_CONFIG": "1",
		"NV_AUTH_SERVICE_ID":                         "SORT_SERVICE",
		"NV_AUTH_SERVICE_SECRET":                     "test-secret",
		"NV_AUTH_API_URL":                            "http://mock-services:8080/sg/aaa/",
	},
	WaitingFor: wait.ForListeningPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
}
