// Package subscription provides GraphQL subscription support via pub/sub.
package subscription

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"sync"
)

// EventType represents the type of record event.
type EventType string

const (
	// EventCreate indicates a new record was created.
	EventCreate EventType = "create"
	// EventUpdate indicates a record was updated.
	EventUpdate EventType = "update"
	// EventDelete indicates a record was deleted.
	EventDelete EventType = "delete"

	// SubscriberBufferSize is the per-subscriber event channel buffer.
	// PubSub delivery is best-effort and lossy: events are dropped rather than
	// blocking the publisher when a matching subscriber's buffer is full.
	SubscriberBufferSize = 100
)

// RecordEvent represents a record change event.
type RecordEvent struct {
	Type       EventType   `json:"type"`
	URI        string      `json:"uri"`
	CID        string      `json:"cid"`
	DID        string      `json:"did"`
	Collection string      `json:"collection"`
	Record     interface{} `json:"record,omitempty"`

	// TypedRecord carries the validated create/update record or the previously
	// visible record for a delete. It is internal subscription-routing metadata.
	TypedRecord  map[string]interface{} `json:"-"`
	TypedVisible bool                   `json:"-"`
	WasValid     bool                   `json:"-"`
}

// Subscriber is a channel that receives events.
type Subscriber struct {
	ID         string
	Collection string // Empty means all collections
	Events     chan *RecordEvent
}

// PubSub manages subscriptions and event broadcasting.
type PubSub struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber
	nextID      int64
}

// NewPubSub creates a new pub/sub instance.
func NewPubSub() *PubSub {
	return &PubSub{
		subscribers: make(map[string]*Subscriber),
	}
}

// Subscribe creates a new subscription.
// Returns a subscriber with a channel that receives events.
// If collection is empty, subscribes to all collections.
func (ps *PubSub) Subscribe(collection string) *Subscriber {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.nextID++
	id := strconv.FormatInt(ps.nextID, 10)

	sub := &Subscriber{
		ID:         id,
		Collection: collection,
		Events:     make(chan *RecordEvent, SubscriberBufferSize),
	}

	ps.subscribers[id] = sub
	return sub
}

// Unsubscribe removes a subscription.
func (ps *PubSub) Unsubscribe(sub *Subscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, ok := ps.subscribers[sub.ID]; ok {
		close(sub.Events)
		delete(ps.subscribers, sub.ID)
	}
}

// Publish sends an event to all matching subscribers.
func (ps *PubSub) Publish(event *RecordEvent) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	for _, sub := range ps.subscribers {
		// Check if subscriber wants this collection
		if sub.Collection != "" && sub.Collection != event.Collection {
			continue
		}

		// Non-blocking send (drop if buffer full)
		select {
		case sub.Events <- event:
		default:
			slog.Debug("Subscription event dropped, buffer full",
				"subscriber", sub.ID,
				"collection", event.Collection,
			)
		}
	}
}

// RawGraphQLValue returns the generic recordEvents payload. Internal typed
// visibility metadata is intentionally excluded.
func (event *RecordEvent) RawGraphQLValue() map[string]interface{} {
	return map[string]interface{}{
		"type":       string(event.Type),
		"uri":        event.URI,
		"cid":        event.CID,
		"did":        event.DID,
		"collection": event.Collection,
		"record":     event.Record,
	}
}

// PublishRecord publishes a raw create/update event that is eligible for typed
// delivery. Ingestion should use PublishRecordWithValidation with the actual
// classification result.
func (ps *PubSub) PublishRecord(eventType EventType, uri, cid, did, collection string, recordJSON []byte) {
	ps.PublishRecordWithValidation(eventType, uri, cid, did, collection, recordJSON, true)
}

// PublishRecordWithValidation publishes every raw create/update event and
// carries its write-time typed visibility to prevent later URI updates from
// changing eligibility for an earlier queued event.
func (ps *PubSub) PublishRecordWithValidation(eventType EventType, uri, cid, did, collection string, recordJSON []byte, typedVisible bool) {
	ps.Publish(&RecordEvent{
		Type:         eventType,
		URI:          uri,
		CID:          cid,
		DID:          did,
		Collection:   collection,
		Record:       decodeRawSubscriptionRecord(recordJSON),
		TypedRecord:  decodeTypedSubscriptionRecord(recordJSON, uri, cid),
		TypedVisible: typedVisible,
	})
}

// PublishDelete publishes a raw delete and carries the pre-delete typed record
// only for typed subscription eligibility and payload resolution.
func (ps *PubSub) PublishDelete(uri, cid, did, collection string, previousRecordJSON []byte, wasValid bool) {
	ps.Publish(&RecordEvent{
		Type:        EventDelete,
		URI:         uri,
		CID:         cid,
		DID:         did,
		Collection:  collection,
		TypedRecord: decodeTypedSubscriptionRecord(previousRecordJSON, uri, cid),
		WasValid:    wasValid,
	})
}

func decodeRawSubscriptionRecord(recordJSON []byte) interface{} {
	if len(recordJSON) == 0 {
		return nil
	}
	var record interface{}
	if err := json.Unmarshal(recordJSON, &record); err != nil {
		return nil
	}
	return record
}

func decodeTypedSubscriptionRecord(recordJSON []byte, uri, cid string) map[string]interface{} {
	if len(recordJSON) == 0 {
		return nil
	}
	var record map[string]interface{}
	if err := json.Unmarshal(recordJSON, &record); err != nil || record == nil {
		return nil
	}
	record["uri"] = uri
	record["cid"] = cid
	return record
}

// SubscriberCount returns the current number of subscribers.
func (ps *PubSub) SubscriberCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.subscribers)
}
