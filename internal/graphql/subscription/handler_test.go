package subscription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/authoridentity"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestHandlerSubscriptionResolvesAuthorHandle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	if err := db.Actors.UpsertIdentity(ctx, "did:plc:alice", "alice.example"); err != nil {
		t.Fatalf("UpsertIdentity() error = %v", err)
	}

	recordEventType := graphql.NewObject(graphql.ObjectConfig{
		Name: "TestRecordEvent",
		Fields: graphql.Fields{
			"did":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"author": authoridentity.Field(),
		},
	})
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "TestQuery",
		Fields: graphql.Fields{
			"ok": &graphql.Field{Type: graphql.Boolean, Resolve: func(graphql.ResolveParams) (interface{}, error) { return true, nil }},
		},
	})
	subscriptionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "TestSubscription",
		Fields: graphql.Fields{
			"recordEvents": &graphql.Field{
				Type: recordEventType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					root, _ := p.Source.(map[string]interface{})
					return root["recordEvents"], nil
				},
			},
		},
	})
	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType, Subscription: subscriptionType})
	if err != nil {
		t.Fatalf("NewSchema() error = %v", err)
	}

	pubsub := NewPubSub()
	handler := NewHandler(&schema, pubsub, &resolver.Repositories{Actors: db.Actors}, []string{"*"})
	server := httptest.NewServer(handler)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{graphqlWSProtocol}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]interface{}{"type": msgConnectionInit}); err != nil {
		t.Fatalf("write connection_init: %v", err)
	}
	var message wsMessage
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read connection_ack: %v", err)
	}
	if message.Type != msgConnectionAck {
		t.Fatalf("connection response type = %q, want %q", message.Type, msgConnectionAck)
	}

	if err := conn.WriteJSON(map[string]interface{}{
		"id":   "author-subscription",
		"type": msgSubscribe,
		"payload": map[string]interface{}{
			"query": `subscription { recordEvents { did author { did handle } } }`,
		},
	}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for pubsub.SubscriberCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if pubsub.SubscriberCount() == 0 {
		t.Fatal("subscription was not registered")
	}

	pubsub.Publish(&RecordEvent{DID: "did:plc:alice"})
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read subscription event: %v", err)
	}
	if message.Type != msgNext {
		t.Fatalf("subscription response type = %q, want %q", message.Type, msgNext)
	}
	var payload struct {
		Data struct {
			RecordEvents struct {
				DID    string `json:"did"`
				Author struct {
					DID    string `json:"did"`
					Handle string `json:"handle"`
				} `json:"author"`
			} `json:"recordEvents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatalf("decode subscription payload: %v", err)
	}
	if payload.Data.RecordEvents.DID != "did:plc:alice" || payload.Data.RecordEvents.Author.DID != "did:plc:alice" || payload.Data.RecordEvents.Author.Handle != "alice.example" {
		t.Fatalf("subscription payload = %+v", payload.Data.RecordEvents)
	}
}

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
