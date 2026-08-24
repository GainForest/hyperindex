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

	// defaultSlowEventThreshold is how long an event may remain in one handler
	// before the consumer starts emitting blocked-processing diagnostics.
	defaultSlowEventThreshold = 5 * time.Second

	// defaultBlockedEventLogInterval controls follow-up diagnostics while the
	// same event remains in flight.
	defaultBlockedEventLogInterval = 30 * time.Second

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

	// SlowEventThreshold and BlockedEventLogInterval override the production
	// watchdog timings. Zero values use the defaults.
	SlowEventThreshold      time.Duration
	BlockedEventLogInterval time.Duration

	// DatabaseStats supplies a non-sensitive pool snapshot for diagnostics.
	DatabaseStats func() DatabasePoolStats
}

// EventHandler processes Tap events. Return nil to ack, error to nack.
type EventHandler interface {
	HandleRecord(ctx context.Context, event *RecordEvent) error
	HandleIdentity(ctx context.Context, event *IdentityEvent) error
}

// DatabasePoolStats contains the database/sql pool state useful when an event
// is waiting on a connection or database operation.
type DatabasePoolStats struct {
	MaxOpenConnections int
	OpenConnections    int
	InUse              int
	Idle               int
	WaitCount          int64
	WaitDuration       time.Duration
}

// InFlightEventStats describes the single Tap event currently being processed.
type InFlightEventStats struct {
	EventID    int64
	Type       EventType
	DID        string
	Collection string
	RKey       string
	Action     ActionType
	Phase      string
	StartedAt  time.Time
	Duration   time.Duration
}

// Stats tracks consumer statistics and processing diagnostics.
type Stats struct {
	EventsReceived      int64
	RecordsCreated      int64
	RecordsUpdated      int64
	RecordsDeleted      int64
	IdentityEvents      int64
	Errors              int64
	LastEventReceivedAt *time.Time
	LastAckAt           *time.Time
	InFlight            *InFlightEventStats
	DatabasePool        *DatabasePoolStats
}

// Consumer connects to Tap's WebSocket and dispatches events.
type Consumer struct {
	config  ConsumerConfig
	handler EventHandler

	// writeTextFn allows overriding WebSocket write behavior in tests.
	// When nil, writeText is used.
	writeTextFn func(conn *websocket.Conn, msg string) error

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

	diagnosticsMu       sync.RWMutex
	lastEventReceivedAt time.Time
	lastAckAt           time.Time
	inFlight            *eventTrace
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

		connected, immediateReconnect, err := c.runOnce(ctx)

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

		if immediateReconnect {
			slog.Warn("Tap ack write failed, reconnecting immediately", "error", err)
			slog.Info("Attempting to reconnect to Tap...")
			continue
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
// Returns (connected, immediateReconnect, error) where connected is true if the dial
// succeeded (even if the connection later dropped with an error), and
// immediateReconnect indicates a known-dead connection scenario (like ack write
// failure) that should bypass backoff.
func (c *Consumer) runOnce(ctx context.Context) (bool, bool, error) {
	channelURL := c.config.TapURL + tapChannelPath

	slog.Info("Connecting to Tap", "url", channelURL)

	var header http.Header
	if c.config.Password != "" {
		creds := base64.StdEncoding.EncodeToString([]byte("admin:" + c.config.Password))
		header = http.Header{"Authorization": []string{"Basic " + creds}}
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, channelURL, header)
	if err != nil {
		return false, false, fmt.Errorf("failed to connect to Tap: %w", err)
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
			return true, false, ctx.Err()
		case <-c.done:
			return true, false, nil
		default:
		}

		// Set read deadline.
		if err := conn.SetReadDeadline(time.Now().Add(defaultReadTimeout)); err != nil {
			return true, false, fmt.Errorf("failed to set read deadline: %w", err)
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return true, false, nil
			}
			return true, false, fmt.Errorf("read error: %w", err)
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
				return true, true, fmt.Errorf("failed during Tap ack write: %w", err)
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
func (c *Consumer) dispatch(ctx context.Context, conn *websocket.Conn, event *Event) (err error) {
	ctx, trace := c.beginEvent(ctx, event)
	defer func() { c.finishEvent(trace, err) }()

	var handlerErr error

	switch {
	case event.IsRecord():
		setEventPhase(ctx, "handler.record")
		handlerErr = c.handler.HandleRecord(ctx, event.Record)
		if handlerErr == nil {
			c.incrementRecordStat(event.Record.Action)
		}

	case event.IsIdentity():
		setEventPhase(ctx, "handler.identity")
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
		setEventPhase(ctx, "ack.write")
		ackMsg := fmt.Sprintf(`{"type":"ack","id":%d}`, event.ID)
		writeFn := c.writeText
		if c.writeTextFn != nil {
			writeFn = c.writeTextFn
		}
		if err := writeFn(conn, ackMsg); err != nil {
			return fmt.Errorf("failed to send ack for event %d: %w", event.ID, err)
		}
		c.markAcked(time.Now())
	}

	setEventPhase(ctx, "completed")
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

// Stats returns the current event counts and processing diagnostics.
func (c *Consumer) Stats() Stats {
	stats := Stats{
		EventsReceived: atomic.LoadInt64(&c.eventsReceived),
		RecordsCreated: atomic.LoadInt64(&c.recordsCreated),
		RecordsUpdated: atomic.LoadInt64(&c.recordsUpdated),
		RecordsDeleted: atomic.LoadInt64(&c.recordsDeleted),
		IdentityEvents: atomic.LoadInt64(&c.identityEvents),
		Errors:         atomic.LoadInt64(&c.errors),
	}

	c.diagnosticsMu.RLock()
	if !c.lastEventReceivedAt.IsZero() {
		lastReceived := c.lastEventReceivedAt
		stats.LastEventReceivedAt = &lastReceived
	}
	if !c.lastAckAt.IsZero() {
		lastAck := c.lastAckAt
		stats.LastAckAt = &lastAck
	}
	if c.inFlight != nil {
		stats.InFlight = c.inFlight.snapshot(time.Now())
	}
	c.diagnosticsMu.RUnlock()
	if c.config.DatabaseStats != nil {
		pool := c.config.DatabaseStats()
		stats.DatabasePool = &pool
	}
	return stats
}
