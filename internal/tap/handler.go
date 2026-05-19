package tap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/subscription"
)

var (
	tapHandlerTimingLogAll        = boolEnv("HYPERINDEX_TAP_HANDLER_TIMING_LOG_ALL", false)
	tapHandlerTimingSlowThreshold = durationEnv("HYPERINDEX_TAP_HANDLER_TIMING_SLOW_THRESHOLD", 2*time.Second)
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

type recordHandlerTiming struct {
	actor          time.Duration
	record         time.Duration
	activityLog    time.Duration
	activityStatus time.Duration
	pubsub         time.Duration
}

type identityHandlerTiming struct {
	recordsDelete time.Duration
	actorDelete   time.Duration
	actorUpsert   time.Duration
}

// HandleRecord processes a record event by storing or deleting the record and
// publishing to GraphQL subscriptions.
func (h *IndexHandler) HandleRecord(ctx context.Context, event *RecordEvent) (err error) {
	started := time.Now()
	uri := event.URI()
	timing := recordHandlerTiming{}
	result := "ignored_unknown_action"
	actorErr := ""
	activityLogErr := ""
	activityStatusErr := ""

	defer func() {
		logTapRecordHandlerTiming(ctx, event, uri, result, len(event.Record), timing, time.Since(started), actorErr, activityLogErr, activityStatusErr, err)
	}()

	switch event.Action {
	case ActionCreate, ActionUpdate:
		// Events may arrive without a record body (e.g. during Tap backfill when
		// the PDS record could not be fetched). Ack and skip — nothing to store.
		if len(event.Record) == 0 {
			result = "skipped_empty_record"
			slog.Debug("Skipping create/update event with no record body", "uri", uri)
			return nil
		}

		// Ensure actor exists (empty handle; identity events update it)
		stageStarted := time.Now()
		if upsertErr := h.actors.Upsert(ctx, event.DID, ""); upsertErr != nil {
			actorErr = upsertErr.Error()
			slog.Debug("Failed to upsert actor", "did", event.DID, "error", upsertErr)
		}
		timing.actor = time.Since(stageStarted)

		// Store record
		stageStarted = time.Now()
		insertResult, insertErr := h.records.Insert(ctx, uri, event.CID, event.DID, event.Collection, string(event.Record))
		timing.record = time.Since(stageStarted)
		if insertErr != nil {
			return fmt.Errorf("failed to insert record: %w", insertErr)
		}
		if insertResult == repositories.Skipped {
			result = "skipped_unchanged_cid"
			slog.Debug("Record insert skipped (unchanged CID)", "uri", uri, "cid", event.CID)
			return nil
		}
		result = "stored"

		// Log activity (if activity repo available)
		if h.activity != nil {
			stageStarted = time.Now()
			activityID, logErr := h.activity.LogActivity(ctx, time.Now(), string(event.Action), event.Collection, event.DID, event.RKey, string(event.Record))
			timing.activityLog = time.Since(stageStarted)
			if logErr != nil {
				activityLogErr = logErr.Error()
				slog.Debug("Failed to log activity", "error", logErr)
			} else {
				stageStarted = time.Now()
				if statusErr := h.activity.UpdateStatus(ctx, activityID, "completed", nil); statusErr != nil {
					activityStatusErr = statusErr.Error()
					slog.Debug("Failed to update activity status", "error", statusErr)
				}
				timing.activityStatus = time.Since(stageStarted)
			}
		}

		// Publish to GraphQL subscriptions
		eventType := subscription.EventCreate
		if event.Action == ActionUpdate {
			eventType = subscription.EventUpdate
		}
		if h.pubsub != nil {
			stageStarted = time.Now()
			h.pubsub.PublishRecord(eventType, uri, event.CID, event.DID, event.Collection, event.Record)
			timing.pubsub = time.Since(stageStarted)
		}

	case ActionDelete:
		stageStarted := time.Now()
		if deleteErr := h.records.Delete(ctx, uri); deleteErr != nil {
			timing.record = time.Since(stageStarted)
			return fmt.Errorf("failed to delete record: %w", deleteErr)
		}
		timing.record = time.Since(stageStarted)
		result = "deleted"

		if h.pubsub != nil {
			stageStarted = time.Now()
			h.pubsub.PublishRecord(subscription.EventDelete, uri, "", event.DID, event.Collection, nil)
			timing.pubsub = time.Since(stageStarted)
		}
		if h.activity != nil {
			stageStarted = time.Now()
			activityID, logErr := h.activity.LogActivity(ctx, time.Now(), "delete", event.Collection, event.DID, event.RKey, "")
			timing.activityLog = time.Since(stageStarted)
			if logErr != nil {
				activityLogErr = logErr.Error()
				slog.Debug("Failed to log delete activity", "error", logErr)
			} else {
				stageStarted = time.Now()
				if statusErr := h.activity.UpdateStatus(ctx, activityID, "completed", nil); statusErr != nil {
					activityStatusErr = statusErr.Error()
					slog.Debug("Failed to update activity status", "error", statusErr)
				}
				timing.activityStatus = time.Since(stageStarted)
			}
		}
	}

	slog.Debug("Handled record event", "action", event.Action, "uri", uri)
	return nil
}

// HandleIdentity processes an identity event by updating the actor's handle.
func (h *IndexHandler) HandleIdentity(ctx context.Context, event *IdentityEvent) (err error) {
	started := time.Now()
	timing := identityHandlerTiming{}
	result := "upserted"

	defer func() {
		logTapIdentityHandlerTiming(ctx, event, result, timing, time.Since(started), err)
	}()

	if identityStatus(event) == "active" && event.IsActivePresent && !event.IsActive {
		slog.Warn("Keeping active identity despite false is_active flag", "did", event.DID)
	}

	if shouldPurgeIdentity(event) {
		stageStarted := time.Now()
		if deleteErr := h.records.DeleteByDID(ctx, event.DID); deleteErr != nil {
			timing.recordsDelete = time.Since(stageStarted)
			return fmt.Errorf("failed to delete records by did: %w", deleteErr)
		}
		timing.recordsDelete = time.Since(stageStarted)

		stageStarted = time.Now()
		if deleteErr := h.actors.DeleteByDID(ctx, event.DID); deleteErr != nil {
			timing.actorDelete = time.Since(stageStarted)
			return fmt.Errorf("failed to delete actor by did: %w", deleteErr)
		}
		timing.actorDelete = time.Since(stageStarted)
		result = "purged"

		slog.Info("Purged identity from index",
			"did", event.DID,
			"is_active", event.IsActive,
			"status", event.Status,
		)
		return nil
	}

	stageStarted := time.Now()
	if upsertErr := h.actors.Upsert(ctx, event.DID, event.Handle); upsertErr != nil {
		timing.actorUpsert = time.Since(stageStarted)
		return upsertErr
	}
	timing.actorUpsert = time.Since(stageStarted)
	return nil
}

func logTapRecordHandlerTiming(
	ctx context.Context,
	event *RecordEvent,
	uri string,
	result string,
	recordBytes int,
	timing recordHandlerTiming,
	total time.Duration,
	actorErr string,
	activityLogErr string,
	activityStatusErr string,
	err error,
) {
	attrs := []any{
		"event_id", event.EventID,
		"action", event.Action,
		"result", result,
		"uri", uri,
		"did", event.DID,
		"collection", event.Collection,
		"rkey", event.RKey,
		"cid", event.CID,
		"record_bytes", recordBytes,
		"total_ms", durationMillis(total),
		"actor_ms", durationMillis(timing.actor),
		"record_ms", durationMillis(timing.record),
		"activity_log_ms", durationMillis(timing.activityLog),
		"activity_status_ms", durationMillis(timing.activityStatus),
		"pubsub_ms", durationMillis(timing.pubsub),
	}
	if actorErr != "" {
		attrs = append(attrs, "actor_error", actorErr)
	}
	if activityLogErr != "" {
		attrs = append(attrs, "activity_log_error", activityLogErr)
	}
	if activityStatusErr != "" {
		attrs = append(attrs, "activity_status_error", activityStatusErr)
	}
	logTapHandlerTiming(ctx, "Tap record handler", total, err, attrs...)
}

func logTapIdentityHandlerTiming(ctx context.Context, event *IdentityEvent, result string, timing identityHandlerTiming, total time.Duration, err error) {
	attrs := []any{
		"event_id", event.EventID,
		"result", result,
		"did", event.DID,
		"handle", event.Handle,
		"status", event.Status,
		"is_active", event.IsActive,
		"total_ms", durationMillis(total),
		"records_delete_ms", durationMillis(timing.recordsDelete),
		"actor_delete_ms", durationMillis(timing.actorDelete),
		"actor_upsert_ms", durationMillis(timing.actorUpsert),
	}
	logTapHandlerTiming(ctx, "Tap identity handler", total, err, attrs...)
}

func logTapHandlerTiming(ctx context.Context, message string, total time.Duration, err error, attrs ...any) {
	if err != nil {
		attrs = append(attrs, "error", err)
		slog.WarnContext(ctx, message+" failed", attrs...)
		return
	}
	if total >= tapHandlerTimingSlowThreshold {
		attrs = append(attrs, "slow_threshold_ms", durationMillis(tapHandlerTimingSlowThreshold))
		slog.WarnContext(ctx, message+" slow", attrs...)
		return
	}
	if tapHandlerTimingLogAll {
		slog.InfoContext(ctx, message+" timing", attrs...)
	}
}

func durationMillis(duration time.Duration) int64 {
	return duration.Milliseconds()
}

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		slog.Warn("Invalid boolean environment value, using fallback", "key", key, "value", value, "fallback", fallback)
		return fallback
	}
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	millis, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		slog.Warn("Invalid duration environment value, using fallback", "key", key, "value", value, "fallback", fallback)
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
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
