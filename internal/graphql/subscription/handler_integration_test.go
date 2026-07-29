package subscription_test

import (
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/schema"
	"github.com/GainForest/hyperindex/internal/graphql/subscription"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
)

func TestHandlerTypedSubscriptionSkipsInvalidEvent(t *testing.T) {
	registry := lexicon.NewRegistry()
	registry.Register(&lexicon.Lexicon{
		ID: "com.example.subscription.record",
		Defs: lexicon.Defs{Main: &lexicon.RecordDef{
			Type: "record",
			Key:  "tid",
			Properties: []lexicon.PropertyEntry{
				{Name: "text", Property: lexicon.Property{Type: lexicon.TypeString}},
			},
		}},
	})
	gqlSchema, err := schema.NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	repos := &resolver.Repositories{Records: db.Records}
	handler := subscription.NewHandler(gqlSchema, pubsub, repos, []string{"*"})
	server := httptest.NewServer(handler)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	writeWSJSON(t, conn, map[string]interface{}{"type": "connection_init"})
	var ack map[string]interface{}
	readWSJSON(t, conn, &ack)
	if ack["type"] != "connection_ack" {
		t.Fatalf("ack type = %v, want connection_ack", ack["type"])
	}
	writeWSJSON(t, conn, map[string]interface{}{
		"id":   "typed",
		"type": "subscribe",
		"payload": map[string]interface{}{
			"query": `subscription { comExampleSubscriptionRecordEvents { uri text } }`,
		},
	})
	waitForSubscribers(t, pubsub, 1)

	uri := "at://did:plc:test/com.example.subscription.record/one"
	if _, err := db.Records.Insert(t.Context(), uri, "cid-invalid", "did:plc:test", "com.example.subscription.record", `{"text":"invalid"}`); err != nil {
		t.Fatalf("Insert(invalid) error = %v", err)
	}
	if err := db.Records.UpdateValidationStatus(t.Context(), uri, validation.StatusInvalid, "bad", "hash-current"); err != nil {
		t.Fatalf("mark invalid: %v", err)
	}
	pubsub.PublishRecordWithValidation(subscription.EventCreate, uri, "cid-invalid", "did:plc:test", "com.example.subscription.record", []byte(`{"text":"invalid"}`), false)

	if _, err := db.Records.Insert(t.Context(), uri, "cid-valid", "did:plc:test", "com.example.subscription.record", `{"text":"valid"}`); err != nil {
		t.Fatalf("Insert(valid) error = %v", err)
	}
	if err := db.Records.UpdateValidationStatus(t.Context(), uri, validation.StatusValid, "", "hash-current"); err != nil {
		t.Fatalf("mark valid: %v", err)
	}
	pubsub.PublishRecordWithValidation(subscription.EventUpdate, uri, "cid-valid", "did:plc:test", "com.example.subscription.record", []byte(`{"text":"valid"}`), true)

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var next struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Payload struct {
			Data map[string]map[string]interface{} `json:"data"`
		} `json:"payload"`
	}
	readWSJSON(t, conn, &next)
	if next.Type != "next" || next.ID != "typed" {
		t.Fatalf("message = id:%q type:%q, want typed/next", next.ID, next.Type)
	}
	payload := next.Payload.Data["comExampleSubscriptionRecordEvents"]
	if payload["text"] != "valid" {
		t.Fatalf("first typed payload text = %v, want valid; invalid event should have been suppressed", payload["text"])
	}
}

func TestHandlerRecordEventsEnforcesResolvedCollectionArgument(t *testing.T) {
	server, pubsub, _ := newSubscriptionTestServer(t)

	inline := openSubscriptionConnection(t, server)
	defer inline.Close()
	writeWSJSON(t, inline, map[string]interface{}{
		"id": "inline", "type": "subscribe",
		"payload": map[string]interface{}{
			"query": `subscription { recordEvents(collection: "com.example.subscription.record") { uri collection } }`,
		},
	})

	variable := openSubscriptionConnection(t, server)
	defer variable.Close()
	writeWSJSON(t, variable, map[string]interface{}{
		"id": "variable", "type": "subscribe",
		"payload": map[string]interface{}{
			"query":     `subscription Filtered($wanted: String!) { recordEvents(collection: $wanted) { uri collection } }`,
			"variables": map[string]interface{}{"wanted": "com.example.subscription.record"},
		},
	})
	waitForSubscribers(t, pubsub, 2)

	pubsub.PublishRecordWithValidation(subscription.EventCreate,
		"at://did:plc:test/com.example.other/one", "cid-other", "did:plc:test", "com.example.other", []byte(`{"text":"other"}`), true)
	pubsub.PublishRecordWithValidation(subscription.EventCreate,
		"at://did:plc:test/com.example.subscription.record/one", "cid-match", "did:plc:test", "com.example.subscription.record", []byte(`{"text":"matching"}`), true)

	for id, conn := range map[string]*websocket.Conn{"inline": inline, "variable": variable} {
		payload := readSubscriptionPayload(t, conn, id, "recordEvents")
		if payload["collection"] != "com.example.subscription.record" {
			t.Fatalf("%s first delivered collection = %v, want matching collection; nonmatching event was not filtered", id, payload["collection"])
		}
	}
}

func TestHandlerConcurrentRawAndTypedSubscribersDoNotShareMutableRecord(t *testing.T) {
	server, pubsub, db := newSubscriptionTestServer(t)

	rawConn := openSubscriptionConnection(t, server)
	defer rawConn.Close()
	writeWSJSON(t, rawConn, map[string]interface{}{
		"id": "raw", "type": "subscribe",
		"payload": map[string]interface{}{
			"query": `subscription { recordEvents { uri record } }`,
		},
	})

	const typedSubscribers = 8
	typedConnections := make([]*websocket.Conn, 0, typedSubscribers)
	for i := 0; i < typedSubscribers; i++ {
		conn := openSubscriptionConnection(t, server)
		typedConnections = append(typedConnections, conn)
		defer conn.Close()
		writeWSJSON(t, conn, map[string]interface{}{
			"id": "typed", "type": "subscribe",
			"payload": map[string]interface{}{
				"query": `subscription { comExampleSubscriptionRecordEvents { uri text } }`,
			},
		})
	}
	waitForSubscribers(t, pubsub, typedSubscribers+1)

	uri := "at://did:plc:test/com.example.subscription.record/shared"
	if _, err := db.Records.Insert(t.Context(), uri, "cid-shared", "did:plc:test", "com.example.subscription.record", `{"$type":"com.example.subscription.record"}`); err != nil {
		t.Fatalf("Insert(shared event record) error = %v", err)
	}
	if err := db.Records.UpdateValidationStatus(t.Context(), uri, validation.StatusValid, "", "hash-current"); err != nil {
		t.Fatalf("mark shared event record valid: %v", err)
	}
	pubsub.PublishRecordWithValidation(subscription.EventCreate, uri, "cid-shared", "did:plc:test", "com.example.subscription.record", []byte(`{"$type":"com.example.subscription.record"}`), true)

	rawPayload := readSubscriptionPayload(t, rawConn, "raw", "recordEvents")
	rawRecord, ok := rawPayload["record"].(map[string]interface{})
	if !ok {
		t.Fatalf("raw record payload = %#v, want object", rawPayload["record"])
	}
	if _, exists := rawRecord["text"]; exists {
		t.Fatalf("typed coercion mutated raw record payload: %#v", rawRecord)
	}
	for _, conn := range typedConnections {
		payload := readSubscriptionPayload(t, conn, "typed", "comExampleSubscriptionRecordEvents")
		if payload["text"] != "" {
			t.Fatalf("typed coerced text = %#v, want empty string", payload["text"])
		}
	}
}

func TestHandlerRawSubscriptionPreservesNonObjectJSONAndTypedSuppressesIt(t *testing.T) {
	server, pubsub, _ := newSubscriptionTestServer(t)
	rawConn := openSubscriptionConnection(t, server)
	defer rawConn.Close()
	typedConn := openSubscriptionConnection(t, server)
	defer typedConn.Close()
	writeWSJSON(t, rawConn, map[string]interface{}{
		"id": "raw-types", "type": "subscribe",
		"payload": map[string]interface{}{"query": `subscription { recordEvents { uri record } }`},
	})
	writeWSJSON(t, typedConn, map[string]interface{}{
		"id": "typed-types", "type": "subscribe",
		"payload": map[string]interface{}{"query": `subscription { comExampleSubscriptionRecordEvents { uri text } }`},
	})
	waitForSubscribers(t, pubsub, 2)

	tests := []struct {
		name string
		raw  string
		want interface{}
	}{
		{name: "array", raw: `["array"]`, want: []interface{}{"array"}},
		{name: "string", raw: `"string"`, want: "string"},
		{name: "number", raw: `42`, want: float64(42)},
		{name: "boolean", raw: `true`, want: true},
		{name: "null", raw: `null`, want: nil},
	}
	for _, test := range tests {
		uri := "at://did:plc:test/com.example.subscription.record/" + test.name
		pubsub.PublishRecordWithValidation(subscription.EventCreate, uri, "cid-"+test.name, "did:plc:test", "com.example.subscription.record", []byte(test.raw), false)
		payload := readSubscriptionPayload(t, rawConn, "raw-types", "recordEvents")
		if !reflect.DeepEqual(payload["record"], test.want) {
			t.Fatalf("%s raw record = %#v, want %#v", test.name, payload["record"], test.want)
		}
	}
	assertNoWSMessage(t, typedConn)
}

func TestHandlerTypedSubscriptionPopulatesMetadataForCreateUpdateDelete(t *testing.T) {
	server, pubsub, db := newSubscriptionTestServer(t)
	conn := openSubscriptionConnection(t, server)
	defer conn.Close()
	writeWSJSON(t, conn, map[string]interface{}{
		"id": "metadata", "type": "subscribe",
		"payload": map[string]interface{}{
			"query": `subscription { comExampleSubscriptionRecordEvents { uri cid did rkey text } }`,
		},
	})
	waitForSubscribers(t, pubsub, 1)

	uri := "at://did:plc:metadata/com.example.subscription.record/metadata-key"
	for _, event := range []struct {
		typeName subscription.EventType
		cid      string
		text     string
	}{
		{typeName: subscription.EventCreate, cid: "cid-create", text: "created"},
		{typeName: subscription.EventUpdate, cid: "cid-update", text: "updated"},
	} {
		if _, err := db.Records.Insert(t.Context(), uri, event.cid, "did:plc:metadata", "com.example.subscription.record", `{"text":"`+event.text+`"}`); err != nil {
			t.Fatalf("Insert(%s) error = %v", event.typeName, err)
		}
		if err := db.Records.UpdateValidationStatus(t.Context(), uri, validation.StatusValid, "", "hash-current"); err != nil {
			t.Fatalf("mark %s valid: %v", event.typeName, err)
		}
		pubsub.PublishRecordWithValidation(event.typeName, uri, event.cid, "did:plc:metadata", "com.example.subscription.record", []byte(`{"text":"`+event.text+`"}`), true)
		payload := readSubscriptionPayload(t, conn, "metadata", "comExampleSubscriptionRecordEvents")
		assertTypedSubscriptionMetadata(t, payload, uri, event.cid, "did:plc:metadata", "metadata-key")
		if payload["text"] != event.text {
			t.Fatalf("%s text = %v, want %s", event.typeName, payload["text"], event.text)
		}
	}

	deleted, err := db.Records.DeleteReturning(t.Context(), uri)
	if err != nil || deleted == nil {
		t.Fatalf("DeleteReturning() record=%v error=%v", deleted, err)
	}
	pubsub.PublishDelete(uri, deleted.CID, deleted.DID, deleted.Collection, []byte(deleted.JSON), true)
	payload := readSubscriptionPayload(t, conn, "metadata", "comExampleSubscriptionRecordEvents")
	assertTypedSubscriptionMetadata(t, payload, uri, "cid-update", "did:plc:metadata", "metadata-key")
	if payload["text"] != "updated" {
		t.Fatalf("delete text = %v, want updated", payload["text"])
	}
}

func TestHandlerRejectsInvalidOperationsAndUsesProtocolCloseCodes(t *testing.T) {
	t.Run("invalid query is terminal before registration", func(t *testing.T) {
		server, pubsub, _ := newSubscriptionTestServer(t)
		conn := openSubscriptionConnection(t, server)
		defer conn.Close()
		writeWSJSON(t, conn, map[string]interface{}{
			"id": "invalid", "type": "subscribe",
			"payload": map[string]interface{}{"query": `subscription { missingEvents { uri } }`},
		})
		assertWSError(t, conn, "invalid")
		if pubsub.SubscriberCount() != 0 {
			t.Fatalf("SubscriberCount() = %d, want 0 after invalid operation", pubsub.SubscriberCount())
		}
		assertNoWSMessage(t, conn)
	})

	t.Run("duplicate active id closes 4409 and cleans up", func(t *testing.T) {
		server, pubsub, _ := newSubscriptionTestServer(t)
		conn := openSubscriptionConnection(t, server)
		defer conn.Close()
		message := map[string]interface{}{
			"id": "duplicate", "type": "subscribe",
			"payload": map[string]interface{}{"query": `subscription { recordEvents { uri } }`},
		}
		writeWSJSON(t, conn, message)
		waitForSubscribers(t, pubsub, 1)
		writeWSJSON(t, conn, map[string]interface{}{
			"id": "duplicate", "type": "subscribe",
			"payload": map[string]interface{}{"query": `{ malformed`},
		})
		assertWSCloseCode(t, conn, 4409, "already exists")
		waitForSubscribers(t, pubsub, 0)
	})

	t.Run("oversized duplicate id closes 4409 and cleans up", func(t *testing.T) {
		server, pubsub, _ := newSubscriptionTestServer(t)
		conn := openSubscriptionConnection(t, server)
		defer conn.Close()
		oversizedID := strings.Repeat("subscriber-", 20)
		message := map[string]interface{}{
			"id": oversizedID, "type": "subscribe",
			"payload": map[string]interface{}{"query": `subscription { recordEvents { uri } }`},
		}
		writeWSJSON(t, conn, message)
		waitForSubscribers(t, pubsub, 1)
		writeWSJSON(t, conn, message)
		assertWSCloseCode(t, conn, 4409, "Subscriber already exists")
		waitForSubscribers(t, pubsub, 0)
	})

	t.Run("subscribe before initialization closes 4401", func(t *testing.T) {
		server, pubsub, _ := newSubscriptionTestServer(t)
		conn := dialSubscriptionConnection(t, server)
		defer conn.Close()
		writeWSJSON(t, conn, map[string]interface{}{
			"id": "unauthorized", "type": "subscribe",
			"payload": map[string]interface{}{"query": `subscription { recordEvents { uri } }`},
		})
		assertWSCloseCode(t, conn, 4401, "Unauthorized")
		if pubsub.SubscriberCount() != 0 {
			t.Fatalf("SubscriberCount() = %d, want 0", pubsub.SubscriberCount())
		}
	})

	t.Run("repeated connection init closes 4429", func(t *testing.T) {
		server, pubsub, _ := newSubscriptionTestServer(t)
		conn := openSubscriptionConnection(t, server)
		defer conn.Close()
		writeWSJSON(t, conn, map[string]interface{}{"type": "connection_init"})
		assertWSCloseCode(t, conn, 4429, "Too many initialization requests")
		if pubsub.SubscriberCount() != 0 {
			t.Fatalf("SubscriberCount() = %d, want 0", pubsub.SubscriberCount())
		}
	})
}

func TestHandlerGraphQLErrorIsTerminalWithoutCompleteOrLaterMessages(t *testing.T) {
	server, pubsub, db := newSubscriptionTestServer(t)
	conn := openSubscriptionConnection(t, server)
	defer conn.Close()
	writeWSJSON(t, conn, map[string]interface{}{
		"id": "runtime-error", "type": "subscribe",
		"payload": map[string]interface{}{"query": `subscription { comExampleSubscriptionRecordEvents { uri text } }`},
	})
	waitForSubscribers(t, pubsub, 1)
	if err := db.Executor.Close(); err != nil {
		t.Fatalf("Close database: %v", err)
	}
	pubsub.PublishRecordWithValidation(subscription.EventCreate,
		"at://did:plc:test/com.example.subscription.record/error", "cid-error", "did:plc:test", "com.example.subscription.record", []byte(`{"text":"error"}`), true)
	assertWSError(t, conn, "runtime-error")
	waitForSubscribers(t, pubsub, 0)
	pubsub.PublishRecordWithValidation(subscription.EventCreate,
		"at://did:plc:test/com.example.subscription.record/later", "cid-later", "did:plc:test", "com.example.subscription.record", []byte(`{"text":"later"}`), true)
	assertNoWSMessage(t, conn)
}

func assertTypedSubscriptionMetadata(t *testing.T, payload map[string]interface{}, uri, cid, did, rkey string) {
	t.Helper()
	for field, want := range map[string]string{"uri": uri, "cid": cid, "did": did, "rkey": rkey} {
		if payload[field] != want {
			t.Fatalf("typed payload %s = %v, want %s; payload=%#v", field, payload[field], want, payload)
		}
	}
}

func assertWSError(t *testing.T, conn *websocket.Conn, wantID string) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var message map[string]interface{}
	readWSJSON(t, conn, &message)
	if message["type"] != "error" || message["id"] != wantID {
		t.Fatalf("message = %#v, want terminal error for %s", message, wantID)
	}
}

func assertWSCloseCode(t *testing.T, conn *websocket.Conn, wantCode int, wantReason string) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, _, err := conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("ReadMessage() error = %T %v, want WebSocket close %d", err, err, wantCode)
	}
	if closeErr.Code != wantCode || !strings.Contains(closeErr.Text, wantReason) {
		t.Fatalf("close = code:%d reason:%q, want %d containing %q", closeErr.Code, closeErr.Text, wantCode, wantReason)
	}
}

func assertNoWSMessage(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, raw, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("unexpected post-terminal WebSocket message: %s", raw)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("post-terminal read error = %v, want timeout with no message", err)
	}
}

func newSubscriptionTestServer(t *testing.T) (*httptest.Server, *subscription.PubSub, *testutil.TestDB) {
	t.Helper()
	registry := lexicon.NewRegistry()
	registry.Register(&lexicon.Lexicon{
		ID: "com.example.subscription.record",
		Defs: lexicon.Defs{Main: &lexicon.RecordDef{
			Type: "record",
			Key:  "tid",
			Properties: []lexicon.PropertyEntry{
				{Name: "text", Property: lexicon.Property{Type: lexicon.TypeString, Required: true}},
			},
		}},
	})
	gqlSchema, err := schema.NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	handler := subscription.NewHandler(gqlSchema, pubsub, &resolver.Repositories{Records: db.Records}, []string{"*"})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, pubsub, db
}

func dialSubscriptionConnection(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	return conn
}

func openSubscriptionConnection(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	conn := dialSubscriptionConnection(t, server)
	writeWSJSON(t, conn, map[string]interface{}{"type": "connection_init"})
	var ack map[string]interface{}
	readWSJSON(t, conn, &ack)
	if ack["type"] != "connection_ack" {
		t.Fatalf("ack type = %v, want connection_ack", ack["type"])
	}
	return conn
}

func readSubscriptionPayload(t *testing.T, conn *websocket.Conn, wantID, field string) map[string]interface{} {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var next struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Payload struct {
			Data map[string]interface{} `json:"data"`
		} `json:"payload"`
	}
	readWSJSON(t, conn, &next)
	if next.Type != "next" || next.ID != wantID {
		t.Fatalf("message = id:%q type:%q, want %s/next", next.ID, next.Type, wantID)
	}
	payload, ok := next.Payload.Data[field].(map[string]interface{})
	if !ok {
		t.Fatalf("subscription field %s payload = %#v, want object", field, next.Payload.Data[field])
	}
	return payload
}

func writeWSJSON(t *testing.T, conn *websocket.Conn, value interface{}) {
	t.Helper()
	if err := conn.WriteJSON(value); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
}

func readWSJSON(t *testing.T, conn *websocket.Conn, target interface{}) {
	t.Helper()
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode WebSocket message %s: %v", raw, err)
	}
}

func waitForSubscribers(t *testing.T, pubsub *subscription.PubSub, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if pubsub.SubscriberCount() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("SubscriberCount() = %d, want %d", pubsub.SubscriberCount(), want)
}
