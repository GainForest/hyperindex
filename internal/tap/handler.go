package tap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/subscription"
	"github.com/GainForest/hyperindex/internal/validation"
)

// IndexHandler implements EventHandler and stores events in the database.
type IndexHandler struct {
	records   *repositories.RecordsRepository
	actors    *repositories.ActorsRepository
	activity  *repositories.IndexingActivityRepository // records indexing activity
	pubsub    *subscription.PubSub
	validator validation.RecordValidator

	// Optional version history for opted-in collections (RECORD_HISTORY_COLLECTIONS).
	versions           *repositories.RecordVersionsRepository
	historyCollections repositories.CollectionMatcher
}

// WithRecordHistory enables append-only version history for the collections
// the matcher selects. Every distinct version (CID) and delete is recorded.
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
	validators ...validation.RecordValidator,
) *IndexHandler {
	var validator validation.RecordValidator
	if len(validators) > 0 {
		validator = validators[0]
	}
	return &IndexHandler{
		records:   records,
		actors:    actors,
		activity:  activity,
		pubsub:    pubsub,
		validator: validator,
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
		if err := h.actors.Ensure(ctx, event.DID); err != nil {
			slog.Debug("Failed to upsert actor", "did", event.DID, "error", err)
		}

		// Record the version before the current-state upsert: if either write
		// fails, Tap redelivers and the version insert is an idempotent no-op.
		if h.keepsHistory(event.Collection) {
			action := repositories.RecordVersionCreate
			if event.Action == ActionUpdate {
				action = repositories.RecordVersionUpdate
			}
			body := string(event.Record)
			live := event.Live
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

		validationResult := validation.ClassifyRecord(h.validator, event.Collection, event.RKey, event.Record)
		writeResult, err := h.records.UpsertWithValidation(ctx, repositories.RecordWrite{
			URI:              uri,
			CID:              event.CID,
			DID:              event.DID,
			Collection:       event.Collection,
			RKey:             event.RKey,
			JSON:             string(event.Record),
			ValidationStatus: validationResult.Status,
			ValidationError:  validationResult.Error,
			LexiconHash:      validationResult.LexiconHash,
		})
		if err != nil {
			return fmt.Errorf("failed to store record with validation metadata: %w", err)
		}
		if writeResult == repositories.Skipped {
			slog.Debug("Record content unchanged; validation metadata is current", "uri", uri, "cid", event.CID)
		} else if h.activity != nil {
			activityID, err := h.activity.LogActivity(ctx, time.Now(), string(event.Action), event.Collection, event.DID, event.RKey, string(event.Record))
			if err != nil {
				slog.Debug("Failed to log activity", "error", err)
			} else if err := h.activity.UpdateStatus(ctx, activityID, "completed", nil); err != nil {
				slog.Debug("Failed to update activity status", "error", err)
			}
		}

		// Publish every stored raw event. Typed resolvers apply validation gating.
		eventType := subscription.EventCreate
		if event.Action == ActionUpdate {
			eventType = subscription.EventUpdate
		}
		if h.pubsub != nil {
			h.pubsub.PublishRecordWithValidation(eventType, uri, event.CID, event.DID, event.Collection, event.Record, validationResult.Status == validation.StatusValid)
		}

	case ActionDelete:
		// Tombstone the version being deleted before removing it. If either
		// write fails the event is retried: the tombstone insert is idempotent
		// (keyed by the deleted CID) and the record is still there to delete.
		if h.keepsHistory(event.Collection) {
			current, err := h.records.GetByURI(ctx, uri)
			switch {
			case err == nil:
				live := event.Live
				if err := h.versions.Append(ctx, repositories.RecordVersionWrite{
					URI:        uri,
					CID:        current.CID,
					DID:        event.DID,
					Collection: event.Collection,
					Action:     repositories.RecordVersionDelete,
					Live:       &live,
				}); err != nil {
					return fmt.Errorf("failed to record delete in version history: %w", err)
				}
			case errors.Is(err, sql.ErrNoRows):
				// Nothing indexed to delete (or already deleted and tombstoned).
			default:
				return fmt.Errorf("failed to read record before delete: %w", err)
			}
		}

		deleted, err := h.records.DeleteReturning(ctx, uri)
		if err != nil {
			return fmt.Errorf("failed to delete record: %w", err)
		}
		if h.pubsub != nil {
			previousCID := ""
			deleteDID := event.DID
			deleteCollection := event.Collection
			var previousJSON []byte
			wasValid := false
			if deleted != nil {
				previousCID = deleted.CID
				deleteDID = deleted.DID
				deleteCollection = deleted.Collection
				previousJSON = []byte(deleted.JSON)
				wasValid = deleted.ValidationStatus == validation.StatusValid
			}
			h.pubsub.PublishDelete(uri, previousCID, deleteDID, deleteCollection, previousJSON, wasValid)
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
