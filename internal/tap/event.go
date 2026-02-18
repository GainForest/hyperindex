// Package tap provides a client for Bluesky's Tap sync utility.
package tap

import (
	"encoding/json"
	"fmt"
)

// EventType is the top-level event type from Tap.
type EventType string

const (
	EventTypeRecord   EventType = "record"
	EventTypeIdentity EventType = "identity"
)

// ActionType is the record action type.
type ActionType string

const (
	ActionCreate ActionType = "create"
	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
)

// Event is the top-level Tap event envelope.
type Event struct {
	ID       int64          `json:"id"`
	Type     EventType      `json:"type"`
	Record   *RecordEvent   `json:"record,omitempty"`
	Identity *IdentityEvent `json:"identity,omitempty"`
}

// RecordEvent is a record change event from Tap.
type RecordEvent struct {
	Live       bool            `json:"live"`
	Rev        string          `json:"rev"`
	DID        string          `json:"did"`
	Collection string          `json:"collection"`
	RKey       string          `json:"rkey"`
	Action     ActionType      `json:"action"`
	CID        string          `json:"cid,omitempty"`
	Record     json.RawMessage `json:"record,omitempty"` // Only for create/update
}

// URI returns the AT-URI for this record event.
func (r *RecordEvent) URI() string {
	return fmt.Sprintf("at://%s/%s/%s", r.DID, r.Collection, r.RKey)
}

// IdentityEvent is an identity change event from Tap.
type IdentityEvent struct {
	DID      string `json:"did"`
	Handle   string `json:"handle"`
	IsActive bool   `json:"isActive"`
	Status   string `json:"status"` // active, takendown, suspended, deactivated, deleted
}

// ParseEvent parses a Tap event from JSON bytes.
func ParseEvent(data []byte) (*Event, error) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to parse tap event: %w", err)
	}
	if event.Type == "" {
		return nil, fmt.Errorf("tap event missing type field")
	}
	// Validate record events.
	if event.Type == EventTypeRecord {
		if event.Record == nil {
			return nil, fmt.Errorf("tap record event missing record payload")
		}
		if event.Record.DID == "" {
			return nil, fmt.Errorf("tap record event missing did field")
		}
		if event.Record.Collection == "" {
			return nil, fmt.Errorf("tap record event missing collection field")
		}
		if event.Record.RKey == "" {
			return nil, fmt.Errorf("tap record event missing rkey field")
		}
		if event.Record.Action == "" {
			return nil, fmt.Errorf("tap record event missing action field")
		}
		if event.Record.Action == ActionCreate || event.Record.Action == ActionUpdate {
			if len(event.Record.Record) == 0 {
				return nil, fmt.Errorf("tap record event action %q missing record body", event.Record.Action)
			}
		}
	}
	// Validate identity events.
	if event.Type == EventTypeIdentity {
		if event.Identity == nil {
			return nil, fmt.Errorf("tap identity event missing identity payload")
		}
		if event.Identity.DID == "" {
			return nil, fmt.Errorf("tap identity event missing did field")
		}
	}
	return &event, nil
}

// IsRecord returns true if this is a record event.
func (e *Event) IsRecord() bool {
	return e.Type == EventTypeRecord && e.Record != nil
}

// IsIdentity returns true if this is an identity event.
func (e *Event) IsIdentity() bool {
	return e.Type == EventTypeIdentity && e.Identity != nil
}
