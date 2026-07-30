// Package validationrefresh classifies stored records against the fixed Lexicon
// set selected at startup.
package validationrefresh

import (
	"context"
	"fmt"
	"log/slog"
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
}

// NewScheduler creates a validation refresher for the startup Lexicon set.
func NewScheduler(records *repositories.RecordsRepository, validator validation.RecordValidator) *Scheduler {
	return &Scheduler{records: records, validator: validator}
}

// RefreshCollections synchronously reconciles stored records with the supplied
// startup collection set. Collections absent from that set are marked
// unknown_schema before active collections refresh missing or stale metadata.
func (s *Scheduler) RefreshCollections(ctx context.Context, collections []string, reason string) error {
	active := make(map[string]struct{}, len(collections))
	for _, collection := range collections {
		active[collection] = struct{}{}
	}

	stats, err := s.records.GetCollectionStats(ctx)
	if err != nil {
		return fmt.Errorf("list stored collections for validation cleanup: %w", err)
	}
	for _, stat := range stats {
		if _, ok := active[stat.Collection]; ok {
			continue
		}
		message := fmt.Sprintf("no saved lexicon for collection %s in the startup schema", stat.Collection)
		if err := s.records.MarkCollectionUnknownSchema(ctx, stat.Collection, message); err != nil {
			return fmt.Errorf("mark collection %s unknown schema: %w", stat.Collection, err)
		}
	}

	for _, collection := range collections {
		if err := s.RefreshCollection(ctx, collection, reason); err != nil {
			return fmt.Errorf("refresh validation for %s: %w", collection, err)
		}
	}
	return nil
}

// RefreshCollection synchronously classifies stale or unvalidated records for a
// collection against its startup Lexicon hash.
func (s *Scheduler) RefreshCollection(ctx context.Context, collection, reason string) error {
	started := time.Now()
	currentHash, ok := s.validator.LexiconHash(collection)
	if !ok {
		return s.records.MarkCollectionUnknownSchema(ctx, collection, fmt.Sprintf("no saved lexicon for collection %s", collection))
	}

	var afterURI string
	var processed, valid, invalid, hidden, concurrentSkipped int
	for {
		records, err := s.records.ListRecordsNeedingValidation(ctx, collection, currentHash, afterURI, refreshBatchSize)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			slog.Info("validation refresh completed", "collection", collection, "reason", reason, "processed", processed, "valid", valid, "invalid", invalid, "unknown_or_error", hidden, "concurrent_skipped", concurrentSkipped, "elapsed", time.Since(started))
			return nil
		}

		for _, rec := range records {
			result := s.validator.ValidateRecord(rec.Collection, rec.RKey, []byte(rec.JSON))
			updated, err := s.records.UpdateValidationStatusIfUnchanged(ctx, rec, result.Status, result.Error, result.LexiconHash)
			if err != nil {
				return err
			}
			processed++
			afterURI = rec.URI
			if !updated {
				concurrentSkipped++
				continue
			}
			switch result.Status {
			case validation.StatusValid:
				valid++
			case validation.StatusInvalid:
				invalid++
			default:
				hidden++
			}
		}

		if processed%refreshBatchSize == 0 {
			slog.Info("validation refresh progress", "collection", collection, "reason", reason, "processed", processed, "valid", valid, "invalid", invalid, "unknown_or_error", hidden, "concurrent_skipped", concurrentSkipped, "elapsed", time.Since(started))
		}
	}
}
