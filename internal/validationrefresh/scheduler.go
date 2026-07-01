// Package validationrefresh classifies stored records after lexicon lifecycle
// events so typed GraphQL only serves records validated against current schemas.
package validationrefresh

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/validation"
)

const refreshBatchSize = 500

// Scheduler runs per-collection validation refresh jobs using the local record
// validator and record repository.
type Scheduler struct {
	records   *repositories.RecordsRepository
	validator validation.RecordValidator
	mu        sync.Mutex
	running   map[string]bool
}

// NewScheduler creates an in-process validation refresh scheduler.
func NewScheduler(records *repositories.RecordsRepository, validator validation.RecordValidator) *Scheduler {
	return &Scheduler{records: records, validator: validator, running: make(map[string]bool)}
}

// ScheduleValidationRefresh starts one background refresh for a collection. If a
// refresh for the collection is already running, this call is ignored.
func (s *Scheduler) ScheduleValidationRefresh(collection, reason string) {
	s.mu.Lock()
	if s.running[collection] {
		s.mu.Unlock()
		return
	}
	s.running[collection] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, collection)
			s.mu.Unlock()
		}()

		if err := s.RefreshCollection(context.Background(), collection, reason); err != nil {
			slog.Warn("validation refresh failed", "collection", collection, "reason", reason, "error", err)
		}
	}()
}

// RefreshCollections synchronously refreshes every supplied collection. Use this
// during startup before GraphQL starts serving requests.
func (s *Scheduler) RefreshCollections(ctx context.Context, collections []string, reason string) error {
	for _, collection := range collections {
		if err := s.RefreshCollection(ctx, collection, reason); err != nil {
			return fmt.Errorf("refresh validation for %s: %w", collection, err)
		}
	}
	return nil
}

// RefreshCollection synchronously classifies stale or unvalidated records for a
// collection against the current saved lexicon hash.
func (s *Scheduler) RefreshCollection(ctx context.Context, collection, reason string) error {
	started := time.Now()
	currentHash, ok := s.validator.LexiconHash(collection)
	if !ok {
		return s.records.MarkCollectionUnknownSchema(ctx, collection, fmt.Sprintf("no saved lexicon for collection %s", collection))
	}

	var afterURI string
	var processed, valid, invalid, hidden int
	for {
		records, err := s.records.ListRecordsNeedingValidation(ctx, collection, currentHash, afterURI, refreshBatchSize)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			slog.Info("validation refresh completed", "collection", collection, "reason", reason, "processed", processed, "valid", valid, "invalid", invalid, "unknown_or_error", hidden, "elapsed", time.Since(started))
			return nil
		}

		for _, rec := range records {
			result := s.validator.ValidateRecord(rec.Collection, rec.RKey, []byte(rec.JSON))
			if err := s.records.UpdateValidationStatus(ctx, rec.URI, result.Status, result.Error, result.LexiconHash); err != nil {
				return err
			}
			processed++
			switch result.Status {
			case validation.StatusValid:
				valid++
			case validation.StatusInvalid:
				invalid++
			default:
				hidden++
			}
			afterURI = rec.URI
		}

		if processed%refreshBatchSize == 0 {
			slog.Info("validation refresh progress", "collection", collection, "reason", reason, "processed", processed, "valid", valid, "invalid", invalid, "unknown_or_error", hidden, "elapsed", time.Since(started))
		}
	}
}
