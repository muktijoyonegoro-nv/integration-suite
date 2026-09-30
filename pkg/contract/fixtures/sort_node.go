package fixtures

import (
	"integration-suite/proto/sortmistake"
)

// NewSortNodeCreatedEvent creates a contract-compliant SortNodeEvents message for node creation.
func NewSortNodeCreatedEvent(systemID string, hubID, nodeID int64, name string, nodeType sortmistake.NodeType) *sortmistake.SortNodeEvents {
	return &sortmistake.SortNodeEvents{
		SystemId: systemID,
		Type:     sortmistake.NodeEventType_NODE_EVENT_TYPE_INTERNAL,
		Node: []*sortmistake.SortNode{
			{
				Id:        nodeID,
				RealId:    nodeID,
				HubId:     hubID,
				Name:      name,
				Type:      nodeType,
				SystemId:  systemID,
				NodeEvent: sortmistake.NodeEvent_NODE_EVENT_CREATED,
			},
		},
	}
}

// NewSortNodeUpdatedEvent creates a contract-compliant SortNodeEvents message for node update.
func NewSortNodeUpdatedEvent(systemID string, hubID, nodeID int64, name string, nodeType sortmistake.NodeType) *sortmistake.SortNodeEvents {
	return &sortmistake.SortNodeEvents{
		SystemId: systemID,
		Type:     sortmistake.NodeEventType_NODE_EVENT_TYPE_INTERNAL,
		Node: []*sortmistake.SortNode{
			{
				Id:        nodeID,
				RealId:    nodeID,
				HubId:     hubID,
				Name:      name,
				Type:      nodeType,
				SystemId:  systemID,
				NodeEvent: sortmistake.NodeEvent_NODE_EVENT_UPDATED,
			},
		},
	}
}

// NewSortNodeDeletedEvent creates a contract-compliant SortNodeEvents message for node deletion.
func NewSortNodeDeletedEvent(systemID string, hubID, nodeID int64) *sortmistake.SortNodeEvents {
	return &sortmistake.SortNodeEvents{
		SystemId: systemID,
		Type:     sortmistake.NodeEventType_NODE_EVENT_TYPE_INTERNAL,
		Node: []*sortmistake.SortNode{
			{
				Id:        nodeID,
				RealId:    nodeID,
				HubId:     hubID,
				SystemId:  systemID,
				NodeEvent: sortmistake.NodeEvent_NODE_EVENT_DELETED,
			},
		},
	}
}
