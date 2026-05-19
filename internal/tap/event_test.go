package tap

import (
	"encoding/json"
	"testing"
)

func TestParseEvent_RecordCreate(t *testing.T) {
	data := []byte(`{
		"id": 12345,
		"type": "record",
		"record": {
			"live": true,
			"rev": "3kgn2v3",
			"did": "did:plc:abc",
			"collection": "app.bsky.feed.post",
			"rkey": "abc",
			"action": "create",
			"cid": "bafyrei123",
			"record": {"text": "hello"}
		}
	}`)

	event, err := ParseEvent(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ID != 12345 {
		t.Errorf("expected ID 12345, got %d", event.ID)
	}
	if event.Type != EventTypeRecord {
		t.Errorf("expected type %q, got %q", EventTypeRecord, event.Type)
	}
	if event.Record == nil {
		t.Fatal("expected record to be non-nil")
	}
	if event.Record.EventID != event.ID {
		t.Errorf("expected record EventID %d, got %d", event.ID, event.Record.EventID)
	}
	if !event.Record.Live {
		t.Error("expected live to be true")
	}
	if event.Record.Rev != "3kgn2v3" {
		t.Errorf("expected rev %q, got %q", "3kgn2v3", event.Record.Rev)
	}
	if event.Record.DID != "did:plc:abc" {
		t.Errorf("expected DID %q, got %q", "did:plc:abc", event.Record.DID)
	}
	if event.Record.Collection != "app.bsky.feed.post" {
		t.Errorf("expected collection %q, got %q", "app.bsky.feed.post", event.Record.Collection)
	}
	if event.Record.RKey != "abc" {
		t.Errorf("expected rkey %q, got %q", "abc", event.Record.RKey)
	}
	if event.Record.Action != ActionCreate {
		t.Errorf("expected action %q, got %q", ActionCreate, event.Record.Action)
	}
	if event.Record.CID != "bafyrei123" {
		t.Errorf("expected cid %q, got %q", "bafyrei123", event.Record.CID)
	}
	if string(event.Record.Record) != `{"text": "hello"}` {
		t.Errorf("unexpected record body: %s", event.Record.Record)
	}
}

func TestParseEvent_RecordDelete(t *testing.T) {
	data := []byte(`{
		"id": 12346,
		"type": "record",
		"record": {
			"live": false,
			"rev": "3kgn2v4",
			"did": "did:plc:abc",
			"collection": "app.bsky.feed.post",
			"rkey": "abc",
			"action": "delete"
		}
	}`)

	event, err := ParseEvent(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != EventTypeRecord {
		t.Errorf("expected type %q, got %q", EventTypeRecord, event.Type)
	}
	if event.Record == nil {
		t.Fatal("expected record to be non-nil")
	}
	if event.Record.EventID != event.ID {
		t.Errorf("expected record EventID %d, got %d", event.ID, event.Record.EventID)
	}
	if event.Record.Action != ActionDelete {
		t.Errorf("expected action %q, got %q", ActionDelete, event.Record.Action)
	}
	if event.Record.CID != "" {
		t.Errorf("expected empty cid for delete, got %q", event.Record.CID)
	}
	if event.Record.Record != nil {
		t.Errorf("expected nil record body for delete, got %s", event.Record.Record)
	}
}

func TestParseEvent_Identity(t *testing.T) {
	data := []byte(`{
		"id": 12347,
		"type": "identity",
		"identity": {
			"did": "did:plc:abc",
			"handle": "alice.bsky.social",
			"is_active": true,
			"status": "active"
		}
	}`)

	event, err := ParseEvent(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ID != 12347 {
		t.Errorf("expected ID 12347, got %d", event.ID)
	}
	if event.Type != EventTypeIdentity {
		t.Errorf("expected type %q, got %q", EventTypeIdentity, event.Type)
	}
	if event.Identity == nil {
		t.Fatal("expected identity to be non-nil")
	}
	if event.Identity.EventID != event.ID {
		t.Errorf("expected identity EventID %d, got %d", event.ID, event.Identity.EventID)
	}
	if event.Identity.DID != "did:plc:abc" {
		t.Errorf("expected DID %q, got %q", "did:plc:abc", event.Identity.DID)
	}
	if event.Identity.Handle != "alice.bsky.social" {
		t.Errorf("expected handle %q, got %q", "alice.bsky.social", event.Identity.Handle)
	}
	if !event.Identity.IsActive {
		t.Error("expected is_active to be true")
	}
	if !event.Identity.IsActivePresent {
		t.Error("expected is_active presence to be tracked")
	}
	if event.Identity.Status != "active" {
		t.Errorf("expected status %q, got %q", "active", event.Identity.Status)
	}
}

func TestParseEvent_IdentityIgnoresCamelCaseIsActive(t *testing.T) {
	data := []byte(`{
		"id": 12348,
		"type": "identity",
		"identity": {
			"did": "did:plc:abc",
			"handle": "alice.bsky.social",
			"isActive": true,
			"status": "active"
		}
	}`)

	event, err := ParseEvent(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Identity == nil {
		t.Fatal("expected identity to be non-nil")
	}
	if event.Identity.IsActive {
		t.Error("expected camelCase isActive to be ignored")
	}
	if event.Identity.IsActivePresent {
		t.Error("expected camelCase isActive not to mark is_active present")
	}
	if shouldPurgeIdentity(event.Identity) {
		t.Error("expected active status with camelCase isActive to keep identity")
	}
}

func TestParseEvent_MissingType(t *testing.T) {
	data := []byte(`{"id": 1, "record": {}}`)

	_, err := ParseEvent(data)
	if err == nil {
		t.Fatal("expected error for missing type field, got nil")
	}
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid json`)

	_, err := ParseEvent(data)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRecordEvent_URI(t *testing.T) {
	tests := []struct {
		name       string
		did        string
		collection string
		rkey       string
		wantURI    string
	}{
		{
			name:       "standard AT-URI",
			did:        "did:plc:abc",
			collection: "app.bsky.feed.post",
			rkey:       "abc",
			wantURI:    "at://did:plc:abc/app.bsky.feed.post/abc",
		},
		{
			name:       "different collection",
			did:        "did:plc:xyz",
			collection: "app.bsky.actor.profile",
			rkey:       "self",
			wantURI:    "at://did:plc:xyz/app.bsky.actor.profile/self",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RecordEvent{
				DID:        tt.did,
				Collection: tt.collection,
				RKey:       tt.rkey,
			}
			got := r.URI()
			if got != tt.wantURI {
				t.Errorf("URI() = %q, want %q", got, tt.wantURI)
			}
		})
	}
}

func TestEvent_IsRecord(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		{
			name: "record event with record",
			event: Event{
				Type:   EventTypeRecord,
				Record: &RecordEvent{},
			},
			want: true,
		},
		{
			name: "record type but nil record",
			event: Event{
				Type:   EventTypeRecord,
				Record: nil,
			},
			want: false,
		},
		{
			name: "identity event",
			event: Event{
				Type:     EventTypeIdentity,
				Identity: &IdentityEvent{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.IsRecord(); got != tt.want {
				t.Errorf("IsRecord() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvent_IsIdentity(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		{
			name: "identity event with identity",
			event: Event{
				Type:     EventTypeIdentity,
				Identity: &IdentityEvent{},
			},
			want: true,
		},
		{
			name: "identity type but nil identity",
			event: Event{
				Type:     EventTypeIdentity,
				Identity: nil,
			},
			want: false,
		},
		{
			name: "record event",
			event: Event{
				Type:   EventTypeRecord,
				Record: &RecordEvent{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.IsIdentity(); got != tt.want {
				t.Errorf("IsIdentity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseEvent_AcceptanceCriteria_RecordCreate(t *testing.T) {
	// Exact format from acceptance criteria
	data := []byte(`{"id": 12345, "type": "record", "record": {"live": true, "rev": "abc123", "did": "did:plc:abc", "collection": "app.bsky.feed.post", "rkey": "abc", "action": "create", "cid": "bafyrei...", "record": {"text": "hello"}}}`)

	event, err := ParseEvent(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ID != 12345 {
		t.Errorf("expected ID 12345, got %d", event.ID)
	}
	if event.Type != EventTypeRecord {
		t.Errorf("expected type %q, got %q", EventTypeRecord, event.Type)
	}
	if event.Record == nil {
		t.Fatal("expected record to be non-nil")
	}
	if event.Record.DID != "did:plc:abc" {
		t.Errorf("expected DID %q, got %q", "did:plc:abc", event.Record.DID)
	}
	if event.Record.Collection != "app.bsky.feed.post" {
		t.Errorf("expected collection %q, got %q", "app.bsky.feed.post", event.Record.Collection)
	}
	if event.Record.Action != ActionCreate {
		t.Errorf("expected action %q, got %q", ActionCreate, event.Record.Action)
	}
	// Verify record body is valid JSON
	var body map[string]interface{}
	if err := json.Unmarshal(event.Record.Record, &body); err != nil {
		t.Errorf("record body is not valid JSON: %v", err)
	}
}

func TestParseEvent_AcceptanceCriteria_Identity(t *testing.T) {
	// Exact format from acceptance criteria
	data := []byte(`{"id": 12346, "type": "identity", "identity": {"did": "did:plc:abc", "handle": "alice.bsky.social", "is_active": true, "status": "active"}}`)

	event, err := ParseEvent(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ID != 12346 {
		t.Errorf("expected ID 12346, got %d", event.ID)
	}
	if event.Type != EventTypeIdentity {
		t.Errorf("expected type %q, got %q", EventTypeIdentity, event.Type)
	}
	if event.Identity == nil {
		t.Fatal("expected identity to be non-nil")
	}
	if event.Identity.DID != "did:plc:abc" {
		t.Errorf("expected DID %q, got %q", "did:plc:abc", event.Identity.DID)
	}
	if event.Identity.Handle != "alice.bsky.social" {
		t.Errorf("expected handle %q, got %q", "alice.bsky.social", event.Identity.Handle)
	}
	if !event.Identity.IsActive {
		t.Error("expected is_active to be true")
	}
	if !event.Identity.IsActivePresent {
		t.Error("expected is_active presence to be tracked")
	}
	if event.Identity.Status != "active" {
		t.Errorf("expected status %q, got %q", "active", event.Identity.Status)
	}
}

func TestRecordEvent_URI_AcceptanceCriteria(t *testing.T) {
	r := &RecordEvent{
		DID:        "did:plc:abc",
		Collection: "app.bsky.feed.post",
		RKey:       "abc",
	}
	got := r.URI()
	want := "at://did:plc:abc/app.bsky.feed.post/abc"
	if got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestParseEvent_RecordValidation(t *testing.T) {
	validRecord := `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","rkey":"abc","action":"create","record":{"text":"hello"}}}`

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid record event",
			data:    validRecord,
			wantErr: false,
		},
		{
			name:    "record with empty DID",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"","collection":"app.bsky.feed.post","rkey":"abc","action":"create","record":{"text":"hello"}}}`,
			wantErr: true,
		},
		{
			name:    "record with missing DID",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","collection":"app.bsky.feed.post","rkey":"abc","action":"create","record":{"text":"hello"}}}`,
			wantErr: true,
		},
		{
			name:    "record with empty Collection",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"","rkey":"abc","action":"create","record":{"text":"hello"}}}`,
			wantErr: true,
		},
		{
			name:    "record with missing Collection",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","rkey":"abc","action":"create","record":{"text":"hello"}}}`,
			wantErr: true,
		},
		{
			name:    "record with empty RKey",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","rkey":"","action":"create","record":{"text":"hello"}}}`,
			wantErr: true,
		},
		{
			name:    "record with missing RKey",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","action":"create","record":{"text":"hello"}}}`,
			wantErr: true,
		},
		{
			name:    "record with empty Action",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","rkey":"abc","action":"","record":{"text":"hello"}}}`,
			wantErr: true,
		},
		{
			name:    "record with missing Action",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","rkey":"abc","record":{"text":"hello"}}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEvent([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseEvent_IdentityValidation(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid identity event",
			data:    `{"id":1,"type":"identity","identity":{"did":"did:plc:abc","handle":"alice.bsky.social","is_active":true,"status":"active"}}`,
			wantErr: false,
		},
		{
			name:    "identity with empty DID",
			data:    `{"id":1,"type":"identity","identity":{"did":"","handle":"alice.bsky.social","is_active":true,"status":"active"}}`,
			wantErr: true,
		},
		{
			name:    "identity with missing DID",
			data:    `{"id":1,"type":"identity","identity":{"handle":"alice.bsky.social","is_active":true,"status":"active"}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEvent([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseEvent_NilPayloadRejection(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "type=record with nil Record payload",
			data:    `{"id":1,"type":"record"}`,
			wantErr: true,
		},
		{
			name:    "type=identity with nil Identity payload",
			data:    `{"id":1,"type":"identity"}`,
			wantErr: true,
		},
		{
			name:    "type=record action=create with empty record body",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","rkey":"abc","action":"create"}}`,
			wantErr: false, // valid Tap protocol event — handler skips gracefully and acks
		},
		{
			name:    "type=record action=update with empty record body",
			data:    `{"id":1,"type":"record","record":{"live":true,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","rkey":"abc","action":"update"}}`,
			wantErr: false, // valid Tap protocol event — handler skips gracefully and acks
		},
		{
			name:    "type=record action=delete with empty record body",
			data:    `{"id":1,"type":"record","record":{"live":false,"rev":"r1","did":"did:plc:abc","collection":"app.bsky.feed.post","rkey":"abc","action":"delete"}}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEvent([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
