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
	DID             string `json:"did"`
	Handle          string `json:"handle"`
	IsActive        bool   `json:"is_active"`
	IsActivePresent bool   `json:"-"`
	Status          string `json:"status"` // active, takendown, suspended, deactivated, deleted
}

// UnmarshalJSON parses the canonical Tap identity payload and records whether
// is_active was present so a missing boolean is never confused with false.
func (i *IdentityEvent) UnmarshalJSON(data []byte) error {
	var payload struct {
		DID      string `json:"did"`
		Handle   string `json:"handle"`
		IsActive *bool  `json:"is_active"`
		Status   string `json:"status"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*i = IdentityEvent{
		DID:             payload.DID,
		Handle:          payload.Handle,
		IsActivePresent: payload.IsActive != nil,
		Status:          payload.Status,
	}
	if payload.IsActive != nil {
		i.IsActive = *payload.IsActive
	}
	return nil
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
		// Note: create/update events may arrive without a record body (e.g. during
		// Tap backfill when the PDS record could not be fetched). These are valid
		// protocol events — the handler will skip them gracefully rather than
		// treating them as errors. Rejecting them here would cause an un-acked
		// delivery loop when TAP_DISABLE_ACKS=false.
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
