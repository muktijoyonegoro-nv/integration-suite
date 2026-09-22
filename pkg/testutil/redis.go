package testutil

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"

	"integration-suite/proto/sortmistake"
)

// ConnectRedis opens and checks connection to a Redis server.
func ConnectRedis(addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping redis at %s: %w", addr, err)
	}

	return client, nil
}

// FlushRedis flushes all keys from the Redis database.
func FlushRedis(ctx context.Context, client *redis.Client) error {
	return client.FlushDB(ctx).Err()
}

// GetSortServiceIntraNode retrieves a cached SortNode from sort-service Redis.
// Hash key format: intra_node_<system_id>_<hub_id>, field: string decimal representation of nodeID.
func GetSortServiceIntraNode(ctx context.Context, client *redis.Client, systemID string, hubID int64, nodeID int64) (*sortmistake.SortNode, error) {
	key := fmt.Sprintf("intra_node_%s_%d", systemID, hubID)
	field := strconv.FormatInt(nodeID, 10)

	bytesVal, err := client.HGet(ctx, key, field).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to HGet from redis [key: %s, field: %s]: %w", key, field, err)
	}

	var node sortmistake.SortNode
	if err := proto.Unmarshal(bytesVal, &node); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SortNode protobuf from redis bytes: %w", err)
	}

	return &node, nil
}
