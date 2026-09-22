package intra_node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-suite/pkg/testutil"
	"integration-suite/proto/sortmistake"
	"integration-suite/suites/common"
)

func TestSortTaskPipeline_FullCRUDCycle(t *testing.T) {
	ctx := context.Background()

	scenario := common.Setup(t, scenarioConfig)

	sortMistakeBaseURL := scenario.Endpoint("sort-mistake")
	redisMistake := scenario.RedisClient("redis-sort-mistake")
	redisSort := scenario.RedisClient("redis-sort")
	require.NotNil(t, redisMistake, "redis-sort-mistake client must not be nil")
	require.NotNil(t, redisSort, "redis-sort client must not be nil")

	// 1. Per-test state isolation
	require.NoError(t, scenario.TruncateTables(ctx), "tables must be clean")
	require.NoError(t, scenario.FlushRedis(ctx), "redis instances must be clean")

	// 2. Setup Kafka Reader to monitor topic
	reader := testutil.NewKafkaReader(scenario.KafkaBroker(), SortMistakeNodesTopic, "pipeline-test-"+uuid.NewString())
	defer reader.Close()

	systemID := "sg"
	hubID := int64(101)
	nodeName := "STATION_ALPHA"
	updatedNodeName := "STATION_ALPHA_UPDATED"

	var createdNodeID int64

	// =========================================================================
	// STEP 1: CREATE SORT TASK
	// =========================================================================
	t.Run("Step 1 - Create Sort Task", func(t *testing.T) {
		// 1. Send HTTP POST to live sort-mistake API
		createPayload := map[string]interface{}{
			"name": nodeName,
			"type": "NODE_TYPE_INTRA_MID",
		}
		body, err := json.Marshal(createPayload)
		require.NoError(t, err)

		url := fmt.Sprintf("%s/1.0/intra/hubs/%d/nodes", sortMistakeBaseURL, hubID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-valid-token")
		req.Header.Set("x-nv-system-id", systemID)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode, "API call failed with body: %s", string(respBody))

		// 2. Assert message published on Kafka topic
		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		receivedEvent, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")
		require.Len(t, receivedEvent.Node, 1)

		createdNode := receivedEvent.Node[0]
		assert.Equal(t, sortmistake.NodeEvent_NODE_EVENT_CREATED, createdNode.NodeEvent)
		assert.Equal(t, systemID, createdNode.SystemId)
		assert.Equal(t, hubID, createdNode.HubId)
		assert.Equal(t, nodeName, createdNode.Name)

		createdNodeID = createdNode.Id

		// 3. Assert downstream sort-service automatically consumed event and synced DB + Redis
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", createdNodeID)
			if err != nil || record == nil || record.Name != nodeName {
				return false
			}
			cachedProto, err := testutil.GetSortServiceIntraNode(ctx, redisSort, systemID, hubID, createdNodeID)
			if err != nil || cachedProto == nil || cachedProto.Name != nodeName {
				return false
			}
			return true
		}, 15*time.Second, 200*time.Millisecond, "sort-service must consume create event and populate DB + Redis")
	})

	// Helper to create an intra-hub node through sort-mistake's live POST API,
	// consume the resulting creation event from Kafka, and ensure downstream DB sync.
	createNodeViaAPI := func(t *testing.T, name string) int64 {
		t.Helper()

		payload := map[string]interface{}{
			"name": name,
			"type": "NODE_TYPE_INTRA_MID",
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)

		url := fmt.Sprintf("%s/1.0/intra/hubs/%d/nodes", sortMistakeBaseURL, hubID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-valid-token")
		req.Header.Set("x-nv-system-id", systemID)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode, "API create failed with body: %s", string(respBody))

		// Read creation event from Kafka to advance reader offset and retrieve node ID
		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		evt, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")
		require.NotEmpty(t, evt.Node)
		nodeID := evt.Node[0].Id

		// Wait for downstream sort-service to sync before returning
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", nodeID)
			return err == nil && record != nil && record.Name == name
		}, 15*time.Second, 200*time.Millisecond, "sort-service must sync created node")

		return nodeID
	}

	// =========================================================================
	// STEP 2: UPDATE SORT TASK
	// =========================================================================
	t.Run("Step 2 - Update Sort Task", func(t *testing.T) {
		// 1. Prerequisite: Create node via live sort-mistake POST endpoint
		targetNodeID := createNodeViaAPI(t, "STATION_UPDATE_TARGET")
		require.NotEmpty(t, targetNodeID, "targetNodeID must be populated from prerequisite creation")

		// 2. Send HTTP PATCH to live sort-mistake API
		updatePayload := map[string]interface{}{
			"name": updatedNodeName,
		}
		body, err := json.Marshal(updatePayload)
		require.NoError(t, err)

		url := fmt.Sprintf("%s/1.0/intra/hubs/%d/nodes/%d", sortMistakeBaseURL, hubID, targetNodeID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-valid-token")
		req.Header.Set("x-nv-system-id", systemID)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		require.Contains(t, []int{http.StatusOK, http.StatusNoContent}, resp.StatusCode, "PATCH API call failed with body: %s", string(respBody))

		// 3. Assert update event published on Kafka topic
		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		receivedEvent, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")
		require.Len(t, receivedEvent.Node, 1)

		updatedNode := receivedEvent.Node[0]
		assert.Equal(t, sortmistake.NodeEvent_NODE_EVENT_UPDATED, updatedNode.NodeEvent)
		assert.Equal(t, targetNodeID, updatedNode.Id)
		assert.Equal(t, updatedNodeName, updatedNode.Name)

		// 4. Assert downstream sort-service automatically consumed update event and synced DB + Redis
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", targetNodeID)
			if err != nil || record == nil || record.Name != updatedNodeName {
				return false
			}
			cachedProto, err := testutil.GetSortServiceIntraNode(ctx, redisSort, systemID, hubID, targetNodeID)
			if err != nil || cachedProto == nil || cachedProto.Name != updatedNodeName {
				return false
			}
			return true
		}, 15*time.Second, 200*time.Millisecond, "sort-service must consume update event and update DB + Redis")
	})

	// =========================================================================
	// STEP 3: DELETE SORT TASK
	// =========================================================================
	t.Run("Step 3 - Delete Sort Task", func(t *testing.T) {
		// 1. Prerequisite: Create node via live sort-mistake POST endpoint
		targetNodeID := createNodeViaAPI(t, "STATION_DELETE_TARGET")
		require.NotEmpty(t, targetNodeID, "targetNodeID must be populated from prerequisite creation")

		// 2. Send HTTP DELETE to live sort-mistake API
		url := fmt.Sprintf("%s/1.0/intra/hubs/%d/nodes/%d", sortMistakeBaseURL, hubID, targetNodeID)
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer test-valid-token")
		req.Header.Set("x-nv-system-id", systemID)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		require.Contains(t, []int{http.StatusOK, http.StatusNoContent}, resp.StatusCode, "DELETE API call failed with body: %s", string(respBody))

		// 3. Assert delete event published on Kafka topic
		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		receivedEvent, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")
		require.Len(t, receivedEvent.Node, 1)

		deletedNode := receivedEvent.Node[0]
		assert.Equal(t, sortmistake.NodeEvent_NODE_EVENT_DELETED, deletedNode.NodeEvent)
		assert.Equal(t, targetNodeID, deletedNode.Id)

		// 4. Assert downstream sort-service automatically consumed delete event and evicted from DB + Redis
		assert.Eventually(t, func() bool {
			ssRecord, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_service", targetNodeID)
			if err != nil || ssRecord != nil {
				return false
			}
			smRecord, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_mistake", targetNodeID)
			if err != nil || smRecord != nil {
				return false
			}
			cachedProto, err := testutil.GetSortServiceIntraNode(ctx, redisSort, systemID, hubID, targetNodeID)
			if err != nil || cachedProto != nil {
				return false
			}
			return true
		}, 15*time.Second, 200*time.Millisecond, "sort-service and sort-mistake must evict deleted node from DB + Redis")
	})
}
