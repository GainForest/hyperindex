package tap

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/GainForest/hyperindex/internal/database/repositories"
)

const (
	handleReconcileBatchSize   = 250
	handleReconcileConcurrency = 8
)

// RepoInfoClient provides the Tap identity metadata needed for handle
// reconciliation.
type RepoInfoClient interface {
	RepoInfo(ctx context.Context, did string) (*RepoInfoResponse, error)
}

// HandleReconciliationStats summarizes one startup reconciliation pass.
type HandleReconciliationStats struct {
	Scanned         int64
	Updated         int64
	Unresolved      int64
	AlreadyResolved int64
	Failed          int64
}

type handleReconciliationResult struct {
	updated         bool
	unresolved      bool
	alreadyResolved bool
	failed          bool
}

// ReconcileActorHandles fills missing Hyperindex actor handles from Tap's local
// repo metadata. Updates are conditional so a concurrent identity event always
// wins over reconciliation.
func ReconcileActorHandles(
	ctx context.Context,
	actors *repositories.ActorsRepository,
	client RepoInfoClient,
) (HandleReconciliationStats, error) {
	var stats HandleReconciliationStats
	if actors == nil {
		return stats, fmt.Errorf("reconcile actor handles: actors repository is required")
	}
	if client == nil {
		return stats, fmt.Errorf("reconcile actor handles: Tap repo info client is required")
	}

	afterDID := ""
	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		dids, err := actors.ListDIDsMissingHandle(ctx, afterDID, handleReconcileBatchSize)
		if err != nil {
			return stats, fmt.Errorf("list actors with missing handles after %q: %w", afterDID, err)
		}
		if len(dids) == 0 {
			return stats, nil
		}
		afterDID = dids[len(dids)-1]
		stats.Scanned += int64(len(dids))

		results := reconcileHandleBatch(ctx, actors, client, dids)
		for result := range results {
			switch {
			case result.updated:
				stats.Updated++
			case result.unresolved:
				stats.Unresolved++
			case result.alreadyResolved:
				stats.AlreadyResolved++
			case result.failed:
				stats.Failed++
			}
		}
	}
}

func reconcileHandleBatch(
	ctx context.Context,
	actors *repositories.ActorsRepository,
	client RepoInfoClient,
	dids []string,
) <-chan handleReconciliationResult {
	jobs := make(chan string)
	results := make(chan handleReconciliationResult, len(dids))

	workerCount := handleReconcileConcurrency
	if len(dids) < workerCount {
		workerCount = len(dids)
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for did := range jobs {
				results <- reconcileActorHandle(ctx, actors, client, did)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, did := range dids {
			select {
			case jobs <- did:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

func reconcileActorHandle(
	ctx context.Context,
	actors *repositories.ActorsRepository,
	client RepoInfoClient,
	did string,
) handleReconciliationResult {
	info, err := client.RepoInfo(ctx, did)
	if err != nil {
		slog.Debug("Failed to read actor handle from Tap", "did", did, "error", err)
		return handleReconciliationResult{failed: true}
	}
	if info == nil || info.DID != did {
		slog.Debug("Tap returned mismatched actor identity", "did", did, "response", info)
		return handleReconciliationResult{failed: true}
	}
	if info.Handle == "" {
		return handleReconciliationResult{unresolved: true}
	}

	updated, err := actors.SetHandleIfMissing(ctx, did, info.Handle)
	if err != nil {
		slog.Debug("Failed to store reconciled actor handle", "did", did, "error", err)
		return handleReconciliationResult{failed: true}
	}
	if !updated {
		return handleReconciliationResult{alreadyResolved: true}
	}
	return handleReconciliationResult{updated: true}
}
