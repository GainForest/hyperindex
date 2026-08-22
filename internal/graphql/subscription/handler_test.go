package subscription

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestSubscriptionResultIsEmpty(t *testing.T) {
	if !subscriptionResultIsEmpty(nil) {
		t.Fatal("nil result should be empty")
	}
	if !subscriptionResultIsEmpty(map[string]interface{}{"typedEvents": nil}) {
		t.Fatal("null-only typed result should be empty")
	}
	if subscriptionResultIsEmpty(map[string]interface{}{"recordEvents": map[string]interface{}{"type": "create"}}) {
		t.Fatal("non-null raw result should not be empty")
	}
}

func TestValidateSubscriptionOperationResolvesRecordEventsCollectionForRouting(t *testing.T) {
	schema := routingTestSchema(t)
	tests := []struct {
		name          string
		query         string
		operationName string
		variables     map[string]interface{}
		want          string
	}{
		{name: "inline literal", query: `subscription { recordEvents(collection: "com.example.match") { uri } }`, want: "com.example.match"},
		{name: "arbitrarily named variable", query: `subscription Routed($wanted: String!) { recordEvents(collection: $wanted) { uri } }`, variables: map[string]interface{}{"wanted": "com.example.match"}, want: "com.example.match"},
		{name: "variable default", query: `subscription Routed($wanted: String = "com.example.match") { recordEvents(collection: $wanted) { uri } }`, want: "com.example.match"},
		{name: "include true", query: `subscription Routed($enabled: Boolean!) { recordEvents(collection: "com.example.match") @include(if: $enabled) { uri } }`, variables: map[string]interface{}{"enabled": true}, want: "com.example.match"},
		{name: "include false broad", query: `subscription Routed($enabled: Boolean!) { recordEvents(collection: "com.example.match") @include(if: $enabled) { uri } }`, variables: map[string]interface{}{"enabled": false}, want: ""},
		{name: "skip false", query: `subscription Routed($disabled: Boolean!) { recordEvents(collection: "com.example.match") @skip(if: $disabled) { uri } }`, variables: map[string]interface{}{"disabled": false}, want: "com.example.match"},
		{name: "skip true broad", query: `subscription Routed($disabled: Boolean!) { recordEvents(collection: "com.example.match") @skip(if: $disabled) { uri } }`, variables: map[string]interface{}{"disabled": true}, want: ""},
		{name: "boolean default", query: `subscription Routed($enabled: Boolean = true) { ...Events @include(if: $enabled) } fragment Events on Subscription { recordEvents(collection: "com.example.match") { uri } }`, want: "com.example.match"},
		{name: "fragment selection", query: `subscription { ...Events } fragment Events on Subscription { recordEvents(collection: "com.example.match") { uri } }`, want: "com.example.match"},
		{name: "named operation", query: `subscription Other { recordEvents(collection: "com.example.other") { uri } } subscription Routed { recordEvents(collection: "com.example.match") { uri } }`, operationName: "Routed", want: "com.example.match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collection, err := validateSubscriptionOperation(&schema, subscribePayload{Query: test.query, Variables: test.variables, OperationName: test.operationName})
			if err != nil {
				t.Fatalf("validateSubscriptionOperation() error = %v", err)
			}
			if collection != test.want {
				t.Fatalf("collection = %q, want %q", collection, test.want)
			}

			if test.want == "" {
				return
			}
			pubsub := NewPubSub()
			subscriber := pubsub.Subscribe(collection)
			defer pubsub.Unsubscribe(subscriber)
			for i := 0; i < SubscriberBufferSize*2; i++ {
				pubsub.Publish(&RecordEvent{URI: fmt.Sprintf("at://did:plc:test/com.example.other/%d", i), Collection: "com.example.other"})
			}
			pubsub.Publish(&RecordEvent{URI: "at://did:plc:test/com.example.match/expected", Collection: "com.example.match"})
			select {
			case event := <-subscriber.Events:
				if event.Collection != "com.example.match" {
					t.Fatalf("buffer-pressure route delivered %q, want matching collection", event.Collection)
				}
			case <-time.After(time.Second):
				t.Fatal("matching event was dropped after unrelated buffer pressure")
			}
		})
	}
}

func routingTestSchema(t *testing.T) graphql.Schema {
	t.Helper()
	eventType := graphql.NewObject(graphql.ObjectConfig{Name: "RoutingEvent", Fields: graphql.Fields{
		"uri": &graphql.Field{Type: graphql.String},
	}})
	subscriptionType := graphql.NewObject(graphql.ObjectConfig{Name: "Subscription", Fields: graphql.Fields{
		"recordEvents": &graphql.Field{Type: eventType, Args: graphql.FieldConfigArgument{
			"collection": &graphql.ArgumentConfig{Type: graphql.String},
		}},
	}})
	queryType := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"ok": &graphql.Field{Type: graphql.String},
	}})
	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType, Subscription: subscriptionType})
	if err != nil {
		t.Fatalf("NewSchema() error = %v", err)
	}
	return schema
}

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
