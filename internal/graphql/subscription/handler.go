package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
)

const (
	// WebSocket subprotocol for GraphQL
	graphqlWSProtocol = "graphql-transport-ws"

	// Message types for graphql-transport-ws protocol
	msgConnectionInit      = "connection_init"
	msgConnectionAck       = "connection_ack"
	msgPing                = "ping"
	msgPong                = "pong"
	msgSubscribe           = "subscribe"
	msgNext                = "next"
	msgError               = "error"
	msgComplete            = "complete"
	msgConnectionTerminate = "connection_terminate"

	closeUnauthorized            = 4401
	closeSubscriberAlreadyExists = 4409
	closeTooManyInitializations  = 4429

	closeReasonSubscriberAlreadyExists = "Subscriber already exists"
)

// wsMessage represents a WebSocket message.
type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// subscribePayload is the payload for subscribe messages.
type subscribePayload struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
	OperationName string                 `json:"operationName,omitempty"`
}

// Handler handles WebSocket connections for GraphQL subscriptions.
type Handler struct {
	schema   *graphql.Schema
	pubsub   *PubSub
	repos    *resolver.Repositories
	upgrader websocket.Upgrader
}

// NewHandler creates a new subscription handler.
// allowedOrigins controls which origins may open WebSocket connections.
// Pass []string{"*"} to allow all origins (development only).
// Pass nil or empty slice to enforce same-origin policy.
func NewHandler(schema *graphql.Schema, pubsub *PubSub, repos *resolver.Repositories, allowedOrigins []string) *Handler {
	return &Handler{
		schema: schema,
		pubsub: pubsub,
		repos:  repos,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{graphqlWSProtocol},
			CheckOrigin:  makeOriginChecker(allowedOrigins),
		},
	}
}

// makeOriginChecker returns a CheckOrigin function based on the allowed origins list.
func makeOriginChecker(allowedOrigins []string) func(r *http.Request) bool {
	// No origins configured or explicitly set to "*": allow all origins.
	// This matches the CORS middleware default behavior. To restrict origins,
	// set ALLOWED_ORIGINS to a comma-separated list of specific origins.
	if len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*") {
		if len(allowedOrigins) == 0 {
			slog.Warn("WebSocket CheckOrigin allows all origins (ALLOWED_ORIGINS not configured)")
		} else {
			slog.Warn("WebSocket CheckOrigin allows all origins (ALLOWED_ORIGINS=\"*\")")
		}
		return func(r *http.Request) bool {
			return true
		}
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Same-origin requests don't send Origin header
		}
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		slog.Warn("WebSocket connection rejected: origin not allowed",
			"origin", origin,
			"allowed_origins", allowedOrigins)
		return false
	}
}

// ServeHTTP upgrades HTTP to WebSocket and handles subscriptions.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}

	client := &wsClient{
		conn:          conn,
		schema:        h.schema,
		pubsub:        h.pubsub,
		repos:         h.repos,
		subscriptions: make(map[string]*subscriptionOperation),
	}

	go client.run()
}

// wsClient manages a single WebSocket connection.
type subscriptionOperation struct {
	cancel context.CancelFunc
}

type wsClient struct {
	conn          *websocket.Conn
	schema        *graphql.Schema
	pubsub        *PubSub
	repos         *resolver.Repositories
	subscriptions map[string]*subscriptionOperation
	mu            sync.Mutex
	initialized   bool
	closed        bool
}

// run handles the WebSocket connection lifecycle.
func (c *wsClient) run() {
	defer c.close()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Debug("WebSocket closed unexpectedly", "error", err)
			}
			return
		}

		var msg wsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Debug("Invalid WebSocket message", "error", err)
			continue
		}

		c.handleMessage(&msg)
	}
}

// handleMessage processes incoming WebSocket messages.
func (c *wsClient) handleMessage(msg *wsMessage) {
	switch msg.Type {
	case msgConnectionInit:
		if c.initialized {
			c.closeWithCode(closeTooManyInitializations, "Too many initialization requests")
			return
		}
		c.initialized = true
		c.send(&wsMessage{Type: msgConnectionAck})

	case msgPing:
		c.send(&wsMessage{Type: msgPong})

	case msgSubscribe:
		if !c.initialized {
			c.closeWithCode(closeUnauthorized, "Unauthorized")
			return
		}
		c.handleSubscribe(msg)

	case msgComplete:
		c.cancelSubscription(msg.ID)

	case msgConnectionTerminate:
		c.close()
	}
}

// handleSubscribe validates and registers a new subscription operation.
func (c *wsClient) handleSubscribe(msg *wsMessage) {
	if msg.ID == "" {
		c.sendError(msg.ID, "Subscription operation id is required")
		return
	}
	c.mu.Lock()
	_, duplicate := c.subscriptions[msg.ID]
	c.mu.Unlock()
	if duplicate {
		c.closeWithCode(closeSubscriberAlreadyExists, closeReasonSubscriberAlreadyExists)
		return
	}

	var payload subscribePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		c.sendError(msg.ID, "Invalid subscribe payload")
		return
	}
	collection, err := validateSubscriptionOperation(c.schema, payload)
	if err != nil {
		c.sendError(msg.ID, err.Error())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = resolver.WithRepositories(ctx, c.repos)
	op := &subscriptionOperation{cancel: cancel}

	c.mu.Lock()
	if _, duplicate := c.subscriptions[msg.ID]; duplicate {
		c.mu.Unlock()
		cancel()
		c.closeWithCode(closeSubscriberAlreadyExists, closeReasonSubscriberAlreadyExists)
		return
	}
	c.subscriptions[msg.ID] = op
	c.mu.Unlock()

	go c.runSubscription(ctx, msg.ID, op, payload, collection)
}

// runSubscription executes a validated subscription and sends events.
func (c *wsClient) runSubscription(ctx context.Context, id string, op *subscriptionOperation, payload subscribePayload, collection string) {
	defer c.finishSubscription(id, op, true)

	sub := c.pubsub.Subscribe(collection)
	defer c.pubsub.Unsubscribe(sub)

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-sub.Events:
			if !ok {
				return
			}

			rootObject := map[string]interface{}{"__recordEvent": event}
			result := graphql.Do(graphql.Params{
				Schema:         *c.schema,
				RequestString:  payload.Query,
				OperationName:  payload.OperationName,
				VariableValues: payload.Variables,
				Context:        ctx,
				RootObject:     rootObject,
			})

			if len(result.Errors) > 0 {
				errPayload, _ := json.Marshal(result.Errors)
				c.sendOperationMessage(id, op, &wsMessage{ID: id, Type: msgError, Payload: errPayload}, true)
				return
			}
			if subscriptionResultIsEmpty(result.Data) {
				continue
			}
			dataPayload, _ := json.Marshal(map[string]interface{}{"data": result.Data})
			if !c.sendOperationMessage(id, op, &wsMessage{ID: id, Type: msgNext, Payload: dataPayload}, false) {
				return
			}
		}
	}
}

func validateSubscriptionOperation(schema *graphql.Schema, payload subscribePayload) (string, error) {
	if schema == nil {
		return "", fmt.Errorf("subscription schema is unavailable")
	}
	if strings.TrimSpace(payload.Query) == "" {
		return "", fmt.Errorf("subscription query is required")
	}
	document, err := parser.Parse(parser.ParseParams{Source: payload.Query})
	if err != nil {
		return "", fmt.Errorf("invalid subscription query: %w", err)
	}
	validationResult := graphql.ValidateDocument(schema, document, nil)
	if !validationResult.IsValid {
		if len(validationResult.Errors) > 0 {
			return "", fmt.Errorf("invalid subscription query: %s", validationResult.Errors[0].Message)
		}
		return "", fmt.Errorf("invalid subscription query")
	}

	operation, err := selectedSubscriptionOperation(document, payload.OperationName)
	if err != nil {
		return "", err
	}
	if operation.Operation != ast.OperationTypeSubscription {
		return "", fmt.Errorf("operation must be a subscription")
	}
	// Execute once with an empty source to validate operation selection and
	// variable coercion before allocating a PubSub subscriber. Subscription root
	// resolvers return nil for the empty source and have no side effects.
	preflight := graphql.Do(graphql.Params{
		Schema:         *schema,
		RequestString:  payload.Query,
		OperationName:  payload.OperationName,
		VariableValues: payload.Variables,
		Context:        context.Background(),
		RootObject:     map[string]interface{}{},
	})
	if len(preflight.Errors) > 0 {
		return "", fmt.Errorf("invalid subscription operation: %s", preflight.Errors[0].Message)
	}
	return selectedRecordEventsCollection(document, operation, payload.Variables), nil
}

func selectedSubscriptionOperation(document *ast.Document, operationName string) (*ast.OperationDefinition, error) {
	var operations []*ast.OperationDefinition
	for _, definition := range document.Definitions {
		operation, ok := definition.(*ast.OperationDefinition)
		if !ok {
			continue
		}
		operations = append(operations, operation)
		if operationName != "" && operation.Name != nil && operation.Name.Value == operationName {
			return operation, nil
		}
	}
	if operationName != "" {
		return nil, fmt.Errorf("subscription operation %q was not found", operationName)
	}
	if len(operations) != 1 {
		return nil, fmt.Errorf("operationName is required when a document contains multiple operations")
	}
	return operations[0], nil
}

func selectedRecordEventsCollection(document *ast.Document, operation *ast.OperationDefinition, variables map[string]interface{}) string {
	resolvedVariables := make(map[string]interface{}, len(variables))
	for name, value := range variables {
		resolvedVariables[name] = value
	}
	for _, definition := range operation.VariableDefinitions {
		if definition.Variable == nil || definition.Variable.Name == nil {
			continue
		}
		name := definition.Variable.Name.Value
		if _, provided := resolvedVariables[name]; provided {
			continue
		}
		switch defaultValue := definition.DefaultValue.(type) {
		case *ast.StringValue:
			resolvedVariables[name] = defaultValue.Value
		case *ast.BooleanValue:
			resolvedVariables[name] = defaultValue.Value
		}
	}
	fragments := make(map[string]*ast.FragmentDefinition)
	for _, definition := range document.Definitions {
		if fragment, ok := definition.(*ast.FragmentDefinition); ok && fragment.Name != nil {
			fragments[fragment.Name.Value] = fragment
		}
	}
	route := &subscriptionRoute{}
	collectRecordEventsRoutes(operation.SelectionSet, fragments, resolvedVariables, make(map[string]bool), route)
	if route.broad || !route.set {
		return ""
	}
	return route.collection
}

type subscriptionRoute struct {
	collection string
	set        bool
	broad      bool
}

func (r *subscriptionRoute) add(collection string, proven bool) {
	if !proven || collection == "" {
		r.broad = true
		return
	}
	if r.set && r.collection != collection {
		r.broad = true
		return
	}
	r.collection = collection
	r.set = true
}

func collectRecordEventsRoutes(selectionSet *ast.SelectionSet, fragments map[string]*ast.FragmentDefinition, variables map[string]interface{}, stack map[string]bool, route *subscriptionRoute) {
	if selectionSet == nil || route.broad {
		return
	}
	for _, selection := range selectionSet.Selections {
		switch selection := selection.(type) {
		case *ast.Field:
			included, known := directivesInclude(selection.Directives, variables)
			if !known {
				route.broad = true
				return
			}
			if !included || selection.Name == nil || selection.Name.Value != "recordEvents" {
				continue
			}
			collection, proven := recordEventsCollectionArgument(selection.Arguments, variables)
			route.add(collection, proven)
		case *ast.InlineFragment:
			included, known := directivesInclude(selection.Directives, variables)
			if !known {
				route.broad = true
				return
			}
			if included {
				collectRecordEventsRoutes(selection.SelectionSet, fragments, variables, stack, route)
			}
		case *ast.FragmentSpread:
			included, known := directivesInclude(selection.Directives, variables)
			if !known {
				route.broad = true
				return
			}
			if !included || selection.Name == nil || stack[selection.Name.Value] {
				continue
			}
			fragment := fragments[selection.Name.Value]
			if fragment == nil {
				route.broad = true
				return
			}
			included, known = directivesInclude(fragment.Directives, variables)
			if !known {
				route.broad = true
				return
			}
			if !included {
				continue
			}
			stack[selection.Name.Value] = true
			collectRecordEventsRoutes(fragment.SelectionSet, fragments, variables, stack, route)
			delete(stack, selection.Name.Value)
		}
	}
}

func recordEventsCollectionArgument(arguments []*ast.Argument, variables map[string]interface{}) (string, bool) {
	for _, argument := range arguments {
		if argument.Name == nil || argument.Name.Value != "collection" {
			continue
		}
		switch value := argument.Value.(type) {
		case *ast.StringValue:
			return value.Value, true
		case *ast.Variable:
			if value.Name != nil {
				collection, ok := variables[value.Name.Value].(string)
				return collection, ok
			}
		}
		return "", false
	}
	return "", false
}

func directivesInclude(directives []*ast.Directive, variables map[string]interface{}) (bool, bool) {
	included := true
	for _, directive := range directives {
		if directive.Name == nil || (directive.Name.Value != "skip" && directive.Name.Value != "include") {
			continue
		}
		condition, known := directiveCondition(directive.Arguments, variables)
		if !known {
			return false, false
		}
		if directive.Name.Value == "skip" && condition {
			included = false
		}
		if directive.Name.Value == "include" && !condition {
			included = false
		}
	}
	return included, true
}

func directiveCondition(arguments []*ast.Argument, variables map[string]interface{}) (bool, bool) {
	for _, argument := range arguments {
		if argument.Name == nil || argument.Name.Value != "if" {
			continue
		}
		switch value := argument.Value.(type) {
		case *ast.BooleanValue:
			return value.Value, true
		case *ast.Variable:
			if value.Name != nil {
				condition, ok := variables[value.Name.Value].(bool)
				return condition, ok
			}
		}
	}
	return false, false
}

func subscriptionResultIsEmpty(data interface{}) bool {
	fields, ok := data.(map[string]interface{})
	if !ok || len(fields) == 0 {
		return true
	}
	for _, value := range fields {
		if value != nil {
			return false
		}
	}
	return true
}

// cancelSubscription cancels and removes an active subscription.
func (c *wsClient) cancelSubscription(id string) {
	c.mu.Lock()
	op, ok := c.subscriptions[id]
	if ok {
		delete(c.subscriptions, id)
	}
	c.mu.Unlock()
	if ok {
		op.cancel()
	}
}

func (c *wsClient) finishSubscription(id string, op *subscriptionOperation, sendComplete bool) {
	c.mu.Lock()
	if current, ok := c.subscriptions[id]; !ok || current != op {
		c.mu.Unlock()
		return
	}
	delete(c.subscriptions, id)
	c.mu.Unlock()
	if sendComplete {
		c.send(&wsMessage{ID: id, Type: msgComplete})
	}
}

// sendError sends an error message.
func (c *wsClient) sendError(id, message string) {
	errPayload, _ := json.Marshal([]map[string]string{
		{"message": message},
	})
	c.send(&wsMessage{
		ID:      id,
		Type:    msgError,
		Payload: errPayload,
	})
}

func (c *wsClient) sendOperationMessage(id string, op *subscriptionOperation, msg *wsMessage, terminal bool) bool {
	data, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if current, ok := c.subscriptions[id]; !ok || current != op {
		return false
	}
	if terminal {
		delete(c.subscriptions, id)
		op.cancel()
	}
	return c.writeMessageLocked(data)
}

// send writes a message to the WebSocket.
func (c *wsClient) send(msg *wsMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeMessageLocked(data)
}

func (c *wsClient) writeMessageLocked(data []byte) bool {
	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return false
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		slog.Debug("WebSocket write failed", "error", err)
		return false
	}
	return true
}

func (c *wsClient) closeWithCode(code int, reason string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for id, op := range c.subscriptions {
		op.cancel()
		delete(c.subscriptions, id)
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(10*time.Second)); err != nil {
		slog.Debug("WebSocket close control write failed", "code", code, "error", err)
	}
	c.mu.Unlock()
	_ = c.conn.Close()
}

// close closes the WebSocket connection and all subscriptions.
func (c *wsClient) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for id, op := range c.subscriptions {
		op.cancel()
		delete(c.subscriptions, id)
	}
	c.mu.Unlock()

	_ = c.conn.Close()
}
