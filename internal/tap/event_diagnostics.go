package tap

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type eventTraceContextKey struct{}

type eventTrace struct {
	mu sync.RWMutex

	eventID    int64
	eventType  EventType
	did        string
	collection string
	rkey       string
	action     ActionType
	phase      string
	startedAt  time.Time

	done       chan struct{}
	watchDone  chan struct{}
	slowLogged atomic.Bool
}

func (c *Consumer) beginEvent(ctx context.Context, event *Event) (context.Context, *eventTrace) {
	trace := &eventTrace{
		eventID:   event.ID,
		eventType: event.Type,
		phase:     "dispatch",
		startedAt: time.Now(),
		done:      make(chan struct{}),
		watchDone: make(chan struct{}),
	}
	if event.Record != nil {
		trace.did = event.Record.DID
		trace.collection = event.Record.Collection
		trace.rkey = event.Record.RKey
		trace.action = event.Record.Action
	} else if event.Identity != nil {
		trace.did = event.Identity.DID
	}

	c.diagnosticsMu.Lock()
	c.lastEventReceivedAt = trace.startedAt
	c.inFlight = trace
	c.diagnosticsMu.Unlock()

	go c.watchEvent(trace)
	return context.WithValue(ctx, eventTraceContextKey{}, trace), trace
}

func setEventPhase(ctx context.Context, phase string) {
	trace, ok := ctx.Value(eventTraceContextKey{}).(*eventTrace)
	if !ok || trace == nil {
		return
	}
	trace.mu.Lock()
	trace.phase = phase
	trace.mu.Unlock()
}

func (c *Consumer) watchEvent(trace *eventTrace) {
	defer close(trace.watchDone)

	threshold := c.config.SlowEventThreshold
	if threshold <= 0 {
		threshold = defaultSlowEventThreshold
	}
	interval := c.config.BlockedEventLogInterval
	if interval <= 0 {
		interval = defaultBlockedEventLogInterval
	}

	timer := time.NewTimer(threshold)
	defer timer.Stop()
	select {
	case <-trace.done:
		return
	case <-timer.C:
		trace.slowLogged.Store(true)
		c.logBlockedEvent(trace)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-trace.done:
			return
		case <-ticker.C:
			c.logBlockedEvent(trace)
		}
	}
}

func (c *Consumer) logBlockedEvent(trace *eventTrace) {
	snapshot := trace.snapshot(time.Now())
	attrs := eventLogAttrs(snapshot)
	if c.config.DatabaseStats != nil {
		attrs = appendDatabaseLogAttrs(attrs, c.config.DatabaseStats())
	}
	slog.Warn("Tap event processing is blocked", attrs...)
}

func (c *Consumer) finishEvent(trace *eventTrace, err error) {
	close(trace.done)
	<-trace.watchDone

	c.diagnosticsMu.Lock()
	if c.inFlight == trace {
		c.inFlight = nil
	}
	c.diagnosticsMu.Unlock()

	if !trace.slowLogged.Load() {
		return
	}

	snapshot := trace.snapshot(time.Now())
	attrs := eventLogAttrs(snapshot)
	if err != nil {
		attrs = append(attrs, "outcome", "failed", "error", err)
	} else {
		attrs = append(attrs, "outcome", "completed")
	}
	if c.config.DatabaseStats != nil {
		attrs = appendDatabaseLogAttrs(attrs, c.config.DatabaseStats())
	}
	slog.Warn("Slow Tap event processing completed", attrs...)
}

func (c *Consumer) markAcked(at time.Time) {
	c.diagnosticsMu.Lock()
	c.lastAckAt = at
	c.diagnosticsMu.Unlock()
}

func (t *eventTrace) snapshot(now time.Time) *InFlightEventStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return &InFlightEventStats{
		EventID:    t.eventID,
		Type:       t.eventType,
		DID:        t.did,
		Collection: t.collection,
		RKey:       t.rkey,
		Action:     t.action,
		Phase:      t.phase,
		StartedAt:  t.startedAt,
		Duration:   now.Sub(t.startedAt),
	}
}

func eventLogAttrs(event *InFlightEventStats) []any {
	return []any{
		"event_id", event.EventID,
		"type", event.Type,
		"did", event.DID,
		"collection", event.Collection,
		"rkey", event.RKey,
		"action", event.Action,
		"phase", event.Phase,
		"started_at", event.StartedAt.UTC().Format(time.RFC3339Nano),
		"duration", event.Duration,
	}
}

func appendDatabaseLogAttrs(attrs []any, stats DatabasePoolStats) []any {
	return append(attrs,
		"database_max_open_connections", stats.MaxOpenConnections,
		"database_open_connections", stats.OpenConnections,
		"database_in_use", stats.InUse,
		"database_idle", stats.Idle,
		"database_wait_count", stats.WaitCount,
		"database_wait_duration", stats.WaitDuration,
	)
}
