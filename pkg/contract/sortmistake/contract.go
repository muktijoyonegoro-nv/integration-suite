package sortmistake

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"integration-suite/proto/sortmistake"
)

// ExpectedSortNode defines expected contract attributes for a SortNode event message.
type ExpectedSortNode struct {
	ID        int64
	HubID     int64
	SystemID  string
	Name      string
	Type      sortmistake.NodeType
	NodeEvent sortmistake.NodeEvent
}

// ValidateSortNodeEventSchema performs strict contract verification on a SortNode message.
// It checks wire formatting, mandatory schema fields, and semantic enum ranges.
func ValidateSortNodeEventSchema(node *sortmistake.SortNode, expectedEvent sortmistake.NodeEvent) error {
	if node == nil {
		return errors.New("contract violation: SortNode is nil")
	}

	if node.Id <= 0 {
		return fmt.Errorf("contract violation: invalid node.Id (%d), must be positive integer", node.Id)
	}

	if node.NodeEvent == sortmistake.NodeEvent_NODE_EVENT_UNSPECIFIED {
		return errors.New("contract violation: node_event is NODE_EVENT_UNSPECIFIED")
	}

	if expectedEvent != sortmistake.NodeEvent_NODE_EVENT_UNSPECIFIED && node.NodeEvent != expectedEvent {
		return fmt.Errorf("contract violation: expected node_event %v, got %v", expectedEvent, node.NodeEvent)
	}

	// For CREATED and UPDATED events, name, type, and hub_id are mandatory invariants
	if node.NodeEvent == sortmistake.NodeEvent_NODE_EVENT_CREATED || node.NodeEvent == sortmistake.NodeEvent_NODE_EVENT_UPDATED {
		if node.HubId <= 0 {
			return fmt.Errorf("contract violation: invalid hub_id (%d), must be positive integer", node.HubId)
		}
		if node.Name == "" {
			return errors.New("contract violation: node name cannot be empty")
		}
		if node.Type == sortmistake.NodeType_NODE_TYPE_UNSPECIFIED {
			return errors.New("contract violation: node type cannot be NODE_TYPE_UNSPECIFIED")
		}
	}

	return nil
}

// AssertSortNodeEventsContract validates that a published SortNodeEvents container message
// satisfies all producer-consumer contract requirements.
func AssertSortNodeEventsContract(t *testing.T, events *sortmistake.SortNodeEvents, expected ExpectedSortNode) {
	t.Helper()

	require.NotNil(t, events, "SortNodeEvents must not be nil")
	require.NotEmpty(t, events.SystemId, "contract violation: events.system_id must not be empty")

	if expected.SystemID != "" {
		assert.Equal(t, expected.SystemID, events.SystemId, "contract violation: system_id mismatch")
	}

	require.NotEmpty(t, events.Node, "contract violation: events.node list must contain at least one node")
	targetNode := events.Node[0]

	err := ValidateSortNodeEventSchema(targetNode, expected.NodeEvent)
	require.NoError(t, err, "SortNode schema contract validation failed")

	if expected.ID > 0 {
		assert.Equal(t, expected.ID, targetNode.Id, "contract violation: node.id mismatch")
	}
	if expected.HubID > 0 {
		assert.Equal(t, expected.HubID, targetNode.HubId, "contract violation: node.hub_id mismatch")
	}
	if expected.Name != "" {
		assert.Equal(t, expected.Name, targetNode.Name, "contract violation: node.name mismatch")
	}
	if expected.Type != sortmistake.NodeType_NODE_TYPE_UNSPECIFIED {
		assert.Equal(t, expected.Type, targetNode.Type, "contract violation: node.type mismatch")
	}
	assert.Equal(t, expected.NodeEvent, targetNode.NodeEvent, "contract violation: node.node_event mismatch")
}

// AssertNodeCreatedContract is a specialized contract assertion for NODE_EVENT_CREATED.
// Returns the allocated node ID.
func AssertNodeCreatedContract(t *testing.T, events *sortmistake.SortNodeEvents, systemID string, hubID int64, name string, nodeType sortmistake.NodeType) int64 {
	t.Helper()
	AssertSortNodeEventsContract(t, events, ExpectedSortNode{
		HubID:     hubID,
		SystemID:  systemID,
		Name:      name,
		Type:      nodeType,
		NodeEvent: sortmistake.NodeEvent_NODE_EVENT_CREATED,
	})
	return events.Node[0].Id
}

// AssertNodeUpdatedContract is a specialized contract assertion for NODE_EVENT_UPDATED.
func AssertNodeUpdatedContract(t *testing.T, events *sortmistake.SortNodeEvents, systemID string, hubID, nodeID int64, name string) {
	t.Helper()
	AssertSortNodeEventsContract(t, events, ExpectedSortNode{
		ID:        nodeID,
		HubID:     hubID,
		SystemID:  systemID,
		Name:      name,
		NodeEvent: sortmistake.NodeEvent_NODE_EVENT_UPDATED,
	})
}

// AssertNodeDeletedContract is a specialized contract assertion for NODE_EVENT_DELETED.
func AssertNodeDeletedContract(t *testing.T, events *sortmistake.SortNodeEvents, systemID string, hubID, nodeID int64) {
	t.Helper()
	AssertSortNodeEventsContract(t, events, ExpectedSortNode{
		ID:        nodeID,
		HubID:     hubID,
		SystemID:  systemID,
		NodeEvent: sortmistake.NodeEvent_NODE_EVENT_DELETED,
	})
}
