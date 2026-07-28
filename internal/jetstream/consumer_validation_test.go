package jetstream

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/graphql/subscription"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
)

type fakeRecordValidator struct {
	results map[string]validation.Result
}

func (v fakeRecordValidator) ValidateRecord(collection string, rkey string, rawJSON []byte) validation.Result {
	if result, ok := v.results[rkey]; ok {
		return result
	}
	return validation.Result{Status: validation.StatusValid, LexiconHash: "hash-current"}
}

func (v fakeRecordValidator) LexiconHash(collection string) (string, bool) {
	return "hash-current", true
}

func TestConsumerHandleCommitStoresHiddenRecordsAndPublishesRawEvents(t *testing.T) {
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	subscriber := pubsub.Subscribe("com.example.record")
	defer pubsub.Unsubscribe(subscriber)

	validator := fakeRecordValidator{results: map[string]validation.Result{
		"invalid": {
			Status:      validation.StatusInvalid,
			Error:       "missing required field: name",
			LexiconHash: "hash-current",
		},
		"unknown": {
			Status: validation.StatusUnknownSchema,
			Error:  "no saved lexicon for collection com.example.record",
		},
	}}
	consumer := NewConsumer(ConsumerConfig{}, db.Records, db.Actors, db.Config, db.Activity, pubsub, validator)

	tests := []struct {
		name       string
		rkey       string
		operation  OperationType
		cid        string
		recordJSON string
		wantStatus validation.Status
		wantError  string
		wantHash   string
	}{
		{
			name:       "invalid create",
			rkey:       "invalid",
			operation:  OpCreate,
			cid:        "cid-invalid",
			recordJSON: `{"$type":"com.example.record"}`,
			wantStatus: validation.StatusInvalid,
			wantError:  "missing required field: name",
			wantHash:   "hash-current",
		},
		{
			name:       "unknown-schema update",
			rkey:       "unknown",
			operation:  OpUpdate,
			cid:        "cid-unknown",
			recordJSON: `{"$type":"com.example.record","name":"unknown"}`,
			wantStatus: validation.StatusUnknownSchema,
			wantError:  "no saved lexicon for collection com.example.record",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &Event{
				DID:  "did:plc:test",
				Kind: EventTypeCommit,
				Commit: &CommitEvent{
					Operation:  tt.operation,
					Collection: "com.example.record",
					RKey:       tt.rkey,
					CID:        tt.cid,
					Record:     []byte(tt.recordJSON),
				},
			}
			if err := consumer.handleCommit(context.Background(), event); err != nil {
				t.Fatalf("handleCommit() error = %v", err)
			}

			uri := event.Commit.URI(event.DID)
			rec, err := db.Records.GetByURI(context.Background(), uri)
			if err != nil {
				t.Fatalf("GetByURI() error = %v", err)
			}
			if rec.ValidationStatus != tt.wantStatus {
				t.Fatalf("ValidationStatus = %q, want %q", rec.ValidationStatus, tt.wantStatus)
			}
			if rec.ValidationError != tt.wantError {
				t.Fatalf("ValidationError = %q, want %q", rec.ValidationError, tt.wantError)
			}
			if rec.LexiconHash != tt.wantHash {
				t.Fatalf("LexiconHash = %q, want %q", rec.LexiconHash, tt.wantHash)
			}
			if rec.ValidatedAt == nil {
				t.Fatal("ValidatedAt is nil, want validation timestamp")
			}
			assertRawSubscriptionEvent(t, subscriber.Events, subscription.EventType(tt.operation))
		})
	}
}

func TestConsumerHandleCommitSameCIDRepairsValidationMetadata(t *testing.T) {
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	subscriber := pubsub.Subscribe("com.example.record")
	defer pubsub.Unsubscribe(subscriber)
	consumer := NewConsumer(ConsumerConfig{}, db.Records, db.Actors, db.Config, nil, pubsub, fakeRecordValidator{})
	ctx := context.Background()
	uri := "at://did:plc:test/com.example.record/repair"
	recordJSON := `{"$type":"com.example.record","name":"same"}`
	if _, err := db.Records.Insert(ctx, uri, "cid-same", "did:plc:test", "com.example.record", recordJSON); err != nil {
		t.Fatalf("legacy Insert() error = %v", err)
	}

	event := &Event{DID: "did:plc:test", Kind: EventTypeCommit, Commit: &CommitEvent{
		Operation: OpUpdate, Collection: "com.example.record", RKey: "repair", CID: "cid-same", Record: []byte(recordJSON),
	}}
	if err := consumer.handleCommit(ctx, event); err != nil {
		t.Fatalf("handleCommit() error = %v", err)
	}
	stored, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if stored.ValidationStatus != validation.StatusValid || stored.LexiconHash != "hash-current" || stored.ValidatedAt == nil {
		t.Fatalf("same-CID repair = status:%q hash:%q at:%v", stored.ValidationStatus, stored.LexiconHash, stored.ValidatedAt)
	}
	assertRawSubscriptionEvent(t, subscriber.Events, subscription.EventUpdate)
}

func TestConsumerHandleCommitNilValidatorFailsClosed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	consumer := NewConsumer(ConsumerConfig{}, db.Records, db.Actors, db.Config, nil, nil)
	event := &Event{DID: "did:plc:test", Kind: EventTypeCommit, Commit: &CommitEvent{
		Operation: OpCreate, Collection: "com.example.record", RKey: "nil-validator", CID: "cid-nil", Record: []byte(`{"name":"test"}`),
	}}
	if err := consumer.handleCommit(context.Background(), event); err != nil {
		t.Fatalf("handleCommit() error = %v", err)
	}
	stored, err := db.Records.GetByURI(context.Background(), event.Commit.URI(event.DID))
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if stored.ValidationStatus != validation.StatusValidationError || stored.ValidationError == "" || stored.ValidatedAt == nil {
		t.Fatalf("nil-validator result = status:%q error:%q at:%v", stored.ValidationStatus, stored.ValidationError, stored.ValidatedAt)
	}
}

func TestConsumerHandleCommitDeleteCarriesPreDeleteVisibility(t *testing.T) {
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	subscriber := pubsub.Subscribe("com.example.record")
	defer pubsub.Unsubscribe(subscriber)
	consumer := NewConsumer(ConsumerConfig{}, db.Records, db.Actors, db.Config, nil, pubsub, fakeRecordValidator{})
	ctx := context.Background()
	create := &Event{DID: "did:plc:test", Kind: EventTypeCommit, Commit: &CommitEvent{
		Operation: OpCreate, Collection: "com.example.record", RKey: "delete", CID: "cid-delete", Record: []byte(`{"$type":"com.example.record","name":"delete"}`),
	}}
	if err := consumer.handleCommit(ctx, create); err != nil {
		t.Fatalf("handleCommit(create) error = %v", err)
	}
	<-subscriber.Events

	deleteEvent := &Event{DID: "did:plc:test", Kind: EventTypeCommit, Commit: &CommitEvent{
		Operation: OpDelete, Collection: "com.example.record", RKey: "delete",
	}}
	if err := consumer.handleCommit(ctx, deleteEvent); err != nil {
		t.Fatalf("handleCommit(delete) error = %v", err)
	}
	select {
	case event := <-subscriber.Events:
		if event.Type != subscription.EventDelete || !event.WasValid || event.TypedRecord == nil {
			t.Fatalf("delete event = type:%q wasValid:%v typedRecord:%v", event.Type, event.WasValid, event.TypedRecord)
		}
	default:
		t.Fatal("missing raw delete event")
	}
}

func assertRawSubscriptionEvent(t *testing.T, events <-chan *subscription.RecordEvent, want subscription.EventType) {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != want {
			t.Fatalf("raw subscription event type = %q, want %q", event.Type, want)
		}
	default:
		t.Fatal("missing raw subscription event")
	}
}
