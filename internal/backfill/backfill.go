package backfill

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GainForest/hyperindex/internal/atproto"
	"github.com/GainForest/hyperindex/internal/config"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/oauth"
	"github.com/GainForest/hyperindex/internal/validation"
)

// Config configures the backfill operation.
type Config struct {
	// RelayURL is the AT Protocol relay URL for discovering repos.
	RelayURL string

	// PLCURL is the PLC directory URL for resolving DIDs.
	PLCURL string

	// Collections to backfill.
	Collections []string

	// MaxHTTPConcurrent is the global max concurrent HTTP requests (semaphore).
	// This prevents overwhelming the network or running out of file descriptors.
	MaxHTTPConcurrent int

	// MaxPDSWorkers is the max concurrent PDS endpoints being processed.
	// Uses sliding window pattern to maintain constant throughput.
	MaxPDSWorkers int

	// MaxConcurrentPerPDS is the max concurrent requests per PDS.
	MaxConcurrentPerPDS int

	// MaxConcurrentRepos is the max concurrent repo fetches (DID resolution phase).
	MaxConcurrentRepos int
}

// DefaultConfig returns a default backfill configuration with hardcoded defaults.
func DefaultConfig() Config {
	return Config{
		RelayURL:            DefaultRelayURL,
		PLCURL:              DefaultPLCURL,
		MaxHTTPConcurrent:   50,
		MaxPDSWorkers:       10,
		MaxConcurrentPerPDS: 6,
		MaxConcurrentRepos:  50,
	}
}

// NewConfigFromApp creates a backfill Config from the centralized app config.
// Values that are zero/empty in the app config fall back to defaults.
func NewConfigFromApp(cfg *config.Config) Config {
	c := DefaultConfig()

	if cfg.BackfillRelayURL != "" {
		c.RelayURL = cfg.BackfillRelayURL
	}
	if cfg.BackfillPLCURL != "" {
		c.PLCURL = cfg.BackfillPLCURL
	}
	if cfg.BackfillMaxHTTPConcurrent > 0 {
		c.MaxHTTPConcurrent = cfg.BackfillMaxHTTPConcurrent
	}
	if cfg.BackfillMaxPDSWorkers > 0 {
		c.MaxPDSWorkers = cfg.BackfillMaxPDSWorkers
	}
	if cfg.BackfillMaxPerPDS > 0 {
		c.MaxConcurrentPerPDS = cfg.BackfillMaxPerPDS
	}
	if cfg.BackfillMaxRepos > 0 {
		c.MaxConcurrentRepos = cfg.BackfillMaxRepos
	}

	c.Collections = atproto.ParseCollections(cfg.BackfillCollections)

	return c
}

// Stats tracks backfill statistics.
type Stats struct {
	ReposDiscovered int64
	ReposProcessed  int64
	RecordsInserted int64
	RecordsSkipped  int64 // Records filtered out by CID deduplication
	Errors          int64
	StartTime       time.Time
	EndTime         time.Time
}

// Duration returns the backfill duration.
func (s *Stats) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

const maxBackfillErrorDetails = 20

type boundedErrorCollector struct {
	mu    sync.Mutex
	total int
	errs  []error
}

func (c *boundedErrorCollector) add(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total++
	if len(c.errs) < maxBackfillErrorDetails {
		c.errs = append(c.errs, err)
	}
}

func (c *boundedErrorCollector) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total == 0 {
		return nil
	}
	parts := make([]string, 0, len(c.errs))
	for _, err := range c.errs {
		parts = append(parts, err.Error())
	}
	summary := strings.Join(parts, "; ")
	if c.total > len(c.errs) {
		summary += fmt.Sprintf("; and %d more", c.total-len(c.errs))
	}
	return fmt.Errorf("backfill incomplete with %d error(s): %s", c.total, summary)
}

type backfillClient interface {
	ListReposByCollection(ctx context.Context, collection string) ([]string, error)
	ResolveDID(ctx context.Context, did string) (*AtprotoData, error)
	GetRepo(ctx context.Context, pdsURL, did string, collections []string) ([]CARRecord, error)
	ListRecords(ctx context.Context, pdsURL, repoDID, collection string) ([]ListRecordsRecord, error)
}

// Backfiller coordinates historical data backfill.
type Backfiller struct {
	config       Config
	client       backfillClient
	recordsRepo  *repositories.RecordsRepository
	actorsRepo   *repositories.ActorsRepository
	activityRepo *repositories.IndexingActivityRepository
	validator    validation.RecordValidator

	// httpSem is a global semaphore limiting concurrent HTTP requests.
	// This prevents overwhelming the network and running out of file descriptors.
	httpSem chan struct{}

	// didCache caches DID document resolutions to avoid redundant PLC lookups.
	didCache         *oauth.DIDCache
	stopCacheCleanup func()

	stats Stats
}

// NewBackfiller creates a new backfiller.
func NewBackfiller(
	cfg Config,
	recordsRepo *repositories.RecordsRepository,
	actorsRepo *repositories.ActorsRepository,
	activityRepo *repositories.IndexingActivityRepository,
	validators ...validation.RecordValidator,
) *Backfiller {
	// Create DID resolver with custom PLC URL
	didResolver := oauth.NewDIDResolver(
		oauth.WithPLCDirectoryURL(cfg.PLCURL),
	)

	// Create DID cache with 1 hour TTL
	didCache := oauth.NewDIDCache(
		oauth.WithResolver(didResolver),
		oauth.WithCacheTTL(time.Hour),
	)

	// Start cleanup routine (every 5 minutes)
	stopCleanup := didCache.StartCleanupRoutine(5 * time.Minute)

	var validator validation.RecordValidator
	if len(validators) > 0 {
		validator = validators[0]
	}

	return &Backfiller{
		config:           cfg,
		client:           NewClient(cfg.RelayURL, cfg.PLCURL, cfg.MaxHTTPConcurrent),
		recordsRepo:      recordsRepo,
		actorsRepo:       actorsRepo,
		activityRepo:     activityRepo,
		validator:        validator,
		httpSem:          make(chan struct{}, cfg.MaxHTTPConcurrent),
		didCache:         didCache,
		stopCacheCleanup: stopCleanup,
	}
}

// Close stops the DID cache cleanup routine.
// Should be called when the Backfiller is no longer needed.
func (b *Backfiller) Close() {
	if b.stopCacheCleanup != nil {
		b.stopCacheCleanup()
	}
}

// Run executes the backfill operation.
func (b *Backfiller) Run(ctx context.Context) (*Stats, error) {
	b.stats = Stats{StartTime: time.Now()}
	issues := &boundedErrorCollector{}

	slog.Info("[backfill] Starting backfill operation",
		"collections", b.config.Collections,
		"relay", b.config.RelayURL,
		"max_http_concurrent", b.config.MaxHTTPConcurrent,
		"max_pds_workers", b.config.MaxPDSWorkers,
		"max_per_pds", b.config.MaxConcurrentPerPDS,
	)

	// Step 1: Discover all repos for all collections
	allRepos := make(map[string]struct{})
	for _, collection := range b.config.Collections {
		slog.Info("[backfill] Discovering repos for collection", "collection", collection)

		repos, err := b.client.ListReposByCollection(ctx, collection)
		if err != nil {
			slog.Warn("[backfill] Failed to list repos for collection",
				"collection", collection,
				"error", err,
			)
			atomic.AddInt64(&b.stats.Errors, 1)
			issues.add(fmt.Errorf("discover collection %s: %w", collection, err))
			continue
		}

		for _, repo := range repos {
			allRepos[repo] = struct{}{}
		}

		slog.Info("[backfill] Found repos for collection",
			"collection", collection,
			"count", len(repos),
		)
	}

	repoList := make([]string, 0, len(allRepos))
	for repo := range allRepos {
		repoList = append(repoList, repo)
	}
	atomic.StoreInt64(&b.stats.ReposDiscovered, int64(len(repoList)))

	slog.Info("[backfill] Total unique repos discovered", "count", len(repoList))

	if len(repoList) == 0 {
		slog.Info("[backfill] No repos found, nothing to backfill")
		b.stats.EndTime = time.Now()
		if ctx.Err() != nil {
			return &b.stats, ctx.Err()
		}
		return &b.stats, issues.err()
	}

	// Step 2: Resolve DIDs and group by PDS
	slog.Info("[backfill] Resolving DIDs...")
	reposByPDS := b.resolveAndGroupByPDS(ctx, repoList, issues)

	// Step 3: Process each PDS concurrently
	slog.Info("[backfill] Processing repos by PDS",
		"pds_count", len(reposByPDS),
		"max_concurrent", b.config.MaxConcurrentRepos,
	)

	b.processReposByPDS(ctx, reposByPDS, issues)

	b.stats.EndTime = time.Now()

	slog.Info("[backfill] Backfill complete",
		"repos_discovered", b.stats.ReposDiscovered,
		"repos_processed", b.stats.ReposProcessed,
		"records_inserted", b.stats.RecordsInserted,
		"records_skipped", b.stats.RecordsSkipped,
		"errors", b.stats.Errors,
		"duration", b.stats.Duration(),
	)

	if ctx.Err() != nil {
		return &b.stats, ctx.Err()
	}
	return &b.stats, issues.err()
}

// resolveAndGroupByPDS resolves DIDs and groups repos by their PDS.
// Uses DID caching to avoid redundant PLC lookups and batch upsert for actors.
func (b *Backfiller) resolveAndGroupByPDS(ctx context.Context, repos []string, issues *boundedErrorCollector) map[string][]*AtprotoData {
	result := make(map[string][]*AtprotoData)
	var mu sync.Mutex

	// Collect all resolved actors for batch upsert
	var allResolved []*AtprotoData

	// Track cache hits for logging
	var cacheHits, cacheMisses int64

	// Use a semaphore to limit concurrent DID resolutions
	sem := make(chan struct{}, b.config.MaxConcurrentRepos)
	var wg sync.WaitGroup

	for _, repo := range repos {
		wg.Add(1)
		go func(did string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			// Try DID cache first (internally handles HTTP if not cached)
			doc, err := b.didCache.Get(did)
			if err != nil {
				// Cache miss - need to use HTTP semaphore for rate limiting
				b.httpSem <- struct{}{}
				doc, err = b.didCache.GetWithInvalidate(did, true)
				<-b.httpSem

				if err != nil {
					slog.Debug("[backfill] Failed to resolve DID",
						"did", did,
						"error", err,
					)
					atomic.AddInt64(&b.stats.Errors, 1)
					issues.add(fmt.Errorf("resolve DID %s: %w", did, err))
					return
				}
				atomic.AddInt64(&cacheMisses, 1)
			} else {
				atomic.AddInt64(&cacheHits, 1)
			}

			// Convert oauth.DIDDocument to AtprotoData
			data := &AtprotoData{
				DID:    did,
				Handle: doc.GetHandle(),
				PDS:    doc.GetPDSEndpoint(),
			}

			// Default handle to DID if not found
			if data.Handle == "" {
				data.Handle = did
			}
			// Default PDS if not found
			if data.PDS == "" {
				data.PDS = "https://bsky.social"
			}

			mu.Lock()
			result[data.PDS] = append(result[data.PDS], data)
			allResolved = append(allResolved, data)
			mu.Unlock()
		}(repo)
	}

	wg.Wait()

	// Log cache statistics
	slog.Info("[backfill] DID resolution complete",
		"total", len(repos),
		"resolved", len(allResolved),
		"cache_hits", cacheHits,
		"cache_misses", cacheMisses,
		"cache_size", b.didCache.Size(),
	)

	// Batch upsert all actors at once
	if len(allResolved) > 0 {
		actors := make([]repositories.ActorData, len(allResolved))
		for i, data := range allResolved {
			actors[i] = repositories.ActorData{
				DID:    data.DID,
				Handle: data.Handle,
			}
		}

		if err := b.actorsRepo.BatchUpsert(ctx, actors); err != nil {
			slog.Warn("[backfill] Failed to batch upsert actors",
				"count", len(actors),
				"error", err,
			)
		} else {
			slog.Info("[backfill] Batch upserted actors", "count", len(actors))
		}
	}

	return result
}

// pdsEntry holds PDS URL and its repos for sliding window processing.
type pdsEntry struct {
	pdsURL string
	repos  []*AtprotoData
}

// processReposByPDS processes repos grouped by PDS using sliding window pattern.
// This limits the number of concurrent PDS workers to maintain consistent throughput.
func (b *Backfiller) processReposByPDS(ctx context.Context, reposByPDS map[string][]*AtprotoData, issues *boundedErrorCollector) {
	// Convert map to slice for ordered processing
	entries := make([]pdsEntry, 0, len(reposByPDS))
	for pdsURL, repos := range reposByPDS {
		entries = append(entries, pdsEntry{pdsURL: pdsURL, repos: repos})
	}

	totalPDS := len(entries)
	if totalPDS == 0 {
		return
	}

	// Use sliding window: limit concurrent PDS workers
	pdsSem := make(chan struct{}, b.config.MaxPDSWorkers)
	results := make(chan int, totalPDS)
	var wg sync.WaitGroup

	// Start all workers (they'll block on the semaphore)
	for _, entry := range entries {
		wg.Add(1)
		go func(e pdsEntry) {
			defer wg.Done()

			// Acquire PDS slot
			pdsSem <- struct{}{}
			defer func() { <-pdsSem }()

			count := b.processPDS(ctx, e.pdsURL, e.repos, issues)
			results <- count
		}(entry)
	}

	// Collect results and log progress
	go func() {
		completed := 0
		for count := range results {
			completed++
			slog.Info("[backfill] PDS worker completed",
				"progress", fmt.Sprintf("%d/%d", completed, totalPDS),
				"records", count,
			)
		}
	}()

	wg.Wait()
	close(results)
}

// processPDS processes all repos for a single PDS and returns the total records processed.
func (b *Backfiller) processPDS(ctx context.Context, pdsURL string, repos []*AtprotoData, issues *boundedErrorCollector) int {
	startTime := time.Now()
	slog.Debug("[backfill] Processing PDS",
		"pds", pdsURL,
		"repo_count", len(repos),
	)

	// Use a semaphore to limit concurrent requests to this PDS
	sem := make(chan struct{}, b.config.MaxConcurrentPerPDS)
	var wg sync.WaitGroup
	var totalRecords int64

	for _, repo := range repos {
		wg.Add(1)
		go func(data *AtprotoData) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			count, err := b.safeProcessRepo(ctx, pdsURL, data)
			if err != nil {
				slog.Warn("[backfill] Repo worker failed",
					"did", data.DID,
					"pds", pdsURL,
					"error", err,
				)
				issues.add(fmt.Errorf("repo %s: %w", data.DID, err))
			}
			// Successfully committed partial records still contribute to totals even
			// when the repository remains incomplete.
			atomic.AddInt64(&totalRecords, int64(count))
		}(repo)
	}

	wg.Wait()

	slog.Debug("[backfill] Finished PDS",
		"pds", pdsURL,
		"repos", len(repos),
		"records", totalRecords,
		"duration", time.Since(startTime),
	)

	return int(totalRecords)
}

// safeProcessRepo wraps processRepo with panic recovery.
// This prevents a single repo from crashing the entire backfill operation.
func (b *Backfiller) safeProcessRepo(ctx context.Context, pdsURL string, data *AtprotoData) (count int, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("[backfill] Worker panic recovered",
				"did", data.DID,
				"pds", pdsURL,
				"error", fmt.Sprintf("%v", recovered),
			)
			count = 0
			err = fmt.Errorf("backfill repo worker panic: %v", recovered)
		}
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			atomic.AddInt64(&b.stats.Errors, 1)
		}
	}()
	return b.processRepo(ctx, pdsURL, data)
}

// processRepo processes a single repo using CAR-based fetching.
// This fetches the entire repo in a single HTTP request and filters locally.
// Returns the number of records inserted.
func (b *Backfiller) processRepo(ctx context.Context, pdsURL string, data *AtprotoData) (int, error) {
	b.httpSem <- struct{}{}
	carRecords, carErr := b.client.GetRepo(ctx, pdsURL, data.DID, b.config.Collections)
	<-b.httpSem

	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if carErr != nil && isCARFallbackFailure(carErr) {
		slog.Debug("[backfill] CAR unavailable; using listRecords fallback", "did", data.DID, "error", carErr)
		return b.processRepoLegacy(ctx, pdsURL, data)
	}
	if carErr != nil && !isCARIntegrityFailure(carErr) {
		return 0, carErr
	}

	dbRecords, conversionErr := convertCARRecords(data.DID, carRecords)
	integrityErr := combineCARIntegrityFailures(carErr, conversionErr)
	inserted, writeErr := b.persistRecordBatch(ctx, data.DID, dbRecords)
	if writeErr != nil {
		return 0, writeErr
	}
	if integrityErr != nil {
		return inserted, integrityErr
	}

	atomic.AddInt64(&b.stats.ReposProcessed, 1)
	return inserted, nil
}

func convertCARRecords(did string, records []CARRecord) ([]*repositories.Record, error) {
	dbRecords := make([]*repositories.Record, 0, len(records))
	var failures boundedCARFailures
	for _, rec := range records {
		jsonStr, err := CBORToJSON(rec.Value)
		if err != nil {
			failures.add(fmt.Errorf("convert CAR record %s to JSON: %w", rec.URI, err))
			continue
		}
		dbRecords = append(dbRecords, &repositories.Record{
			URI: rec.URI, CID: rec.CID, DID: did, Collection: rec.Collection, JSON: jsonStr, RKey: rec.RKey,
		})
	}
	return dbRecords, failures.err()
}

type backfillBatchResult struct {
	inserted int
	skipped  int
}

func (b *Backfiller) persistRecordBatch(ctx context.Context, did string, records []*repositories.Record) (int, error) {
	result, err := b.persistBackfillBatch(ctx, did, records)
	if err != nil {
		return 0, err
	}
	atomic.AddInt64(&b.stats.RecordsInserted, int64(result.inserted))
	atomic.AddInt64(&b.stats.RecordsSkipped, int64(result.skipped))
	return result.inserted, nil
}

func (b *Backfiller) persistBackfillBatch(ctx context.Context, did string, records []*repositories.Record) (result backfillBatchResult, err error) {
	if len(records) == 0 {
		return result, nil
	}
	committed, err := b.recordsRepo.BatchUpsertWithValidationForBackfill(ctx, did, b.validationWrites(records))
	if err != nil {
		return result, err
	}
	changed := make([]*repositories.Record, 0, len(committed.ChangedIndices))
	for _, index := range committed.ChangedIndices {
		changed = append(changed, records[index])
	}
	b.logChangedRecordActivity(ctx, changed)
	return backfillBatchResult{inserted: len(changed), skipped: committed.Skipped}, nil
}

func (b *Backfiller) logChangedRecordActivity(ctx context.Context, records []*repositories.Record) {
	if b.activityRepo == nil {
		return
	}
	for _, rec := range records {
		timestamp := atproto.ExtractCreatedAt(rec.JSON, time.Now())
		if _, err := b.activityRepo.LogActivityWithStatus(ctx, timestamp, "create", rec.Collection, rec.DID, rec.RKey, rec.JSON, "success"); err != nil {
			slog.Debug("[backfill] Failed to log activity", "uri", rec.URI, "error", err)
		}
	}
}

// processRepoLegacy processes a repo using per-collection listRecords (fallback).
// Returns the number of records inserted.
func (b *Backfiller) processRepoLegacy(ctx context.Context, pdsURL string, data *AtprotoData) (int, error) {
	var totalInserted int
	issues := &boundedErrorCollector{}

	for _, collection := range b.config.Collections {
		b.httpSem <- struct{}{}
		records, err := b.client.ListRecords(ctx, pdsURL, data.DID, collection)
		<-b.httpSem
		if ctx.Err() != nil {
			return totalInserted, ctx.Err()
		}
		if err != nil {
			issues.add(fmt.Errorf("list records for %s in collection %s: %w", data.DID, collection, err))
			continue
		}

		dbRecords := make([]*repositories.Record, 0, len(records))
		for _, rec := range records {
			dbRecords = append(dbRecords, &repositories.Record{
				URI: rec.URI, CID: rec.CID, DID: data.DID, Collection: collection,
				RKey: extractRKeyFromURI(rec.URI), JSON: string(rec.Value),
			})
		}
		inserted, err := b.persistRecordBatch(ctx, data.DID, dbRecords)
		if err != nil {
			issues.add(fmt.Errorf("collection %s: %w", collection, err))
			continue
		}
		totalInserted += inserted
	}

	if err := issues.err(); err != nil {
		return totalInserted, err
	}
	atomic.AddInt64(&b.stats.ReposProcessed, 1)
	return totalInserted, nil
}

// BackfillActor backfills all collections for a single actor using CAR-based fetching.
func (b *Backfiller) BackfillActor(ctx context.Context, did string) (int, error) {
	slog.Info("[backfill] Starting actor backfill", "did", did)
	startTime := time.Now()

	// Resolve DID
	data, err := b.client.ResolveDID(ctx, did)
	if err != nil {
		return 0, err
	}

	// Ensure actor exists
	if err := b.actorsRepo.Upsert(ctx, data.DID, data.Handle); err != nil {
		slog.Warn("[backfill] Failed to upsert actor", "did", did, "error", err)
	}

	// Try CAR-based approach first
	carRecords, carErr := b.client.GetRepo(ctx, data.PDS, data.DID, b.config.Collections)
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if carErr != nil && isCARFallbackFailure(carErr) {
		slog.Warn("[backfill] CAR unavailable, falling back to listRecords", "did", did, "error", carErr)
		return b.backfillActorLegacy(ctx, data)
	}
	if carErr != nil && !isCARIntegrityFailure(carErr) {
		return 0, carErr
	}

	dbRecords, conversionErr := convertCARRecords(data.DID, carRecords)
	integrityErr := combineCARIntegrityFailures(carErr, conversionErr)

	if len(dbRecords) == 0 {
		if integrityErr != nil {
			return 0, integrityErr
		}
		slog.Info("[backfill] Actor backfill complete (CAR) - no records",
			"did", did,
			"duration", time.Since(startTime),
		)
		return 0, nil
	}

	result, err := b.persistBackfillBatch(ctx, data.DID, dbRecords)
	if err != nil {
		return 0, err
	}
	if integrityErr != nil {
		return result.inserted, integrityErr
	}

	slog.Info("[backfill] Actor backfill complete (CAR)",
		"did", did,
		"records", result.inserted,
		"skipped", result.skipped,
		"duration", time.Since(startTime),
	)

	return result.inserted, nil
}

// backfillActorLegacy uses per-collection listRecords (fallback).
func (b *Backfiller) backfillActorLegacy(ctx context.Context, data *AtprotoData) (int, error) {
	var totalRecords int
	issues := &boundedErrorCollector{}
	for _, collection := range b.config.Collections {
		records, err := b.client.ListRecords(ctx, data.PDS, data.DID, collection)
		if ctx.Err() != nil {
			return totalRecords, ctx.Err()
		}
		if err != nil {
			issues.add(fmt.Errorf("list records for %s in collection %s: %w", data.DID, collection, err))
			continue
		}

		dbRecords := make([]*repositories.Record, 0, len(records))
		for _, rec := range records {
			dbRecords = append(dbRecords, &repositories.Record{
				URI: rec.URI, CID: rec.CID, DID: data.DID, Collection: collection,
				RKey: extractRKeyFromURI(rec.URI), JSON: string(rec.Value),
			})
		}
		result, err := b.persistBackfillBatch(ctx, data.DID, dbRecords)
		if err != nil {
			issues.add(fmt.Errorf("collection %s: %w", collection, err))
			continue
		}
		totalRecords += result.inserted
	}

	slog.Info("[backfill] Actor backfill complete (legacy)", "did", data.DID, "records", totalRecords)
	return totalRecords, issues.err()
}

func (b *Backfiller) validationWrites(records []*repositories.Record) []repositories.RecordWrite {
	writes := make([]repositories.RecordWrite, 0, len(records))
	for _, rec := range records {
		result := validation.ClassifyRecord(b.validator, rec.Collection, rec.RKey, []byte(rec.JSON))
		writes = append(writes, repositories.RecordWrite{
			URI: rec.URI, CID: rec.CID, DID: rec.DID, Collection: rec.Collection, RKey: rec.RKey, JSON: rec.JSON,
			ValidationStatus: result.Status, ValidationError: result.Error, LexiconHash: result.LexiconHash,
		})
	}
	return writes
}

// extractRKeyFromURI extracts the rkey from an AT-URI (at://did/collection/rkey).
func extractRKeyFromURI(uri string) string {
	// URI format: at://did/collection/rkey
	parts := strings.Split(uri, "/")
	if len(parts) >= 5 {
		return parts[len(parts)-1]
	}
	return ""
}
