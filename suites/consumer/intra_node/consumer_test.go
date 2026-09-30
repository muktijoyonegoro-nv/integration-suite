package intra_node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-suite/pkg/contract/fixtures"
	"integration-suite/pkg/testutil"
	protoSortMistake "integration-suite/proto/sortmistake"
	"integration-suite/suites/common"
)

func TestSortService_ConsumerEventIngestion(t *testing.T) {
	ctx := context.Background()

	scenario := common.Setup(t, scenarioConfig)

	redisSort := scenario.RedisClient("redis-sort")
	require.NotNil(t, redisSort, "redis-sort client must not be nil")

	// 1. Per-test state isolation
	require.NoError(t, scenario.TruncateTables(ctx), "tables must be clean")
	require.NoError(t, scenario.FlushRedis(ctx), "redis instances must be clean")

	systemID := "sg"
	hubID := int64(101)
	nodeID := int64(98765)
	nodeName := "CONSUMER_INJECTED_NODE"
	updatedNodeName := "CONSUMER_INJECTED_NODE_UPDATED"

	// =========================================================================
	// STEP 1: INJECT CONTRACT CREATED EVENT & ASSERT CONSUMER SYNC
	// =========================================================================
	t.Run("Step 1 - Consume Injected NODE_EVENT_CREATED", func(t *testing.T) {
		createdEvt := fixtures.NewSortNodeCreatedEvent(
			systemID,
			hubID,
			nodeID,
			nodeName,
			protoSortMistake.NodeType_NODE_TYPE_INTRA_MID,
		)

		err := testutil.PublishSortNodeEvents(ctx, scenario.KafkaBroker(), SortMistakeNodesTopic, createdEvt)
		require.NoError(t, err, "failed publishing synthetic created event to kafka")

		// Assert sort-service consumed the event and populated MySQL + Redis
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", nodeID)
			if err != nil || record == nil || record.Name != nodeName {
				return false
			}
			cachedProto, err := testutil.GetSortServiceIntraNode(ctx, redisSort, systemID, hubID, nodeID)
			if err != nil || cachedProto == nil || cachedProto.Name != nodeName {
				return false
			}
			return true
		}, 15*time.Second, 200*time.Millisecond, "sort-service must consume created event and sync DB + Redis")
	})

	// =========================================================================
	// STEP 2: INJECT CONTRACT UPDATED EVENT & ASSERT CONSUMER SYNC
	// =========================================================================
	t.Run("Step 2 - Consume Injected NODE_EVENT_UPDATED", func(t *testing.T) {
		updatedEvt := fixtures.NewSortNodeUpdatedEvent(
			systemID,
			hubID,
			nodeID,
			updatedNodeName,
			protoSortMistake.NodeType_NODE_TYPE_INTRA_MID,
		)

		err := testutil.PublishSortNodeEvents(ctx, scenario.KafkaBroker(), SortMistakeNodesTopic, updatedEvt)
		require.NoError(t, err, "failed publishing synthetic updated event to kafka")

		// Assert sort-service consumed the event and updated MySQL + Redis
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", nodeID)
			if err != nil || record == nil || record.Name != updatedNodeName {
				return false
			}
			cachedProto, err := testutil.GetSortServiceIntraNode(ctx, redisSort, systemID, hubID, nodeID)
			if err != nil || cachedProto == nil || cachedProto.Name != updatedNodeName {
				return false
			}
			return true
		}, 15*time.Second, 200*time.Millisecond, "sort-service must consume updated event and update DB + Redis")
	})

	// =========================================================================
	// STEP 3: INJECT CONTRACT DELETED EVENT & ASSERT CONSUMER EVICTION
	// =========================================================================
	t.Run("Step 3 - Consume Injected NODE_EVENT_DELETED", func(t *testing.T) {
		deletedEvt := fixtures.NewSortNodeDeletedEvent(
			systemID,
			hubID,
			nodeID,
		)

		err := testutil.PublishSortNodeEvents(ctx, scenario.KafkaBroker(), SortMistakeNodesTopic, deletedEvt)
		require.NoError(t, err, "failed publishing synthetic deleted event to kafka")

		// Assert sort-service consumed the event and evicted from MySQL + Redis
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", nodeID)
			if err != nil || record != nil {
				return false
			}
			cachedProto, err := testutil.GetSortServiceIntraNode(ctx, redisSort, systemID, hubID, nodeID)
			if err != nil || cachedProto != nil {
				return false
			}
			return true
		}, 15*time.Second, 200*time.Millisecond, "sort-service must consume deleted event and evict from DB + Redis")
	})

	// =========================================================================
	// STEP 4: CONSUMER IDEMPOTENCY / EDGE CASE VALIDATION
	// =========================================================================
	t.Run("Step 4 - Consumer Idempotency On Repeated Delete", func(t *testing.T) {
		// Re-publishing a delete event on an already deleted node must be idempotent and non-crashing
		duplicateDeleteEvt := fixtures.NewSortNodeDeletedEvent(
			systemID,
			hubID,
			nodeID,
		)

		err := testutil.PublishSortNodeEvents(ctx, scenario.KafkaBroker(), SortMistakeNodesTopic, duplicateDeleteEvt)
		require.NoError(t, err, "failed re-publishing duplicate delete event")

		// Wait briefly and verify DB and Redis remain empty and container remains healthy
		time.Sleep(1 * time.Second)
		record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", nodeID)
		require.NoError(t, err)
		assert.Nil(t, record, "record must remain deleted after repeated delete event")
	})
}
