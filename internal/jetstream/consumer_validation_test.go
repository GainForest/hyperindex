package jetstream

import (
	"context"
	"testing"
	"time"

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

func TestConsumerHandleCommitStoresHiddenRecordsAndSuppressesTypedPubSub(t *testing.T) {
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
			assertNoSubscriptionEvent(t, subscriber.Events)
		})
	}
}

func assertNoSubscriptionEvent(t *testing.T, events <-chan *subscription.RecordEvent) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected typed subscription event: %#v", event)
	case <-time.After(25 * time.Millisecond):
	}
}
