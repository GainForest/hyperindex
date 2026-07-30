// Package subscription provides GraphQL subscription support via pub/sub.
package subscription

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
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
	// PubSub drops events (non-blocking) for slow subscribers to avoid
	// blocking the publisher. Subscribers reconnect to catch up.
	SubscriberBufferSize = 100
)

// RecordEvent represents a record change event.
type RecordEvent struct {
	Type       EventType              `json:"type"`
	URI        string                 `json:"uri"`
	CID        string                 `json:"cid"`
	DID        string                 `json:"did"`
	Collection string                 `json:"collection"`
	Record     map[string]interface{} `json:"record,omitempty"`
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

// PublishRecord is a convenience method to publish a record event.
func (ps *PubSub) PublishRecord(eventType EventType, uri, cid, did, collection string, recordJSON []byte) {
	var record map[string]interface{}
	if len(recordJSON) > 0 && eventType != EventDelete {
		_ = json.Unmarshal(recordJSON, &record)
	}

	// Add standard fields
	if record != nil {
		record["uri"] = uri
		record["cid"] = cid
		record["did"] = did
		if slash := strings.LastIndex(uri, "/"); slash >= 0 && slash+1 < len(uri) {
			record["rkey"] = uri[slash+1:]
		}
	}

	ps.Publish(&RecordEvent{
		Type:       eventType,
		URI:        uri,
		CID:        cid,
		DID:        did,
		Collection: collection,
		Record:     record,
	})
}

// SubscriberCount returns the current number of subscribers.
func (ps *PubSub) SubscriberCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.subscribers)
}
