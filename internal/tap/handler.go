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
	records  *repositories.RecordsRepository
	actors   *repositories.ActorsRepository
	activity *repositories.IndexingActivityRepository // records indexing activity
	pubsub   *subscription.PubSub

	// Optional version history for opted-in collections (RECORD_HISTORY_COLLECTIONS).
	versions           *repositories.RecordVersionsRepository
	historyCollections repositories.CollectionMatcher
}

// WithRecordHistory gives the handler the version store. New versions are
// recorded for the collections the matcher selects (an empty matcher records
// none); deletes always purge a record's stored history, even for
// collections no longer tracked.
func (h *IndexHandler) WithRecordHistory(versions *repositories.RecordVersionsRepository, collections repositories.CollectionMatcher) *IndexHandler {
	h.versions = versions
	h.historyCollections = collections
	return h
}

func (h *IndexHandler) keepsHistory(collection string) bool {
	return h.versions != nil && h.historyCollections.Matches(collection)
}

// NewIndexHandler creates a new IndexHandler.
func NewIndexHandler(
	records *repositories.RecordsRepository,
	actors *repositories.ActorsRepository,
	activity *repositories.IndexingActivityRepository,
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

		// Ensure the actor exists without erasing identity metadata populated by
		// Tap identity events.
		setEventPhase(ctx, "actors.ensure")
		if err := h.actors.Ensure(ctx, event.DID); err != nil {
			slog.Debug("Failed to upsert actor", "did", event.DID, "error", err)
		}

		// Record the version before the current-state write: if either write
		// fails, Tap redelivers and the version insert is an idempotent no-op.
		if h.keepsHistory(event.Collection) {
			action := repositories.RecordVersionCreate
			if event.Action == ActionUpdate {
				action = repositories.RecordVersionUpdate
			}
			body := string(event.Record)
			live := event.Live
			setEventPhase(ctx, "record_versions.append")
			if err := h.versions.Append(ctx, repositories.RecordVersionWrite{
				URI:        uri,
				CID:        event.CID,
				DID:        event.DID,
				Collection: event.Collection,
				Action:     action,
				JSON:       &body,
				Live:       &live,
			}); err != nil {
				return fmt.Errorf("failed to record version history: %w", err)
			}
		}

		// Store record
		setEventPhase(ctx, "records.insert")
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
			setEventPhase(ctx, "activity.log")
			activityID, err := h.activity.LogActivity(ctx, time.Now(), string(event.Action), event.Collection, event.DID, event.RKey, string(event.Record))
			if err != nil {
				slog.Debug("Failed to log activity", "error", err)
			} else {
				setEventPhase(ctx, "activity.update_status")
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
			setEventPhase(ctx, "pubsub.publish")
			h.pubsub.PublishRecord(eventType, uri, event.CID, event.DID, event.Collection, event.Record)
		}

	case ActionDelete:
		setEventPhase(ctx, "records.delete")
		if err := h.records.Delete(ctx, uri); err != nil {
			return fmt.Errorf("failed to delete record: %w", err)
		}
		// A deleted record takes its history with it. Runs on every delivery,
		// so a failed purge is retried when Tap redelivers the event.
		if h.versions != nil {
			setEventPhase(ctx, "record_versions.delete")
			if err := h.versions.DeleteByURI(ctx, uri); err != nil {
				return fmt.Errorf("failed to delete record version history: %w", err)
			}
		}
		if h.pubsub != nil {
			setEventPhase(ctx, "pubsub.publish")
			h.pubsub.PublishRecord(subscription.EventDelete, uri, "", event.DID, event.Collection, nil)
		}
		if h.activity != nil {
			setEventPhase(ctx, "activity.log")
			activityID, err := h.activity.LogActivity(ctx, time.Now(), "delete", event.Collection, event.DID, event.RKey, "")
			if err != nil {
				slog.Debug("Failed to log delete activity", "error", err)
			} else {
				setEventPhase(ctx, "activity.update_status")
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
		setEventPhase(ctx, "records.purge_actor_data")
		if err := h.records.PurgeActorData(ctx, event.DID); err != nil {
			return fmt.Errorf("failed to purge actor data: %w", err)
		}

		slog.Info("Purged identity from index",
			"did", event.DID,
			"is_active", event.IsActive,
			"status", event.Status,
		)
		return nil
	}

	setEventPhase(ctx, "actors.upsert_identity")
	return h.actors.UpsertIdentity(ctx, event.DID, event.Handle)
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
