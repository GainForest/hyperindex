package tap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/graphql/subscription"
)

// IndexHandler implements EventHandler and stores events in the database.
type IndexHandler struct {
	records  *repositories.RecordsRepository
	actors   *repositories.ActorsRepository
	activity *repositories.JetstreamActivityRepository // reuse existing activity repo
	pubsub   *subscription.PubSub
}

// NewIndexHandler creates a new IndexHandler.
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
	if !event.IsActive {
		return true
	}

	status := strings.ToLower(strings.TrimSpace(event.Status))
	switch status {
	case "deleted", "deactivated", "suspended", "takendown":
		return true
	default:
		return false
	}
}
