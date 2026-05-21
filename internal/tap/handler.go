package tap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/subscription"
)

// IndexHandler implements EventHandler and stores events in the database.
type IndexHandler struct {
	records      *repositories.RecordsRepository
	actors       *repositories.ActorsRepository
	audit        *repositories.AuditRepository
	auditEnabled bool
	activity     *repositories.JetstreamActivityRepository // reuse existing activity repo
	pubsub       *subscription.PubSub
}

// NewIndexHandler creates an IndexHandler that applies Tap events directly to
// the current-state repositories without storing append-only audit history.
func NewIndexHandler(
	records *repositories.RecordsRepository,
	actors *repositories.ActorsRepository,
	activity *repositories.JetstreamActivityRepository,
	pubsub *subscription.PubSub,
) *IndexHandler {
	return &IndexHandler{
		records:  records,
		actors:   actors,
		activity: activity,
		pubsub:   pubsub,
	}
}

// NewAuditIndexHandler creates an IndexHandler that stores Tap deliveries through
// AuditRepository before publishing subscriptions. In this mode, HandleEvent does
// not call RecordsRepository or ActorsRepository directly; the audit repository
// owns the transaction that writes audit rows and current-state projections.
func NewAuditIndexHandler(
	records *repositories.RecordsRepository,
	actors *repositories.ActorsRepository,
	audit *repositories.AuditRepository,
	activity *repositories.JetstreamActivityRepository,
	pubsub *subscription.PubSub,
) *IndexHandler {
	return &IndexHandler{
		records:      records,
		actors:       actors,
		audit:        audit,
		auditEnabled: true,
		activity:     activity,
		pubsub:       pubsub,
	}
}

// HandleEvent processes a parsed Tap delivery. In audit mode it writes through
// AuditRepository first and publishes record subscriptions only after the audit
// transaction commits. Otherwise it preserves the existing current-state path.
func (h *IndexHandler) HandleEvent(ctx context.Context, rawPayload []byte, event *Event) error {
	if event == nil {
		return fmt.Errorf("tap index handler requires a parsed event; got nil")
	}
	if h.auditEnabled {
		return h.handleAuditEvent(ctx, rawPayload, event)
	}

	switch {
	case event.IsRecord():
		return h.HandleRecord(ctx, event.Record)
	case event.IsIdentity():
		return h.HandleIdentity(ctx, event.Identity)
	default:
		return nil
	}
}

func (h *IndexHandler) handleAuditEvent(ctx context.Context, rawPayload []byte, event *Event) error {
	if h.audit == nil {
		return fmt.Errorf("audit Tap handler requires an AuditRepository; got nil")
	}

	result, err := h.audit.IngestTapEvent(ctx, rawPayload, auditTapEventFromEvent(event))
	if err != nil {
		return err
	}
	if event.IsRecord() && result.Inserted {
		h.publishAuditRecordSubscription(event.Record)
	}
	return nil
}

func auditTapEventFromEvent(event *Event) *repositories.AuditTapEvent {
	auditEvent := &repositories.AuditTapEvent{
		ID:   event.ID,
		Type: string(event.Type),
	}
	if event.IsRecord() {
		record := event.Record
		auditEvent.Record = &repositories.AuditTapRecordEvent{
			Live:       record.Live,
			Rev:        record.Rev,
			DID:        record.DID,
			Collection: record.Collection,
			RKey:       record.RKey,
			Action:     string(record.Action),
			CID:        record.CID,
			Record:     append([]byte(nil), record.Record...),
		}
	}
	if event.IsIdentity() {
		identity := event.Identity
		auditEvent.Identity = &repositories.AuditTapIdentityEvent{
			DID:             identity.DID,
			Handle:          identity.Handle,
			IsActive:        identity.IsActive,
			IsActivePresent: identity.IsActivePresent,
			Status:          identity.Status,
		}
	}
	return auditEvent
}

func (h *IndexHandler) publishAuditRecordSubscription(event *RecordEvent) {
	if h.pubsub == nil || event == nil {
		return
	}

	uri := event.URI()
	switch event.Action {
	case ActionCreate:
		if len(event.Record) == 0 {
			return
		}
		h.pubsub.PublishRecord(subscription.EventCreate, uri, event.CID, event.DID, event.Collection, event.Record)
	case ActionUpdate:
		if len(event.Record) == 0 {
			return
		}
		h.pubsub.PublishRecord(subscription.EventUpdate, uri, event.CID, event.DID, event.Collection, event.Record)
	case ActionDelete:
		h.pubsub.PublishRecord(subscription.EventDelete, uri, "", event.DID, event.Collection, nil)
	}
}

// HandleRecord processes a record event by storing or deleting the record and
// publishing to GraphQL subscriptions.
func (h *IndexHandler) HandleRecord(ctx context.Context, event *RecordEvent) error {
	uri := event.URI()

	switch event.Action {
	case ActionCreate, ActionUpdate:
		// Events may arrive without a record body (e.g. during Tap backfill when
		// the PDS record could not be fetched). Ack and skip — nothing to store.
		if len(event.Record) == 0 {
			slog.Debug("Skipping create/update event with no record body", "uri", uri)
			return nil
		}

		// Ensure actor exists (empty handle; identity events update it)
		if err := h.actors.Upsert(ctx, event.DID, ""); err != nil {
			slog.Debug("Failed to upsert actor", "did", event.DID, "error", err)
		}

		// Store record
		result, err := h.records.Insert(ctx, uri, event.CID, event.DID, event.Collection, string(event.Record))
		if err != nil {
			return fmt.Errorf("failed to insert record: %w", err)
		}
		if result == repositories.Skipped {
			slog.Debug("Record insert skipped (unchanged CID)", "uri", uri, "cid", event.CID)
			return nil
		}

		// Log activity (if activity repo available)
		if h.activity != nil {
			activityID, err := h.activity.LogActivity(ctx, time.Now(), string(event.Action), event.Collection, event.DID, event.RKey, string(event.Record))
			if err != nil {
				slog.Debug("Failed to log activity", "error", err)
			} else {
				if err := h.activity.UpdateStatus(ctx, activityID, "completed", nil); err != nil {
					slog.Debug("Failed to update activity status", "error", err)
				}
			}
		}

		// Publish to GraphQL subscriptions
		eventType := subscription.EventCreate
		if event.Action == ActionUpdate {
			eventType = subscription.EventUpdate
		}
		if h.pubsub != nil {
			h.pubsub.PublishRecord(eventType, uri, event.CID, event.DID, event.Collection, event.Record)
		}

	case ActionDelete:
		if err := h.records.Delete(ctx, uri); err != nil {
			return fmt.Errorf("failed to delete record: %w", err)
		}
		if h.pubsub != nil {
			h.pubsub.PublishRecord(subscription.EventDelete, uri, "", event.DID, event.Collection, nil)
		}
		if h.activity != nil {
			activityID, err := h.activity.LogActivity(ctx, time.Now(), "delete", event.Collection, event.DID, event.RKey, "")
			if err != nil {
				slog.Debug("Failed to log delete activity", "error", err)
			} else {
				if err := h.activity.UpdateStatus(ctx, activityID, "completed", nil); err != nil {
					slog.Debug("Failed to update activity status", "error", err)
				}
			}
		}
	}

	slog.Debug("Handled record event", "action", event.Action, "uri", uri)
	return nil
}

// HandleIdentity processes an identity event by updating the actor's handle.
func (h *IndexHandler) HandleIdentity(ctx context.Context, event *IdentityEvent) error {
	if identityStatus(event) == "active" && event.IsActivePresent && !event.IsActive {
		slog.Warn("Keeping active identity despite false is_active flag", "did", event.DID)
	}

	if shouldPurgeIdentity(event) {
		if err := h.records.DeleteByDID(ctx, event.DID); err != nil {
			return fmt.Errorf("failed to delete records by did: %w", err)
		}
		if err := h.actors.DeleteByDID(ctx, event.DID); err != nil {
			return fmt.Errorf("failed to delete actor by did: %w", err)
		}

		slog.Info("Purged identity from index",
			"did", event.DID,
			"is_active", event.IsActive,
			"status", event.Status,
		)
		return nil
	}

	return h.actors.Upsert(ctx, event.DID, event.Handle)
}

func shouldPurgeIdentity(event *IdentityEvent) bool {
	switch identityStatus(event) {
	case "deleted", "deactivated", "suspended", "takendown":
		return true
	default:
		return false
	}
}

func identityStatus(event *IdentityEvent) string {
	return strings.ToLower(strings.TrimSpace(event.Status))
}
