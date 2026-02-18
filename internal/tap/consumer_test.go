package tap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// upgrader is used by the mock WebSocket server.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// mockHandler is a test implementation of EventHandler.
type mockHandler struct {
	mu             sync.Mutex
	recordEvents   []*RecordEvent
	identityEvents []*IdentityEvent
	recordErr      error
	identityErr    error
}

func (m *mockHandler) HandleRecord(_ context.Context, event *RecordEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordEvents = append(m.recordEvents, event)
	return m.recordErr
}

func (m *mockHandler) HandleIdentity(_ context.Context, event *IdentityEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.identityEvents = append(m.identityEvents, event)
	return m.identityErr
}

func (m *mockHandler) RecordCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.recordEvents)
}

func (m *mockHandler) IdentityCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.identityEvents)
}

// newTestServer creates a mock WebSocket server that calls serverFn for each connection.
// serverFn receives the WebSocket connection and can send messages and read acks.
func newTestServer(t *testing.T, serverFn func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/channel", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()
		serverFn(conn)
	})
	return httptest.NewServer(mux)
}

// wsURL converts an http:// test server URL to ws://.
func wsURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

// sendEvent sends a Tap event JSON to the WebSocket connection.
func sendEvent(t *testing.T, conn *websocket.Conn, event Event) {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Logf("write error (may be expected on close): %v", err)
	}
}

// readAck reads one text message from the connection and returns it.
func readAck(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read ack: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("expected text message for ack, got type %d", msgType)
	}
	return string(data)
}

// TestConsumer_ConnectsToChannel verifies the consumer connects to /channel.
func TestConsumer_ConnectsToChannel(t *testing.T) {
	connected := make(chan struct{})

	srv := newTestServer(t, func(conn *websocket.Conn) {
		close(connected)
		// Keep connection open briefly then close.
		time.Sleep(100 * time.Millisecond)
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	select {
	case <-connected:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("consumer did not connect within timeout")
	}

	consumer.Stop()
}

// TestConsumer_ReceivesAndAcksRecordEvent verifies record events are dispatched and acked.
func TestConsumer_ReceivesAndAcksRecordEvent(t *testing.T) {
	ackReceived := make(chan string, 1)

	recordEvent := Event{
		ID:   12345,
		Type: EventTypeRecord,
		Record: &RecordEvent{
			Live:       true,
			Rev:        "abc123",
			DID:        "did:plc:alice",
			Collection: "app.bsky.feed.post",
			RKey:       "rkey1",
			Action:     ActionCreate,
			CID:        "bafyreiabc",
			Record:     json.RawMessage(`{"text":"hello"}`),
		},
	}

	srv := newTestServer(t, func(conn *websocket.Conn) {
		sendEvent(t, conn, recordEvent)
		ack := readAck(t, conn)
		ackReceived <- ack
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	select {
	case ack := <-ackReceived:
		if ack != "12345" {
			t.Errorf("expected ack '12345', got %q", ack)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive ack within timeout")
	}

	consumer.Stop()

	if handler.RecordCount() != 1 {
		t.Errorf("expected 1 record event, got %d", handler.RecordCount())
	}
}

// TestConsumer_ReceivesAndAcksIdentityEvent verifies identity events are dispatched and acked.
func TestConsumer_ReceivesAndAcksIdentityEvent(t *testing.T) {
	ackReceived := make(chan string, 1)

	identityEvent := Event{
		ID:   12346,
		Type: EventTypeIdentity,
		Identity: &IdentityEvent{
			DID:      "did:plc:alice",
			Handle:   "alice.bsky.social",
			IsActive: true,
			Status:   "active",
		},
	}

	srv := newTestServer(t, func(conn *websocket.Conn) {
		sendEvent(t, conn, identityEvent)
		ack := readAck(t, conn)
		ackReceived <- ack
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	select {
	case ack := <-ackReceived:
		if ack != "12346" {
			t.Errorf("expected ack '12346', got %q", ack)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive ack within timeout")
	}

	consumer.Stop()

	if handler.IdentityCount() != 1 {
		t.Errorf("expected 1 identity event, got %d", handler.IdentityCount())
	}
}

// TestConsumer_DisableAcks verifies no ack is sent when DisableAcks=true.
func TestConsumer_DisableAcks(t *testing.T) {
	eventHandled := make(chan struct{})
	ackReceived := make(chan struct{})

	recordEvent := Event{
		ID:   99,
		Type: EventTypeRecord,
		Record: &RecordEvent{
			Live:       true,
			DID:        "did:plc:bob",
			Collection: "app.bsky.feed.post",
			RKey:       "rkey2",
			Action:     ActionCreate,
		},
	}

	srv := newTestServer(t, func(conn *websocket.Conn) {
		sendEvent(t, conn, recordEvent)
		// Try to read an ack with a short timeout — should not arrive.
		_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		_, _, err := conn.ReadMessage()
		if err == nil {
			close(ackReceived)
		}
		// Signal that we've processed the event check.
		close(eventHandled)
		// Keep connection open so consumer doesn't reconnect.
		time.Sleep(500 * time.Millisecond)
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{
		TapURL:      wsURL(srv.URL),
		DisableAcks: true,
	}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	select {
	case <-eventHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish within timeout")
	}

	select {
	case <-ackReceived:
		t.Error("received unexpected ack when DisableAcks=true")
	default:
		// Good — no ack received.
	}

	consumer.Stop()
}

// TestConsumer_ReconnectsOnConnectionLoss verifies reconnection with backoff.
func TestConsumer_ReconnectsOnConnectionLoss(t *testing.T) {
	var connectionCount int32

	srv := newTestServer(t, func(conn *websocket.Conn) {
		count := atomic.AddInt32(&connectionCount, 1)
		if count == 1 {
			// First connection: close immediately to trigger reconnect.
			conn.Close()
			return
		}
		// Second connection: stay open briefly.
		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	handler := &mockHandler{}
	// Use a very short initial backoff for testing by overriding via a custom consumer.
	// We can't easily override minBackoff, so we just wait long enough for the default 1s backoff.
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	// Wait for at least 2 connections (initial + reconnect after 1s backoff).
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&connectionCount) >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	consumer.Stop()

	if atomic.LoadInt32(&connectionCount) < 2 {
		t.Errorf("expected at least 2 connections (reconnect), got %d", atomic.LoadInt32(&connectionCount))
	}
}

// TestConsumer_StopGracefully verifies Stop closes the connection cleanly.
func TestConsumer_StopGracefully(t *testing.T) {
	serverDone := make(chan struct{})

	srv := newTestServer(t, func(conn *websocket.Conn) {
		defer close(serverDone)
		// Read until connection closes.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	started := make(chan struct{})
	go func() {
		close(started)
		_ = consumer.Start(ctx)
	}()

	<-started
	// Give the consumer time to connect.
	time.Sleep(100 * time.Millisecond)

	consumer.Stop()

	select {
	case <-serverDone:
		// Server saw the connection close — graceful shutdown confirmed.
	case <-time.After(2 * time.Second):
		t.Fatal("server did not see connection close after Stop()")
	}
}

// TestConsumer_Stats verifies Stats() returns correct event counts.
func TestConsumer_Stats(t *testing.T) {
	allAcked := make(chan struct{})
	var ackCount int32

	events := []Event{
		{
			ID:   1,
			Type: EventTypeRecord,
			Record: &RecordEvent{
				DID: "did:plc:a", Collection: "app.bsky.feed.post", RKey: "r1",
				Action: ActionCreate,
			},
		},
		{
			ID:   2,
			Type: EventTypeRecord,
			Record: &RecordEvent{
				DID: "did:plc:a", Collection: "app.bsky.feed.post", RKey: "r1",
				Action: ActionUpdate,
			},
		},
		{
			ID:   3,
			Type: EventTypeRecord,
			Record: &RecordEvent{
				DID: "did:plc:a", Collection: "app.bsky.feed.post", RKey: "r1",
				Action: ActionDelete,
			},
		},
		{
			ID:       4,
			Type:     EventTypeIdentity,
			Identity: &IdentityEvent{DID: "did:plc:a", Handle: "alice.bsky.social", IsActive: true, Status: "active"},
		},
	}

	srv := newTestServer(t, func(conn *websocket.Conn) {
		for _, ev := range events {
			sendEvent(t, conn, ev)
		}
		// Read all acks.
		for i := 0; i < len(events); i++ {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if atomic.AddInt32(&ackCount, 1) == int32(len(events)) {
				close(allAcked)
			}
		}
		// Keep connection open.
		time.Sleep(500 * time.Millisecond)
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	select {
	case <-allAcked:
	case <-time.After(4 * time.Second):
		t.Fatal("did not receive all acks within timeout")
	}

	consumer.Stop()

	stats := consumer.Stats()
	if stats.EventsReceived != 4 {
		t.Errorf("EventsReceived: want 4, got %d", stats.EventsReceived)
	}
	if stats.RecordsCreated != 1 {
		t.Errorf("RecordsCreated: want 1, got %d", stats.RecordsCreated)
	}
	if stats.RecordsUpdated != 1 {
		t.Errorf("RecordsUpdated: want 1, got %d", stats.RecordsUpdated)
	}
	if stats.RecordsDeleted != 1 {
		t.Errorf("RecordsDeleted: want 1, got %d", stats.RecordsDeleted)
	}
	if stats.IdentityEvents != 1 {
		t.Errorf("IdentityEvents: want 1, got %d", stats.IdentityEvents)
	}
	if stats.Errors != 0 {
		t.Errorf("Errors: want 0, got %d", stats.Errors)
	}
}

// TestConsumer_StopDuringDispatch verifies that calling Stop() concurrently with
// active event dispatching does not cause a data race. gorilla/websocket does not
// allow concurrent writers, so Stop() must not call WriteMessage while dispatch()
// may be sending acks.
func TestConsumer_StopDuringDispatch(t *testing.T) {
	const numEvents = 200

	srv := newTestServer(t, func(conn *websocket.Conn) {
		// Send many events rapidly to keep dispatch() busy writing acks.
		for i := 0; i < numEvents; i++ {
			ev := Event{
				ID:   int64(i + 1),
				Type: EventTypeRecord,
				Record: &RecordEvent{
					DID:        "did:plc:race",
					Collection: "app.bsky.feed.post",
					RKey:       fmt.Sprintf("rkey%d", i),
					Action:     ActionCreate,
				},
			}
			data, err := json.Marshal(ev)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				// Connection may have been closed by Stop() — that is expected.
				return
			}
		}
		// Drain any acks that arrive before the connection closes.
		for {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	// Let the consumer connect and start receiving events, then stop it
	// concurrently while dispatch() is actively writing acks.
	time.Sleep(10 * time.Millisecond)
	consumer.Stop()
}

// TestConsumer_BackoffResetsAfterSuccess verifies that after a successful connection
// the backoff is reset to minBackoff, so subsequent reconnections are fast.
func TestConsumer_BackoffResetsAfterSuccess(t *testing.T) {
	var connectionCount int32
	// secondConnected is closed when the second connection is established.
	secondConnected := make(chan struct{})
	// thirdConnected is closed when the third connection is established.
	thirdConnected := make(chan struct{})

	recordEvent := Event{
		ID:   1,
		Type: EventTypeRecord,
		Record: &RecordEvent{
			DID:        "did:plc:backoff",
			Collection: "app.bsky.feed.post",
			RKey:       "rkey1",
			Action:     ActionCreate,
		},
	}

	srv := newTestServer(t, func(conn *websocket.Conn) {
		count := atomic.AddInt32(&connectionCount, 1)
		switch count {
		case 1:
			// First connection: close immediately to trigger backoff escalation.
			conn.Close()
		case 2:
			// Second connection: signal connected, send one event, then close cleanly.
			close(secondConnected)
			sendEvent(t, conn, recordEvent)
			// Read the ack so dispatch completes successfully.
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			conn.ReadMessage() //nolint:errcheck
			// Close with a normal closure so runOnce returns nil (success).
			conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		case 3:
			// Third connection: signal that we reconnected quickly.
			close(thirdConnected)
			// Keep open briefly.
			time.Sleep(200 * time.Millisecond)
		}
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	// Wait for the second connection (after the 1s backoff from the first failure).
	select {
	case <-secondConnected:
	case <-time.After(5 * time.Second):
		t.Fatal("second connection did not arrive within timeout")
	}

	// After the second connection closes cleanly, backoff should reset to minBackoff (1s).
	// The third connection should arrive within ~1.5s, not 2s+ (which would indicate
	// the backoff was not reset and stayed at 2s from the first failure).
	start := time.Now()
	select {
	case <-thirdConnected:
		elapsed := time.Since(start)
		// Should reconnect within 1.5s (minBackoff=1s + some slack).
		// If backoff was NOT reset it would wait 2s (doubled from 1s after first failure).
		if elapsed > 1500*time.Millisecond {
			t.Errorf("third connection took %v; expected <1.5s (backoff should have reset to minBackoff)", elapsed)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("third connection did not arrive within timeout")
	}

	consumer.Stop()
}

// TestConsumer_LargeMessageRejected verifies that a message exceeding maxMessageSize
// causes a read error and triggers reconnection rather than OOM.
func TestConsumer_LargeMessageRejected(t *testing.T) {
	var connectionCount int32
	secondConnected := make(chan struct{})

	srv := newTestServer(t, func(conn *websocket.Conn) {
		count := atomic.AddInt32(&connectionCount, 1)
		switch count {
		case 1:
			// First connection: send a message larger than 4MB.
			oversized := make([]byte, maxMessageSize+1)
			for i := range oversized {
				oversized[i] = 'x'
			}
			// The write may succeed on the server side; the client will reject it on read.
			_ = conn.WriteMessage(websocket.TextMessage, oversized)
			// Wait briefly for the client to close the connection.
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			conn.ReadMessage() //nolint:errcheck
		case 2:
			// Second connection: signal that the consumer reconnected successfully.
			close(secondConnected)
			// Keep open briefly.
			time.Sleep(200 * time.Millisecond)
		}
	})
	defer srv.Close()

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	// The consumer should reject the oversized message and reconnect.
	select {
	case <-secondConnected:
		// Consumer reconnected after rejecting the large message — no OOM, no crash.
	case <-time.After(8 * time.Second):
		t.Fatal("consumer did not reconnect after receiving oversized message")
	}

	consumer.Stop()

	if atomic.LoadInt32(&connectionCount) < 2 {
		t.Errorf("expected at least 2 connections, got %d", atomic.LoadInt32(&connectionCount))
	}
}

// capturingHandler is a slog.Handler that records all log records.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(name string) slog.Handler       { return h }

func (h *capturingHandler) ErrorRecords() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []slog.Record
	for _, r := range h.records {
		if r.Level >= slog.LevelError {
			errs = append(errs, r)
		}
	}
	return errs
}

// TestConsumer_ShutdownNoSpuriousLog verifies that cancelling the context during
// shutdown does not produce any Error-level log messages.
func TestConsumer_ShutdownNoSpuriousLog(t *testing.T) {
	connected := make(chan struct{})

	srv := newTestServer(t, func(conn *websocket.Conn) {
		close(connected)
		// Keep connection open until the client disconnects.
		for {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	// Install a capturing slog handler for the duration of this test.
	capturing := &capturingHandler{}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(capturing))
	defer slog.SetDefault(origLogger)

	handler := &mockHandler{}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- consumer.Start(ctx)
	}()

	// Wait for the consumer to connect before cancelling.
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer did not connect within timeout")
	}

	// Cancel the context to trigger graceful shutdown.
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("Start() returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("consumer did not shut down within timeout")
	}

	// Verify no Error-level log messages were emitted during shutdown.
	if errRecords := capturing.ErrorRecords(); len(errRecords) > 0 {
		for _, r := range errRecords {
			t.Errorf("unexpected Error-level log during shutdown: %s", r.Message)
		}
	}
}

// TestConsumer_HandlerErrorDoesNotAck verifies that handler errors suppress acks.
func TestConsumer_HandlerErrorDoesNotAck(t *testing.T) {
	ackReceived := make(chan struct{})
	eventSent := make(chan struct{})

	recordEvent := Event{
		ID:   777,
		Type: EventTypeRecord,
		Record: &RecordEvent{
			DID: "did:plc:err", Collection: "app.bsky.feed.post", RKey: "r1",
			Action: ActionCreate,
		},
	}

	srv := newTestServer(t, func(conn *websocket.Conn) {
		sendEvent(t, conn, recordEvent)
		close(eventSent)
		// Try to read ack with short timeout.
		_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		_, _, err := conn.ReadMessage()
		if err == nil {
			close(ackReceived)
		}
		// Keep connection open.
		time.Sleep(500 * time.Millisecond)
	})
	defer srv.Close()

	handler := &mockHandler{recordErr: fmt.Errorf("handler error")}
	consumer := NewConsumer(ConsumerConfig{TapURL: wsURL(srv.URL)}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = consumer.Start(ctx)
	}()

	select {
	case <-eventSent:
	case <-time.After(2 * time.Second):
		t.Fatal("event was not sent within timeout")
	}

	// Wait a bit to ensure no ack arrives.
	time.Sleep(500 * time.Millisecond)

	select {
	case <-ackReceived:
		t.Error("received unexpected ack after handler error")
	default:
		// Good — no ack.
	}

	consumer.Stop()

	stats := consumer.Stats()
	if stats.Errors != 1 {
		t.Errorf("Errors: want 1, got %d", stats.Errors)
	}
}
