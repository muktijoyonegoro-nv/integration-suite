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

	"integration-suite/pkg/contract/sortmistake"
	"integration-suite/pkg/testutil"
	protoSortMistake "integration-suite/proto/sortmistake"
	"integration-suite/suites/common"
)

func TestSortMistake_ProducerContractPipeline(t *testing.T) {
	ctx := context.Background()

	scenario := common.Setup(t, scenarioConfig)

	sortMistakeBaseURL := scenario.Endpoint("sort-mistake")
	redisMistake := scenario.RedisClient("redis-sort-mistake")
	require.NotNil(t, redisMistake, "redis-sort-mistake client must not be nil")

	// 1. Per-test state isolation
	require.NoError(t, scenario.TruncateTables(ctx), "tables must be clean")
	require.NoError(t, scenario.FlushRedis(ctx), "redis instances must be clean")

	// 2. Setup Kafka Reader to monitor topic
	reader := testutil.NewKafkaReader(scenario.KafkaBroker(), SortMistakeNodesTopic, "producer-test-"+uuid.NewString())
	defer reader.Close()

	systemID := "sg"
	hubID := int64(101)
	nodeName := "PRODUCER_STATION_ALPHA"
	updatedNodeName := "PRODUCER_STATION_ALPHA_UPDATED"

	var createdNodeID int64

	// =========================================================================
	// STEP 1: CREATE NODE VIA API & ASSERT CONTRACT
	// =========================================================================
	t.Run("Step 1 - Create Node & Verify Kafka Contract", func(t *testing.T) {
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

		// Read and assert message satisfies schema & data contract
		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		receivedEvent, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")

		// Strict contract validation
		createdNodeID = sortmistake.AssertNodeCreatedContract(
			t,
			receivedEvent,
			systemID,
			hubID,
			nodeName,
			protoSortMistake.NodeType_NODE_TYPE_INTRA_MID,
		)
		require.Positive(t, createdNodeID, "created node ID must be positive")

		// Assert producer's internal MySQL state
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_mistake", createdNodeID)
			return err == nil && record != nil && record.Name == nodeName
		}, 10*time.Second, 200*time.Millisecond, "sort_mistake must persist created node in DB")
	})

	// Helper to create an intra-hub node through sort-mistake's live POST API
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

		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		evt, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")

		return sortmistake.AssertNodeCreatedContract(
			t,
			evt,
			systemID,
			hubID,
			name,
			protoSortMistake.NodeType_NODE_TYPE_INTRA_MID,
		)
	}

	// =========================================================================
	// STEP 2: UPDATE NODE VIA API & ASSERT CONTRACT
	// =========================================================================
	t.Run("Step 2 - Update Node & Verify Kafka Contract", func(t *testing.T) {
		targetNodeID := createNodeViaAPI(t, "PRODUCER_UPDATE_TARGET")
		require.Positive(t, targetNodeID, "targetNodeID must be populated")

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

		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		receivedEvent, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")

		// Strict contract validation
		sortmistake.AssertNodeUpdatedContract(
			t,
			receivedEvent,
			systemID,
			hubID,
			targetNodeID,
			updatedNodeName,
		)

		// Assert producer's internal MySQL state
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_mistake", targetNodeID)
			return err == nil && record != nil && record.Name == updatedNodeName
		}, 10*time.Second, 200*time.Millisecond, "sort_mistake must update node in DB")
	})

	// =========================================================================
	// STEP 3: DELETE NODE VIA API & ASSERT CONTRACT
	// =========================================================================
	t.Run("Step 3 - Delete Node & Verify Kafka Contract", func(t *testing.T) {
		targetNodeID := createNodeViaAPI(t, "PRODUCER_DELETE_TARGET")
		require.Positive(t, targetNodeID, "targetNodeID must be populated")

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

		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()

		receivedEvent, err := testutil.ReadNextSortNodeEvent(readCtx, reader)
		require.NoError(t, err, "should read SortNodeEvents from Kafka")

		// Strict contract validation
		sortmistake.AssertNodeDeletedContract(
			t,
			receivedEvent,
			systemID,
			hubID,
			targetNodeID,
		)

		// Assert producer's internal MySQL state
		assert.Eventually(t, func() bool {
			record, err := testutil.QueryIntraHubNode(ctx, scenario.DB(), "sort_mistake", targetNodeID)
			return err == nil && record == nil
		}, 10*time.Second, 200*time.Millisecond, "sort_mistake must delete node from DB")
	})
}
