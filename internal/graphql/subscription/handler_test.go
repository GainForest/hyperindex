package subscription

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graphql-go/graphql"
)

func TestMakeOriginChecker(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		want           bool
	}{
		{
			name:           "nil origins allows all",
			allowedOrigins: nil,
			requestOrigin:  "https://example.com",
			want:           true,
		},
		{
			name:           "empty origins allows all",
			allowedOrigins: []string{},
			requestOrigin:  "https://example.com",
			want:           true,
		},
		{
			name:           "wildcard allows all",
			allowedOrigins: []string{"*"},
			requestOrigin:  "https://example.com",
			want:           true,
		},
		{
			name:           "no origin header always allowed",
			allowedOrigins: []string{"https://allowed.com"},
			requestOrigin:  "",
			want:           true,
		},
		{
			name:           "matching origin allowed",
			allowedOrigins: []string{"https://allowed.com"},
			requestOrigin:  "https://allowed.com",
			want:           true,
		},
		{
			name:           "non-matching origin rejected",
			allowedOrigins: []string{"https://allowed.com"},
			requestOrigin:  "https://evil.com",
			want:           false,
		},
		{
			name:           "multiple origins one matches",
			allowedOrigins: []string{"https://a.com", "https://b.com"},
			requestOrigin:  "https://b.com",
			want:           true,
		},
		{
			name:           "multiple origins none match",
			allowedOrigins: []string{"https://a.com", "https://b.com"},
			requestOrigin:  "https://c.com",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := makeOriginChecker(tt.allowedOrigins)
			req, _ := http.NewRequest("GET", "/graphql/ws", nil)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
			got := checker(req)
			if got != tt.want {
				t.Errorf("makeOriginChecker(%v) with origin %q = %v, want %v",
					tt.allowedOrigins, tt.requestOrigin, got, tt.want)
			}
		})
	}
}

func TestHandlerSubscribeUsesLatestSchemaProviderOnExistingConnection(t *testing.T) {
	pubsub := NewPubSub()
	provider := &mutableSubscriptionSchemaProvider{schema: testSubscriptionSchema(t, "oldMarker")}
	server := httptest.NewServer(NewHandlerWithSchemaProvider(provider, pubsub, []string{"*"}))
	defer server.Close()

	conn := dialSubscriptionWebSocket(t, server.URL)
	defer conn.Close()
	initializeSubscriptionConnection(t, conn)

	sendSubscribe(t, conn, "old", `subscription { recordEvents { collection oldMarker } }`)
	waitForSubscriberCount(t, pubsub, 1)
	pubsub.Publish(&RecordEvent{Type: EventCreate, Collection: "old.collection"})
	assertNextMessage(t, conn, "old")

	provider.set(testSubscriptionSchema(t, "newMarker"))
	sendSubscribe(t, conn, "new", `subscription { recordEvents { collection newMarker } }`)
	waitForSubscriberCount(t, pubsub, 2)
	pubsub.Publish(&RecordEvent{Type: EventCreate, Collection: "new.collection"})

	seenOld := false
	seenNew := false
	for !seenOld || !seenNew {
		msg := readWSMessage(t, conn)
		switch msg.Type {
		case msgNext:
			switch msg.ID {
			case "old":
				seenOld = true
			case "new":
				seenNew = true
			}
		case msgError:
			t.Fatalf("unexpected subscription error for id %q: %s", msg.ID, string(msg.Payload))
		}
	}
}

func TestHandlerSubscribeWithoutActiveSchemaReturnsError(t *testing.T) {
	pubsub := NewPubSub()
	provider := &mutableSubscriptionSchemaProvider{}
	server := httptest.NewServer(NewHandlerWithSchemaProvider(provider, pubsub, []string{"*"}))
	defer server.Close()

	conn := dialSubscriptionWebSocket(t, server.URL)
	defer conn.Close()
	initializeSubscriptionConnection(t, conn)

	sendSubscribe(t, conn, "missing-schema", `subscription { recordEvents { collection } }`)
	msg := readWSMessage(t, conn)
	if msg.Type != msgError {
		t.Fatalf("message type = %q, want %q", msg.Type, msgError)
	}
	if msg.ID != "missing-schema" {
		t.Fatalf("message id = %q, want missing-schema", msg.ID)
	}
	if !strings.Contains(string(msg.Payload), "public GraphQL schema is unavailable") || !strings.Contains(string(msg.Payload), "reloadSchema") {
		t.Fatalf("expected actionable schema-unavailable error, got %s", string(msg.Payload))
	}
	var errorsPayload []map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &errorsPayload); err != nil {
		t.Fatalf("failed to decode schema-unavailable error payload: %v", err)
	}
	if len(errorsPayload) != 1 {
		t.Fatalf("expected one schema-unavailable error, got %#v", errorsPayload)
	}
	extensions, ok := errorsPayload[0]["extensions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema-unavailable error extensions, got %#v", errorsPayload[0]["extensions"])
	}
	if extensions["code"] != "SCHEMA_UNAVAILABLE" || extensions["httpStatus"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("unexpected schema-unavailable extensions: %#v", extensions)
	}
	if got := pubsub.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count = %d, want 0", got)
	}
}

type mutableSubscriptionSchemaProvider struct {
	mu     sync.RWMutex
	schema *graphql.Schema
}

func (p *mutableSubscriptionSchemaProvider) Schema() *graphql.Schema {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.schema
}

func (p *mutableSubscriptionSchemaProvider) set(schema *graphql.Schema) {
	p.mu.Lock()
	p.schema = schema
	p.mu.Unlock()
}

func testSubscriptionSchema(t *testing.T, markerField string) *graphql.Schema {
	t.Helper()

	recordEventType := graphql.NewObject(graphql.ObjectConfig{
		Name: "RecordEvent" + strings.ToUpper(markerField[:1]) + markerField[1:],
		Fields: graphql.Fields{
			"collection": &graphql.Field{Type: graphql.String},
			markerField: &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return markerField, nil
				},
			},
		},
	})
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Query" + strings.ToUpper(markerField[:1]) + markerField[1:],
		Fields: graphql.Fields{"ping": &graphql.Field{Type: graphql.String}},
	})
	subscriptionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Subscription" + strings.ToUpper(markerField[:1]) + markerField[1:],
		Fields: graphql.Fields{
			"recordEvents": &graphql.Field{Type: recordEventType},
		},
	})

	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType, Subscription: subscriptionType})
	if err != nil {
		t.Fatalf("failed to create subscription schema: %v", err)
	}
	return &schema
}

func dialSubscriptionWebSocket(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http")
	dialer := websocket.Dialer{Subprotocols: []string{graphqlWSProtocol}}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial subscription WebSocket: %v", err)
	}
	return conn
}

func initializeSubscriptionConnection(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	writeWSMessage(t, conn, wsMessage{Type: msgConnectionInit})
	msg := readWSMessage(t, conn)
	if msg.Type != msgConnectionAck {
		t.Fatalf("message type = %q, want %q", msg.Type, msgConnectionAck)
	}
}

func sendSubscribe(t *testing.T, conn *websocket.Conn, id, query string) {
	t.Helper()

	payload, err := json.Marshal(subscribePayload{Query: query})
	if err != nil {
		t.Fatalf("failed to marshal subscribe payload: %v", err)
	}
	writeWSMessage(t, conn, wsMessage{ID: id, Type: msgSubscribe, Payload: payload})
}

func assertNextMessage(t *testing.T, conn *websocket.Conn, id string) {
	t.Helper()

	msg := readWSMessage(t, conn)
	if msg.Type != msgNext || msg.ID != id {
		t.Fatalf("message = {id:%q type:%q payload:%s}, want next for id %q", msg.ID, msg.Type, string(msg.Payload), id)
	}
}

func writeWSMessage(t *testing.T, conn *websocket.Conn, msg wsMessage) {
	t.Helper()

	if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("failed to set write deadline: %v", err)
	}
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("failed to write WebSocket message: %v", err)
	}
}

func readWSMessage(t *testing.T, conn *websocket.Conn) wsMessage {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read WebSocket message: %v", err)
	}

	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("failed to decode WebSocket message %s: %v", string(data), err)
	}
	return msg
}

func waitForSubscriberCount(t *testing.T, pubsub *PubSub, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := pubsub.SubscriberCount(); got >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("subscriber count did not reach %d; got %d", want, pubsub.SubscriberCount())
}
