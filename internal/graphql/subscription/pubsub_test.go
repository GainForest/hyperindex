package subscription

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestPubSub_SubscribeUnsubscribe(t *testing.T) {
	ps := NewPubSub()

	// Subscribe
	sub := ps.Subscribe("")
	if sub == nil {
		t.Fatal("Subscribe returned nil")
		return
	}

	if ps.SubscriberCount() != 1 {
		t.Errorf("SubscriberCount = %d, want 1", ps.SubscriberCount())
	}

	// Unsubscribe
	ps.Unsubscribe(sub)

	if ps.SubscriberCount() != 0 {
		t.Errorf("SubscriberCount after unsubscribe = %d, want 0", ps.SubscriberCount())
	}

	// Channel should be closed
	_, ok := <-sub.Events
	if ok {
		t.Error("Expected channel to be closed")
	}
}

func TestPubSub_Publish(t *testing.T) {
	ps := NewPubSub()
	sub := ps.Subscribe("")

	event := &RecordEvent{
		Type:       EventCreate,
		URI:        "at://did:plc:test/org.example.record/123",
		CID:        "bafytest",
		DID:        "did:plc:test",
		Collection: "org.example.record",
		Record:     map[string]interface{}{"title": "Test"},
	}

	// Publish in goroutine
	go func() {
		ps.Publish(event)
	}()

	// Receive with timeout
	select {
	case received := <-sub.Events:
		if received.URI != event.URI {
			t.Errorf("URI = %s, want %s", received.URI, event.URI)
		}
		if received.Type != EventCreate {
			t.Errorf("Type = %s, want %s", received.Type, EventCreate)
		}
	case <-time.After(time.Second):
		t.Error("Timed out waiting for event")
	}

	ps.Unsubscribe(sub)
}

func TestPubSub_CollectionFilter(t *testing.T) {
	ps := NewPubSub()

	// Subscribe to specific collection
	sub := ps.Subscribe("org.hypercerts.claim.activity")

	// Publish matching event
	matching := &RecordEvent{
		Type:       EventCreate,
		Collection: "org.hypercerts.claim.activity",
	}
	ps.Publish(matching)

	// Publish non-matching event
	nonMatching := &RecordEvent{
		Type:       EventCreate,
		Collection: "org.other.collection",
	}
	ps.Publish(nonMatching)

	// Should only receive matching event
	select {
	case received := <-sub.Events:
		if received.Collection != "org.hypercerts.claim.activity" {
			t.Errorf("Received wrong collection: %s", received.Collection)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive matching event")
	}

	// Should not receive non-matching event
	select {
	case <-sub.Events:
		t.Error("Should not receive non-matching event")
	case <-time.After(100 * time.Millisecond):
		// Good - no event received
	}

	ps.Unsubscribe(sub)
}

func TestPubSub_MultipleSubscribers(t *testing.T) {
	ps := NewPubSub()

	sub1 := ps.Subscribe("")
	sub2 := ps.Subscribe("")
	sub3 := ps.Subscribe("specific.collection")

	if ps.SubscriberCount() != 3 {
		t.Errorf("SubscriberCount = %d, want 3", ps.SubscriberCount())
	}

	event := &RecordEvent{
		Type:       EventUpdate,
		Collection: "other.collection",
	}
	ps.Publish(event)

	// sub1 and sub2 should receive (no filter)
	for _, sub := range []*Subscriber{sub1, sub2} {
		select {
		case <-sub.Events:
			// Good
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected subscriber to receive event")
		}
	}

	// sub3 should not receive (filter doesn't match)
	select {
	case <-sub3.Events:
		t.Error("Filtered subscriber should not receive non-matching event")
	case <-time.After(50 * time.Millisecond):
		// Good
	}

	ps.Unsubscribe(sub1)
	ps.Unsubscribe(sub2)
	ps.Unsubscribe(sub3)
}

func TestPubSub_ConcurrentPublish(t *testing.T) {
	ps := NewPubSub()
	sub := ps.Subscribe("")

	const numEvents = 100
	var wg sync.WaitGroup

	// Concurrent publishers
	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ps.Publish(&RecordEvent{
				Type: EventCreate,
				URI:  "at://test/" + strconv.Itoa(n),
			})
		}(i)
	}

	// Collect events
	received := 0
	done := make(chan bool)
	go func() {
		for range sub.Events {
			received++
			if received >= numEvents {
				done <- true
				return
			}
		}
	}()

	wg.Wait()

	select {
	case <-done:
		// Good - received all events
	case <-time.After(2 * time.Second):
		t.Errorf("Only received %d of %d events", received, numEvents)
	}

	ps.Unsubscribe(sub)
}

func TestPublishRecord(t *testing.T) {
	ps := NewPubSub()
	sub := ps.Subscribe("")

	jsonData := []byte(`{"title": "Test Record", "count": 42}`)
	ps.PublishRecord(EventCreate, "at://did:plc:test/col/123", "bafytest", "did:plc:test", "col", jsonData)

	select {
	case event := <-sub.Events:
		if event.Type != EventCreate {
			t.Errorf("Type = %s, want create", event.Type)
		}
		if event.URI != "at://did:plc:test/col/123" {
			t.Errorf("URI = %s", event.URI)
		}
		rawRecord, ok := event.Record.(map[string]interface{})
		if !ok {
			t.Fatalf("Record = %T, want object", event.Record)
		}
		if rawRecord["title"] != "Test Record" {
			t.Errorf("title = %v", rawRecord["title"])
		}
		if _, exists := rawRecord["uri"]; exists {
			t.Fatalf("raw record unexpectedly contains injected metadata: %#v", rawRecord)
		}
		event.TypedRecord["title"] = "coerced typed value"
		if rawRecord["title"] != "Test Record" {
			t.Fatalf("typed payload mutation changed raw record: %#v", rawRecord)
		}
	case <-time.After(time.Second):
		t.Error("Timed out waiting for event")
	}

	ps.Unsubscribe(sub)
}

func TestPublishRecord_Delete(t *testing.T) {
	ps := NewPubSub()
	sub := ps.Subscribe("")

	// Delete events should have nil record
	ps.PublishRecord(EventDelete, "at://did:plc:test/col/123", "bafytest", "did:plc:test", "col", nil)

	select {
	case event := <-sub.Events:
		if event.Type != EventDelete {
			t.Errorf("Type = %s, want delete", event.Type)
		}
		if event.Record != nil {
			t.Error("Delete event should have nil record")
		}
	case <-time.After(time.Second):
		t.Error("Timed out waiting for event")
	}

	ps.Unsubscribe(sub)
}

func TestPublishDeleteCarriesTypedVisibility(t *testing.T) {
	ps := NewPubSub()
	sub := ps.Subscribe("")
	defer ps.Unsubscribe(sub)

	ps.PublishDelete(
		"at://did:plc:test/col/123",
		"bafytest",
		"did:plc:test",
		"col",
		[]byte(`{"title":"Deleted Record"}`),
		true,
	)

	select {
	case event := <-sub.Events:
		if event.Record != nil {
			t.Fatalf("raw delete Record = %#v, want nil", event.Record)
		}
		if !event.WasValid || event.TypedRecord == nil || event.TypedRecord["title"] != "Deleted Record" {
			t.Fatalf("typed delete metadata = wasValid:%v record:%#v", event.WasValid, event.TypedRecord)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delete event")
	}
}

func TestNewPubSub(t *testing.T) {
	ps := NewPubSub()
	if ps == nil {
		t.Fatal("NewPubSub() returned nil")
	}

	// Each call should return a distinct instance
	ps2 := NewPubSub()
	if ps == ps2 {
		t.Error("NewPubSub() should return distinct instances")
	}
}
