package backfill

import (
	"context"
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

// Backfiller coordinates historical data backfill.
type Backfiller struct {
	config       Config
	client       *Client
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
		return &b.stats, nil
	}

	// Step 2: Resolve DIDs and group by PDS
	slog.Info("[backfill] Resolving DIDs...")
	reposByPDS := b.resolveAndGroupByPDS(ctx, repoList)

	// Step 3: Process each PDS concurrently
	slog.Info("[backfill] Processing repos by PDS",
		"pds_count", len(reposByPDS),
		"max_concurrent", b.config.MaxConcurrentRepos,
	)

	b.processReposByPDS(ctx, reposByPDS)

	b.stats.EndTime = time.Now()

	slog.Info("[backfill] Backfill complete",
		"repos_discovered", b.stats.ReposDiscovered,
		"repos_processed", b.stats.ReposProcessed,
		"records_inserted", b.stats.RecordsInserted,
		"records_skipped", b.stats.RecordsSkipped,
		"errors", b.stats.Errors,
		"duration", b.stats.Duration(),
	)

	return &b.stats, nil
}

// resolveAndGroupByPDS resolves DIDs and groups repos by their PDS.
// Uses DID caching to avoid redundant PLC lookups and batch upsert for actors.
func (b *Backfiller) resolveAndGroupByPDS(ctx context.Context, repos []string) map[string][]*AtprotoData {
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
func (b *Backfiller) processReposByPDS(ctx context.Context, reposByPDS map[string][]*AtprotoData) {
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

			count := b.processPDS(ctx, e.pdsURL, e.repos)
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
func (b *Backfiller) processPDS(ctx context.Context, pdsURL string, repos []*AtprotoData) int {
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

			count := b.safeProcessRepo(ctx, pdsURL, data)
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
func (b *Backfiller) safeProcessRepo(ctx context.Context, pdsURL string, data *AtprotoData) (count int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("[backfill] Worker panic recovered",
				"did", data.DID,
				"pds", pdsURL,
				"error", fmt.Sprintf("%v", r),
			)
			atomic.AddInt64(&b.stats.Errors, 1)
			count = 0
		}
	}()
	return b.processRepo(ctx, pdsURL, data)
}

// processRepo processes a single repo using CAR-based fetching.
// This fetches the entire repo in a single HTTP request and filters locally.
// Returns the number of records inserted.
func (b *Backfiller) processRepo(ctx context.Context, pdsURL string, data *AtprotoData) int {
	totalStart := time.Now()

	// Phase 1: Fetch CAR
	fetchStart := time.Now()

	// Acquire global HTTP semaphore
	b.httpSem <- struct{}{}
	carRecords, err := b.client.GetRepo(ctx, pdsURL, data.DID, b.config.Collections)
	<-b.httpSem // Release immediately after HTTP completes

	fetchMs := time.Since(fetchStart).Milliseconds()

	if err != nil {
		slog.Debug("[backfill] Failed to get repo via CAR",
			"did", data.DID,
			"error", err,
		)
		// Fallback to listRecords approach
		return b.processRepoLegacy(ctx, pdsURL, data)
	}

	// Phase 2: Parse/Convert CBOR to JSON
	parseStart := time.Now()
	dbRecords := make([]*repositories.Record, 0, len(carRecords))
	for _, rec := range carRecords {
		jsonStr, err := CBORToJSON(rec.Value)
		if err != nil {
			slog.Debug("[backfill] Failed to convert CBOR to JSON",
				"uri", rec.URI,
				"error", err,
			)
			continue
		}

		dbRecords = append(dbRecords, &repositories.Record{
			URI:        rec.URI,
			CID:        rec.CID,
			DID:        data.DID,
			Collection: rec.Collection,
			JSON:       jsonStr,
			RKey:       rec.RKey,
		})
	}
	parseMs := time.Since(parseStart).Milliseconds()

	if len(dbRecords) == 0 {
		atomic.AddInt64(&b.stats.ReposProcessed, 1)
		return 0
	}

	// Phase 3: CID deduplication
	dedupStart := time.Now()
	filteredRecords, skipped := b.filterByExistingCIDs(ctx, dbRecords)
	atomic.AddInt64(&b.stats.RecordsSkipped, int64(skipped))
	dedupMs := time.Since(dedupStart).Milliseconds()

	// Phase 4: Batch insert
	insertStart := time.Now()
	insertedCount := 0
	if len(filteredRecords) > 0 {
		if err := b.recordsRepo.BatchInsert(ctx, filteredRecords); err != nil {
			slog.Warn("[backfill] Failed to batch insert records",
				"did", data.DID,
				"count", len(filteredRecords),
				"error", err,
			)
			atomic.AddInt64(&b.stats.Errors, 1)
		} else {
			b.updateValidationMetadata(ctx, filteredRecords)
			insertedCount = len(filteredRecords)
			atomic.AddInt64(&b.stats.RecordsInserted, int64(insertedCount))

			// Log activity for each inserted record (with 'success' status since already inserted)
			if b.activityRepo != nil {
				for _, rec := range filteredRecords {
					timestamp := atproto.ExtractCreatedAt(rec.JSON, time.Now())
					_, err := b.activityRepo.LogActivityWithStatus(ctx, timestamp, "create", rec.Collection, rec.DID, rec.RKey, rec.JSON, "success")
					if err != nil {
						slog.Debug("[backfill] Failed to log activity", "uri", rec.URI, "error", err)
					}
				}
			}
		}
	}
	insertMs := time.Since(insertStart).Milliseconds()

	atomic.AddInt64(&b.stats.ReposProcessed, 1)
	totalMs := time.Since(totalStart).Milliseconds()

	slog.Debug("[backfill] Processed repo",
		"did", data.DID,
		"fetch_ms", fetchMs,
		"parse_ms", parseMs,
		"dedup_ms", dedupMs,
		"insert_ms", insertMs,
		"total_ms", totalMs,
		"records", insertedCount,
		"skipped", skipped,
	)

	return insertedCount
}

// filterByExistingCIDs filters out records that already exist with the same CID.
// Returns the filtered records and the count of skipped records.
func (b *Backfiller) filterByExistingCIDs(ctx context.Context, records []*repositories.Record) ([]*repositories.Record, int) {
	if len(records) == 0 {
		return records, 0
	}

	// Collect URIs and CIDs
	uris := make([]string, len(records))
	cids := make([]string, 0, len(records))
	cidSet := make(map[string]bool)

	for i, rec := range records {
		uris[i] = rec.URI
		if !cidSet[rec.CID] {
			cids = append(cids, rec.CID)
			cidSet[rec.CID] = true
		}
	}

	// Get existing URI->CID mappings
	existingByCID, err := b.recordsRepo.GetCIDsByURIs(ctx, uris)
	if err != nil {
		slog.Debug("[backfill] Failed to get existing CIDs by URI, skipping dedup", "error", err)
		return records, 0
	}

	// Get existing CIDs (for content dedup)
	existingCIDs, err := b.recordsRepo.GetExistingCIDs(ctx, cids)
	if err != nil {
		slog.Debug("[backfill] Failed to get existing CIDs, skipping content dedup", "error", err)
		existingCIDs = make(map[string]bool)
	}

	// Filter records
	filtered := make([]*repositories.Record, 0, len(records))
	skipped := 0

	for _, rec := range records {
		// Check if URI exists with same CID (unchanged)
		if existingCID, ok := existingByCID[rec.URI]; ok {
			if existingCID == rec.CID {
				skipped++
				continue
			}
			// URI exists with different CID - check if new CID exists elsewhere
			if existingCIDs[rec.CID] {
				skipped++
				continue
			}
		} else if existingCIDs[rec.CID] {
			// URI doesn't exist - check if CID exists elsewhere (duplicate content)
			skipped++
			continue
		}

		filtered = append(filtered, rec)
	}

	return filtered, skipped
}

// processRepoLegacy processes a repo using per-collection listRecords (fallback).
// Returns the number of records inserted.
func (b *Backfiller) processRepoLegacy(ctx context.Context, pdsURL string, data *AtprotoData) int {
	var totalInserted int

	for _, collection := range b.config.Collections {
		// Acquire global HTTP semaphore for each request
		b.httpSem <- struct{}{}
		records, err := b.client.ListRecords(ctx, pdsURL, data.DID, collection)
		<-b.httpSem

		if err != nil {
			slog.Debug("[backfill] Failed to list records",
				"did", data.DID,
				"collection", collection,
				"error", err,
			)
			atomic.AddInt64(&b.stats.Errors, 1)
			continue
		}

		for _, rec := range records {
			result, err := b.recordsRepo.Insert(ctx, rec.URI, rec.CID, data.DID, collection, string(rec.Value))
			if err != nil {
				slog.Debug("[backfill] Failed to insert record",
					"uri", rec.URI,
					"error", err,
				)
				atomic.AddInt64(&b.stats.Errors, 1)
				continue
			}

			if result == repositories.Inserted {
				b.updateRecordValidationMetadata(ctx, rec.URI, collection, extractRKeyFromURI(rec.URI), []byte(rec.Value))
				totalInserted++
				atomic.AddInt64(&b.stats.RecordsInserted, 1)
				// Log activity for the inserted record
				if b.activityRepo != nil {
					timestamp := atproto.ExtractCreatedAt(string(rec.Value), time.Now())
					rkey := extractRKeyFromURI(rec.URI)
					_, err := b.activityRepo.LogActivityWithStatus(ctx, timestamp, "create", collection, data.DID, rkey, string(rec.Value), "success")
					if err != nil {
						slog.Debug("[backfill] Failed to log activity", "uri", rec.URI, "error", err)
					}
				}
			}
		}
	}

	atomic.AddInt64(&b.stats.ReposProcessed, 1)
	return totalInserted
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
	carRecords, err := b.client.GetRepo(ctx, data.PDS, data.DID, b.config.Collections)
	if err != nil {
		slog.Warn("[backfill] CAR fetch failed, falling back to listRecords",
			"did", did,
			"error", err,
		)
		return b.backfillActorLegacy(ctx, data)
	}

	// Convert CAR records to database records
	dbRecords := make([]*repositories.Record, 0, len(carRecords))
	for _, rec := range carRecords {
		// Convert CBOR to JSON
		jsonStr, err := CBORToJSON(rec.Value)
		if err != nil {
			slog.Debug("[backfill] Failed to convert CBOR to JSON",
				"uri", rec.URI,
				"error", err,
			)
			continue
		}

		dbRecords = append(dbRecords, &repositories.Record{
			URI:        rec.URI,
			CID:        rec.CID,
			DID:        data.DID,
			Collection: rec.Collection,
			JSON:       jsonStr,
			RKey:       rec.RKey,
		})
	}

	if len(dbRecords) == 0 {
		slog.Info("[backfill] Actor backfill complete (CAR) - no records",
			"did", did,
			"duration", time.Since(startTime),
		)
		return 0, nil
	}

	// CID deduplication: filter out unchanged records before insert
	filteredRecords, skipped := b.filterByExistingCIDs(ctx, dbRecords)

	// Batch insert filtered records
	if len(filteredRecords) > 0 {
		if err := b.recordsRepo.BatchInsert(ctx, filteredRecords); err != nil {
			return 0, fmt.Errorf("batch insert failed: %w", err)
		}
		b.updateValidationMetadata(ctx, filteredRecords)

		// Log activity for each inserted record
		if b.activityRepo != nil {
			for _, rec := range filteredRecords {
				timestamp := atproto.ExtractCreatedAt(rec.JSON, time.Now())
				_, err := b.activityRepo.LogActivityWithStatus(ctx, timestamp, "create", rec.Collection, rec.DID, rec.RKey, rec.JSON, "success")
				if err != nil {
					slog.Debug("[backfill] Failed to log activity", "uri", rec.URI, "error", err)
				}
			}
		}
	}

	slog.Info("[backfill] Actor backfill complete (CAR)",
		"did", did,
		"records", len(filteredRecords),
		"skipped", skipped,
		"duration", time.Since(startTime),
	)

	return len(filteredRecords), nil
}

// backfillActorLegacy uses per-collection listRecords (fallback).
func (b *Backfiller) backfillActorLegacy(ctx context.Context, data *AtprotoData) (int, error) {
	var totalRecords int
	for _, collection := range b.config.Collections {
		records, err := b.client.ListRecords(ctx, data.PDS, data.DID, collection)
		if err != nil {
			slog.Warn("[backfill] Failed to list records for actor",
				"did", data.DID,
				"collection", collection,
				"error", err,
			)
			continue
		}

		for _, rec := range records {
			result, err := b.recordsRepo.Insert(ctx, rec.URI, rec.CID, data.DID, collection, string(rec.Value))
			if err != nil {
				slog.Debug("[backfill] Failed to insert record", "uri", rec.URI, "error", err)
				continue
			}

			if result == repositories.Inserted {
				b.updateRecordValidationMetadata(ctx, rec.URI, collection, extractRKeyFromURI(rec.URI), []byte(rec.Value))
				totalRecords++
				// Log activity for the inserted record
				if b.activityRepo != nil {
					timestamp := atproto.ExtractCreatedAt(string(rec.Value), time.Now())
					rkey := extractRKeyFromURI(rec.URI)
					_, err := b.activityRepo.LogActivityWithStatus(ctx, timestamp, "create", collection, data.DID, rkey, string(rec.Value), "success")
					if err != nil {
						slog.Debug("[backfill] Failed to log activity", "uri", rec.URI, "error", err)
					}
				}
			}
		}
	}

	slog.Info("[backfill] Actor backfill complete (legacy)",
		"did", data.DID,
		"records", totalRecords,
	)

	return totalRecords, nil
}

func (b *Backfiller) updateValidationMetadata(ctx context.Context, records []*repositories.Record) {
	if b.validator == nil {
		return
	}
	for _, rec := range records {
		b.updateRecordValidationMetadata(ctx, rec.URI, rec.Collection, rec.RKey, []byte(rec.JSON))
	}
}

func (b *Backfiller) updateRecordValidationMetadata(ctx context.Context, uri, collection, rkey string, rawJSON []byte) {
	if b.validator == nil {
		return
	}
	result := b.validator.ValidateRecord(collection, rkey, rawJSON)
	if err := b.recordsRepo.UpdateValidationStatus(ctx, uri, result.Status, result.Error, result.LexiconHash); err != nil {
		slog.Warn("[backfill] Failed to update validation metadata", "uri", uri, "collection", collection, "rkey", rkey, "status", result.Status, "error", err)
		atomic.AddInt64(&b.stats.Errors, 1)
	}
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
