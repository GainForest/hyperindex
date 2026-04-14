package tap

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// tapChannelPath is the WebSocket endpoint path on the Tap server.
	tapChannelPath = "/channel"

	// defaultWriteTimeout is the timeout for WebSocket write operations.
	defaultWriteTimeout = 10 * time.Second

	// defaultReadTimeout is the timeout for WebSocket read operations.
	defaultReadTimeout = 60 * time.Second

	// minBackoff is the initial reconnection backoff duration.
	minBackoff = time.Second

	// maxBackoff is the maximum reconnection backoff duration.
	maxBackoff = 2 * time.Minute

	// maxMessageSize is the maximum WebSocket message size accepted from the Tap server.
	// AT Protocol records can be up to 1MB; 4MB gives headroom while preventing OOM from
	// malicious or buggy servers sending arbitrarily large messages.
	maxMessageSize = 4 * 1024 * 1024 // 4 MB
)

// ConsumerConfig configures the Tap consumer.
type ConsumerConfig struct {
	// TapURL is the WebSocket base URL (e.g., "ws://localhost:2480").
	TapURL string

	// Password is the Basic auth password for the /channel WebSocket endpoint.
	// The username is always "admin". Leave empty if Tap has no password set.
	Password string

	// DisableAcks puts the consumer in fire-and-forget mode (no acks sent).
	DisableAcks bool
}

// EventHandler processes Tap events. Return nil to ack, error to nack.
type EventHandler interface {
	HandleRecord(ctx context.Context, event *RecordEvent) error
	HandleIdentity(ctx context.Context, event *IdentityEvent) error
}

// Stats tracks consumer statistics.
type Stats struct {
	EventsReceived int64
	RecordsCreated int64
	RecordsUpdated int64
	RecordsDeleted int64
	IdentityEvents int64
	Errors         int64
}

// Consumer connects to Tap's WebSocket and dispatches events.
type Consumer struct {
	config  ConsumerConfig
	handler EventHandler

	// conn is the active WebSocket connection.
	conn   *websocket.Conn
	connMu sync.Mutex

	// stopOnce ensures Stop is idempotent.
	stopOnce sync.Once

	// done is closed when Stop is called.
	done chan struct{}

	// stats are updated atomically.
	eventsReceived int64
	recordsCreated int64
	recordsUpdated int64
	recordsDeleted int64
	identityEvents int64
	errors         int64
}

// NewConsumer creates a new Tap consumer.
func NewConsumer(config ConsumerConfig, handler EventHandler) *Consumer {
	return &Consumer{
		config:  config,
		handler: handler,
		done:    make(chan struct{}),
	}
}

// Start connects to Tap and begins processing events.
// Blocks until context is cancelled or Stop is called.
// Automatically reconnects on connection loss with exponential backoff.
func (c *Consumer) Start(ctx context.Context) error {
	backoff := minBackoff

	for {
		// Check if we should stop before attempting connection.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return nil
		default:
		}

		connected, err := c.runOnce(ctx)

		// Reset backoff if we successfully established a connection (even if it
		// later dropped with an error). This prevents slow reconnects after a
		// long-running session that ended with a network error.
		if connected {
			backoff = minBackoff
		}

		// Check if we should stop after connection ended.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return nil
		default:
		}

		// If context was cancelled, this is a graceful shutdown — do not log.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			slog.Warn("Tap connection lost, will reconnect",
				"error", err,
				"backoff", backoff,
			)
		} else {
			slog.Warn("Tap connection closed unexpectedly, will reconnect",
				"backoff", backoff,
			)
		}

		// Wait before reconnecting.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return nil
		case <-time.After(backoff):
		}

		// Exponential backoff with cap.
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		slog.Info("Attempting to reconnect to Tap...")
	}
}

// runOnce establishes one WebSocket connection and processes events until it closes.
// Returns (connected, error) where connected is true if the dial succeeded (even if
// the connection later dropped with an error). The caller uses connected to decide
// whether to reset the reconnection backoff.
func (c *Consumer) runOnce(ctx context.Context) (bool, error) {
	channelURL := c.config.TapURL + tapChannelPath

	slog.Info("Connecting to Tap", "url", channelURL)

	var header http.Header
	if c.config.Password != "" {
		creds := base64.StdEncoding.EncodeToString([]byte("admin:" + c.config.Password))
		header = http.Header{"Authorization": []string{"Basic " + creds}}
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, channelURL, header)
	if err != nil {
		return false, fmt.Errorf("failed to connect to Tap: %w", err)
	}
	conn.SetReadLimit(maxMessageSize)

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	defer func() {
		c.connMu.Lock()
		c.conn = nil
		c.connMu.Unlock()
		conn.Close()
	}()

	slog.Info("Connected to Tap", "url", channelURL)

	for {
		// Check for stop signal before reading.
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case <-c.done:
			return true, nil
		default:
		}

		// Set read deadline.
		if err := conn.SetReadDeadline(time.Now().Add(defaultReadTimeout)); err != nil {
			return true, fmt.Errorf("failed to set read deadline: %w", err)
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return true, nil
			}
			return true, fmt.Errorf("read error: %w", err)
		}

		if msgType != websocket.TextMessage {
			// Ignore non-text messages (e.g., ping/pong handled by gorilla automatically).
			continue
		}

		atomic.AddInt64(&c.eventsReceived, 1)

		event, err := ParseEvent(data)
		if err != nil {
			slog.Warn("Failed to parse Tap event", "error", err)
			atomic.AddInt64(&c.errors, 1)
			continue
		}

		if err := c.dispatch(ctx, conn, event); err != nil {
			atomic.AddInt64(&c.errors, 1)
			// If the error came from a write (ack failure), the connection is dead.
			// Return immediately to reconnect rather than continuing to read and
			// generating a cascade of identical errors.
			if isWriteError(err) {
				return true, fmt.Errorf("failed during Tap ack write: %w", err)
			}
			// Handler errors are non-fatal — log and continue processing.
			slog.Warn("Failed to handle Tap event",
				"event_id", event.ID,
				"type", event.Type,
				"error", err,
			)
		}
	}
}

// dispatch routes an event to the appropriate handler and sends an ack on success.
func (c *Consumer) dispatch(ctx context.Context, conn *websocket.Conn, event *Event) error {
	var handlerErr error

	switch {
	case event.IsRecord():
		handlerErr = c.handler.HandleRecord(ctx, event.Record)
		if handlerErr == nil {
			c.incrementRecordStat(event.Record.Action)
		}

	case event.IsIdentity():
		handlerErr = c.handler.HandleIdentity(ctx, event.Identity)
		if handlerErr == nil {
			atomic.AddInt64(&c.identityEvents, 1)
		}

	default:
		// Unknown event type — log and skip without acking.
		slog.Warn("Unknown Tap event type", "type", event.Type, "id", event.ID)
		return nil
	}

	if handlerErr != nil {
		return handlerErr
	}

	// Send ack unless disabled.
	// The Tap server expects JSON: {"type":"ack","id":<id>}
	// See: https://github.com/bluesky-social/indigo/blob/main/cmd/tap/types.go
	if !c.config.DisableAcks {
		ackMsg := fmt.Sprintf(`{"type":"ack","id":%d}`, event.ID)
		if err := c.writeText(conn, ackMsg); err != nil {
			return fmt.Errorf("failed to send ack for event %d: %w", event.ID, err)
		}
	}

	return nil
}

// writeText sends a text message on the WebSocket connection with a write deadline.
func (c *Consumer) writeText(conn *websocket.Conn, msg string) error {
	if err := conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}
	return conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

// isWriteError reports whether err originated from a failed ack write.
// These errors mean the connection is dead and we should reconnect immediately
// rather than continuing to read and generating a cascade of identical errors.
func isWriteError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "failed to send ack") ||
		strings.Contains(msg, "failed to set write deadline")
}

// incrementRecordStat increments the appropriate record stat counter.
func (c *Consumer) incrementRecordStat(action ActionType) {
	switch action {
	case ActionCreate:
		atomic.AddInt64(&c.recordsCreated, 1)
	case ActionUpdate:
		atomic.AddInt64(&c.recordsUpdated, 1)
	case ActionDelete:
		atomic.AddInt64(&c.recordsDeleted, 1)
	}
}

// Stop gracefully shuts down the consumer.
func (c *Consumer) Stop() {
	c.stopOnce.Do(func() {
		close(c.done)

		c.connMu.Lock()
		conn := c.conn
		c.conn = nil
		c.connMu.Unlock()

		if conn != nil {
			_ = conn.Close()
		}
	})
}

// Stats returns the current event counts.
func (c *Consumer) Stats() Stats {
	return Stats{
		EventsReceived: atomic.LoadInt64(&c.eventsReceived),
		RecordsCreated: atomic.LoadInt64(&c.recordsCreated),
		RecordsUpdated: atomic.LoadInt64(&c.recordsUpdated),
		RecordsDeleted: atomic.LoadInt64(&c.recordsDeleted),
		IdentityEvents: atomic.LoadInt64(&c.identityEvents),
		Errors:         atomic.LoadInt64(&c.errors),
	}
}
