package jetstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/subscription"
)

// ConsumerConfig configures the Jetstream consumer.
type ConsumerConfig struct {
	// JetstreamURL is the Jetstream WebSocket endpoint.
	JetstreamURL string

	// Collections to subscribe to.
	Collections []string

	// DisableCursor disables cursor tracking.
	DisableCursor bool

	// CursorFlushInterval is how often to flush the cursor to database.
	CursorFlushInterval time.Duration
}

// Consumer consumes events from Jetstream and stores them in the database.
type Consumer struct {
	config       ConsumerConfig
	client       *Client
	recordsRepo  *repositories.RecordsRepository
	actorsRepo   *repositories.ActorsRepository
	configRepo   *repositories.ConfigRepository
	activityRepo *repositories.IndexingActivityRepository

	// Pub/sub for GraphQL subscriptions
	pubsub *subscription.PubSub

	// Cursor tracking
	cursor     int64
	cursorMu   sync.Mutex
	cursorDone chan struct{}

	// Stats
	stats      Stats
	statsMu    sync.RWMutex
	statsStart time.Time

	// For reconnection
	ctx       context.Context
	ctxCancel context.CancelFunc
	clientMu  sync.Mutex
	running   bool
}

// Stats tracks consumer statistics.
type Stats struct {
	EventsReceived int64
	RecordsCreated int64
	RecordsUpdated int64
	RecordsDeleted int64
	Errors         int64
}

// NewConsumer creates a new Jetstream consumer.
func NewConsumer(
	config ConsumerConfig,
	recordsRepo *repositories.RecordsRepository,
	actorsRepo *repositories.ActorsRepository,
	configRepo *repositories.ConfigRepository,
	activityRepo *repositories.IndexingActivityRepository,
	pubsub *subscription.PubSub,
) *Consumer {
	if config.CursorFlushInterval == 0 {
		config.CursorFlushInterval = 5 * time.Second
	}

	return &Consumer{
		config:       config,
		recordsRepo:  recordsRepo,
		actorsRepo:   actorsRepo,
		configRepo:   configRepo,
		activityRepo: activityRepo,
		pubsub:       pubsub,
		cursorDone:   make(chan struct{}),
		statsStart:   time.Now(),
	}
}

// Start begins consuming events from Jetstream with automatic reconnection.
func (c *Consumer) Start(ctx context.Context) error {
	c.clientMu.Lock()
	c.ctx, c.ctxCancel = context.WithCancel(ctx)
	c.running = true
	c.clientMu.Unlock()

	// Reconnection loop with exponential backoff
	backoff := time.Second
	maxBackoff := 2 * time.Minute

	for {
		err := c.startInternal(c.ctx)

		// Check if we should stop (context cancelled)
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		// If error is nil, connection closed gracefully (shouldn't happen in normal operation)
		// but we still try to reconnect
		if err != nil {
			slog.Error("Jetstream connection lost, will reconnect",
				"error", err,
				"backoff", backoff,
			)
		} else {
			slog.Warn("Jetstream connection closed unexpectedly, will reconnect",
				"backoff", backoff,
			)
		}

		// Wait before reconnecting
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(backoff):
		}

		// Exponential backoff with cap
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		// Reset cursorDone channel for new connection
		c.cursorDone = make(chan struct{})

		slog.Info("Attempting to reconnect to Jetstream...")
	}
}

// startInternal does the actual connection and event processing.
func (c *Consumer) startInternal(ctx context.Context) error {
	// Clean up old client if exists (for reconnection scenarios)
	c.clientMu.Lock()
	if c.client != nil {
		c.client.Stop()
		c.client = nil
	}
	c.clientMu.Unlock()

	// Load cursor from database (fresh load on each connection attempt)
	cursor, err := c.loadCursor(ctx)
	if err != nil {
		slog.Warn("Failed to load cursor, starting from live", "error", err)
	} else if cursor > 0 {
		slog.Info("Resuming from cursor", "cursor", cursor)
	}

	// Create client
	c.clientMu.Lock()
	c.client = NewClient(ClientConfig{
		URL:           c.config.JetstreamURL,
		Collections:   c.config.Collections,
		Cursor:        cursor,
		DisableCursor: c.config.DisableCursor,
	})
	c.clientMu.Unlock()

	// Connect
	if err := c.client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Start cursor flusher
	if !c.config.DisableCursor {
		go c.cursorFlusher(ctx)
	}

	// Start event processor
	go c.processEvents(ctx)

	// Run client (blocking)
	return c.client.Run(ctx)
}

// Stop stops the consumer.
func (c *Consumer) Stop() {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()

	c.running = false
	if c.ctxCancel != nil {
		c.ctxCancel()
	}
	select {
	case <-c.cursorDone:
		// Already closed
	default:
		close(c.cursorDone)
	}
	if c.client != nil {
		c.client.Stop()
	}
}

// UpdateCollections updates the subscribed collections and reconnects.
func (c *Consumer) UpdateCollections(collections []string) error {
	c.clientMu.Lock()
	wasRunning := c.running
	oldClient := c.client
	c.clientMu.Unlock()

	if !wasRunning {
		// Just update config, will be used on next Start
		c.config.Collections = collections
		slog.Info("Updated Jetstream collections (not running)", "collections", collections)
		return nil
	}

	slog.Info("Updating Jetstream collections, reconnecting...", "collections", collections)

	// Stop old client
	if oldClient != nil {
		oldClient.Stop()
	}

	// Update config
	c.config.Collections = collections

	// Reset cursor done channel for new connection
	c.cursorDone = make(chan struct{})

	// Create new context
	c.clientMu.Lock()
	if c.ctxCancel != nil {
		c.ctxCancel()
	}
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	ctx := c.ctx
	c.clientMu.Unlock()

	// Start in background
	go func() {
		if err := c.startInternal(ctx); err != nil {
			slog.Error("Failed to reconnect Jetstream", "error", err)
		}
	}()

	return nil
}

// Collections returns the currently subscribed collections.
func (c *Consumer) Collections() []string {
	return c.config.Collections
}

// Stats returns the current statistics.
func (c *Consumer) Stats() Stats {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()
	return c.stats
}

// processEvents handles incoming events.
func (c *Consumer) processEvents(ctx context.Context) {
	for event := range c.client.Events() {
		c.statsMu.Lock()
		c.stats.EventsReceived++
		eventCount := c.stats.EventsReceived
		c.statsMu.Unlock()

		// Log every event received (for debugging)
		if event.IsCommit() {
			slog.Info("[jetstream] Event received",
				"collection", event.Commit.Collection,
				"operation", event.Commit.Operation,
				"did", event.DID,
				"total_events", eventCount,
			)
		}

		// Process commit events
		if event.IsCommit() {
			if err := c.handleCommit(ctx, event); err != nil {
				slog.Warn("Failed to handle commit",
					"error", err,
					"did", event.DID,
					"collection", event.Commit.Collection,
				)
				c.statsMu.Lock()
				c.stats.Errors++
				c.statsMu.Unlock()
				continue // Don't advance cursor on failure
			}
		}

		// Update cursor only after successful processing
		c.cursorMu.Lock()
		c.cursor = event.TimeUS
		c.cursorMu.Unlock()
	}
}

// handleCommit processes a commit event.
func (c *Consumer) handleCommit(ctx context.Context, event *Event) error {
	commit := event.Commit
	uri := commit.URI(event.DID)

	// Use current time for activity logging (when we processed the event)
	// The event's original timestamp is stored in the event_json
	processedAt := time.Now()

	// Log activity if repository is available
	var activityID int64
	if c.activityRepo != nil {
		var err error
		activityID, err = c.activityRepo.LogActivity(
			ctx,
			processedAt,
			string(commit.Operation),
			commit.Collection,
			event.DID,
			commit.RKey,
			string(commit.Record),
		)
		if err != nil {
			slog.Warn("Failed to log activity", "error", err)
		}
	}

	// Helper to update activity status
	updateActivityStatus := func(status string, errMsg *string) {
		if c.activityRepo != nil && activityID > 0 {
			if err := c.activityRepo.UpdateStatus(ctx, activityID, status, errMsg); err != nil {
				slog.Warn("Failed to update activity status", "error", err)
			}
		}
	}

	switch commit.Operation {
	case OpCreate, OpUpdate:
		// Ensure actor exists (just store the DID, no resolution)
		if err := c.ensureActor(ctx, event.DID); err != nil {
			slog.Warn("Failed to ensure actor", "did", event.DID, "error", err)
			// Continue anyway - record storage is more important
		}

		// Store the record
		result, err := c.recordsRepo.Insert(ctx, uri, commit.CID, event.DID, commit.Collection, string(commit.Record))
		if err != nil {
			errMsg := err.Error()
			updateActivityStatus("error", &errMsg)
			return fmt.Errorf("failed to insert record: %w", err)
		}

		c.statsMu.Lock()
		if result == repositories.Inserted {
			if commit.Operation == OpCreate {
				c.stats.RecordsCreated++
			} else {
				c.stats.RecordsUpdated++
			}
		}
		c.statsMu.Unlock()

		// Publish to GraphQL subscriptions
		eventType := subscription.EventCreate
		if commit.Operation == OpUpdate {
			eventType = subscription.EventUpdate
		}
		c.pubsub.PublishRecord(eventType, uri, commit.CID, event.DID, commit.Collection, commit.Record)

		updateActivityStatus("success", nil)

		slog.Info("[jetstream] Stored record",
			"uri", uri,
			"collection", commit.Collection,
			"operation", commit.Operation,
		)

	case OpDelete:
		if err := c.recordsRepo.Delete(ctx, uri); err != nil {
			errMsg := err.Error()
			updateActivityStatus("error", &errMsg)
			return fmt.Errorf("failed to delete record: %w", err)
		}

		c.statsMu.Lock()
		c.stats.RecordsDeleted++
		c.statsMu.Unlock()

		// Publish delete to GraphQL subscriptions
		c.pubsub.PublishRecord(subscription.EventDelete, uri, commit.CID, event.DID, commit.Collection, nil)

		updateActivityStatus("success", nil)

		slog.Debug("Deleted record", "uri", uri)
	}

	return nil
}

// ensureActor ensures the actor exists in the database.
func (c *Consumer) ensureActor(ctx context.Context, did string) error {
	// Check if actor exists
	exists, err := c.actorsRepo.Exists(ctx, did)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Upsert actor (without handle resolution per user request)
	return c.actorsRepo.Upsert(ctx, did, "") // Empty handle
}

// cursorFlusher periodically flushes the cursor to the database.
func (c *Consumer) cursorFlusher(ctx context.Context) {
	ticker := time.NewTicker(c.config.CursorFlushInterval)
	defer ticker.Stop()

	var lastFlushed int64

	for {
		select {
		case <-ctx.Done():
			// Final flush
			c.flushCursor(context.Background())
			return
		case <-c.cursorDone:
			// Final flush
			c.flushCursor(context.Background())
			return
		case <-ticker.C:
			c.cursorMu.Lock()
			cursor := c.cursor
			c.cursorMu.Unlock()

			if cursor > lastFlushed {
				if err := c.saveCursor(ctx, cursor); err != nil {
					slog.Warn("Failed to save cursor", "error", err)
				} else {
					lastFlushed = cursor
				}
			}
		}
	}
}

// flushCursor flushes the current cursor immediately.
func (c *Consumer) flushCursor(ctx context.Context) {
	c.cursorMu.Lock()
	cursor := c.cursor
	c.cursorMu.Unlock()

	if cursor > 0 {
		if err := c.saveCursor(ctx, cursor); err != nil {
			slog.Warn("Failed to flush cursor", "error", err)
		}
	}
}

// loadCursor loads the cursor from the config table.
func (c *Consumer) loadCursor(ctx context.Context) (int64, error) {
	value, err := c.configRepo.Get(ctx, "jetstream_cursor")
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, nil
	}

	var cursor int64
	if err := json.Unmarshal([]byte(value), &cursor); err != nil {
		return 0, err
	}
	return cursor, nil
}

// saveCursor saves the cursor to the config table.
func (c *Consumer) saveCursor(ctx context.Context, cursor int64) error {
	value, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	return c.configRepo.Set(ctx, "jetstream_cursor", string(value))
}
