// Package backfill provides historical data fetching from AT Protocol relays and PDS servers.
package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/atdata"
	"github.com/bluesky-social/indigo/repo"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/ipfs/go-cid"
)

const (
	// DefaultRelayURL is the default Bluesky relay.
	DefaultRelayURL = "https://relay1.us-west.bsky.network"

	// DefaultPLCURL is the default PLC directory.
	DefaultPLCURL = "https://plc.directory"

	// DefaultTimeout is the default HTTP timeout for individual requests.
	DefaultTimeout = 60 * time.Second

	// DefaultRepoTimeout is the timeout for fetching large repos.
	DefaultRepoTimeout = 120 * time.Second
)

// Client handles HTTP requests to AT Protocol services.
type Client struct {
	httpClient     *http.Client
	repoHTTPClient *http.Client
	relayURL       string
	plcURL         string
	repoTimeout    time.Duration
}

// newTransport creates a connection-pooling HTTP transport with dynamic limits.
// The limits are scaled based on maxConcurrent to optimize for the workload.
func newTransport(maxConcurrent int) *http.Transport {
	// Scale pool limits based on max concurrent requests
	maxIdle := maxConcurrent * 2
	if maxIdle < 10 {
		maxIdle = 10
	}
	if maxIdle > 200 {
		maxIdle = 200
	}

	maxPerHost := maxConcurrent / 5
	if maxPerHost < 2 {
		maxPerHost = 2
	}
	if maxPerHost > 20 {
		maxPerHost = 20
	}

	maxConns := maxConcurrent / 3
	if maxConns < 4 {
		maxConns = 4
	}
	if maxConns > 30 {
		maxConns = 30
	}

	return &http.Transport{
		MaxIdleConns:        maxIdle,
		MaxIdleConnsPerHost: maxPerHost,
		MaxConnsPerHost:     maxConns,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	}
}

// defaultTransport is used when no maxConcurrent is specified.
var defaultTransport = newTransport(50)

// retryPolicy is a custom retry policy that only retries on specific status codes:
// - 429 (Too Many Requests) - rate limiting
// - 503 (Service Unavailable) - temporary outage
// - 504 (Gateway Timeout) - upstream timeout
func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// Don't retry on context cancellation
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	// Retry on connection errors (err is intentionally not propagated - retry handles it)
	if err != nil {
		return true, nil //nolint:nilerr // intentional: signal retry without propagating transient error
	}

	// Only retry on specific status codes
	switch resp.StatusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout:     // 504
		return true, nil
	}

	return false, nil
}

// leveledLogger adapts slog to retryablehttp's LeveledLogger interface.
type leveledLogger struct{}

func (l leveledLogger) Error(msg string, keysAndValues ...interface{}) {
	slog.Error("[backfill/http] "+msg, keysAndValues...)
}
func (l leveledLogger) Info(msg string, keysAndValues ...interface{}) {
	slog.Info("[backfill/http] "+msg, keysAndValues...)
}
func (l leveledLogger) Debug(msg string, keysAndValues ...interface{}) {
	slog.Debug("[backfill/http] "+msg, keysAndValues...)
}
func (l leveledLogger) Warn(msg string, keysAndValues ...interface{}) {
	slog.Warn("[backfill/http] "+msg, keysAndValues...)
}

// NewClient creates a new backfill client with connection pooling and retry support.
// The maxConcurrent parameter controls HTTP connection pool sizing (0 uses default of 50).
func NewClient(relayURL, plcURL string, maxConcurrent ...int) *Client {
	if relayURL == "" {
		relayURL = DefaultRelayURL
	}
	if plcURL == "" {
		plcURL = DefaultPLCURL
	}

	// Use provided maxConcurrent or default
	transport := defaultTransport
	if len(maxConcurrent) > 0 && maxConcurrent[0] > 0 {
		transport = newTransport(maxConcurrent[0])
	}

	return &Client{
		httpClient:     newRetryingHTTPClient(transport, DefaultTimeout),
		repoHTTPClient: newRetryingHTTPClient(transport, 0),
		relayURL:       relayURL,
		plcURL:         plcURL,
		repoTimeout:    DefaultRepoTimeout,
	}
}

func newRetryingHTTPClient(transport http.RoundTripper, timeout time.Duration) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 100 * time.Millisecond
	retryClient.RetryWaitMax = 2 * time.Second
	retryClient.CheckRetry = retryPolicy
	retryClient.Logger = leveledLogger{}
	retryClient.HTTPClient = &http.Client{Timeout: timeout, Transport: transport}
	return retryClient.StandardClient()
}

// RepoInfo contains basic repository information.
type RepoInfo struct {
	DID string `json:"did"`
}

// ListReposByCollectionResponse is the response from listReposByCollection.
type ListReposByCollectionResponse struct {
	Repos  []RepoInfo `json:"repos"`
	Cursor string     `json:"cursor,omitempty"`
}

// ListReposByCollection fetches all repos that have records for a collection.
func (c *Client) ListReposByCollection(ctx context.Context, collection string) ([]string, error) {
	var allRepos []string
	var cursor string

	for {
		repos, nextCursor, err := c.listReposByCollectionPage(ctx, collection, cursor)
		if err != nil {
			return nil, err
		}

		allRepos = append(allRepos, repos...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allRepos, nil
}

func (c *Client) listReposByCollectionPage(ctx context.Context, collection, cursor string) (repos []string, nextCursor string, err error) {
	u, err := url.Parse(c.relayURL + "/xrpc/com.atproto.sync.listReposByCollection")
	if err != nil {
		return nil, "", err
	}

	q := u.Query()
	q.Set("collection", collection)
	q.Set("limit", "1000")
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result ListReposByCollectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("failed to decode response: %w", err)
	}

	repos = make([]string, len(result.Repos))
	for i, r := range result.Repos {
		repos[i] = r.DID
	}

	return repos, result.Cursor, nil
}

// AtprotoData contains resolved DID information.
type AtprotoData struct {
	DID    string
	Handle string
	PDS    string
}

// ResolveDID resolves a DID to get PDS endpoint and handle.
func (c *Client) ResolveDID(ctx context.Context, did string) (*AtprotoData, error) {
	u := c.plcURL + "/" + did

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var doc PLCDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return doc.ToAtprotoData(did), nil
}

// PLCDocument represents a DID document from PLC directory.
type PLCDocument struct {
	Service     []PLCService `json:"service"`
	AlsoKnownAs []string     `json:"alsoKnownAs"`
}

// PLCService represents a service in the DID document.
type PLCService struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

// ToAtprotoData converts a PLC document to AtprotoData.
func (d *PLCDocument) ToAtprotoData(did string) *AtprotoData {
	data := &AtprotoData{
		DID: did,
		PDS: "https://bsky.social", // Default PDS
	}

	// Find AtprotoPersonalDataServer service
	for _, svc := range d.Service {
		if svc.Type == "AtprotoPersonalDataServer" {
			data.PDS = svc.ServiceEndpoint
			break
		}
	}

	// Find handle from alsoKnownAs
	for _, aka := range d.AlsoKnownAs {
		if len(aka) > 5 && aka[:5] == "at://" {
			data.Handle = aka[5:]
			break
		}
	}

	return data
}

// ListRecordsRecord represents a single record from listRecords.
type ListRecordsRecord struct {
	URI   string          `json:"uri"`
	CID   string          `json:"cid"`
	Value json.RawMessage `json:"value"`
}

// ListRecordsResponse is the response from listRecords.
type ListRecordsResponse struct {
	Records []ListRecordsRecord `json:"records"`
	Cursor  string              `json:"cursor,omitempty"`
}

// ListRecords fetches all records for a repo and collection from a PDS.
func (c *Client) ListRecords(ctx context.Context, pdsURL, repoDID, collection string) ([]ListRecordsRecord, error) {
	var allRecords []ListRecordsRecord
	var cursor string

	for {
		records, nextCursor, err := c.listRecordsPage(ctx, pdsURL, repoDID, collection, cursor)
		if err != nil {
			return nil, err
		}

		allRecords = append(allRecords, records...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allRecords, nil
}

func (c *Client) listRecordsPage(ctx context.Context, pdsURL, repoDID, collection, cursor string) ([]ListRecordsRecord, string, error) {
	u, err := url.Parse(pdsURL + "/xrpc/com.atproto.repo.listRecords")
	if err != nil {
		return nil, "", err
	}

	q := u.Query()
	q.Set("repo", repoDID)
	q.Set("collection", collection)
	q.Set("limit", "100")
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result ListRecordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Records, result.Cursor, nil
}

// CARFailureKind classifies whether CAR failure may use listRecords fallback.
type CARFailureKind string

const (
	CARFailureAvailability CARFailureKind = "availability"
	CARFailureUnsupported  CARFailureKind = "unsupported"
	CARFailureIntegrity    CARFailureKind = "integrity"
)

const maxCARErrorBodyBytes = 4 << 10

// CARFailure is a typed, bounded aggregate from fetching or extracting a CAR.
type CARFailure struct {
	Kind   CARFailureKind
	Total  int
	Errors []error
}

func (e *CARFailure) Error() string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		parts = append(parts, err.Error())
	}
	summary := strings.Join(parts, "; ")
	if e.Total > len(e.Errors) {
		summary += fmt.Sprintf("; and %d more", e.Total-len(e.Errors))
	}
	return fmt.Sprintf("CAR %s failure (%d): %s", e.Kind, e.Total, summary)
}

func (e *CARFailure) Unwrap() []error { return e.Errors }

func newCARFailure(kind CARFailureKind, errs ...error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return &CARFailure{Kind: kind, Total: len(filtered), Errors: filtered}
}

func carFailureKind(err error) (CARFailureKind, bool) {
	var failure *CARFailure
	if errors.As(err, &failure) {
		return failure.Kind, true
	}
	return "", false
}

func isCARFallbackFailure(err error) bool {
	kind, ok := carFailureKind(err)
	return ok && (kind == CARFailureAvailability || kind == CARFailureUnsupported)
}

func isCARIntegrityFailure(err error) bool {
	kind, ok := carFailureKind(err)
	return ok && kind == CARFailureIntegrity
}

func combineCARIntegrityFailures(errs ...error) error {
	combined := &CARFailure{Kind: CARFailureIntegrity}
	for _, err := range errs {
		if err == nil {
			continue
		}
		var failure *CARFailure
		if errors.As(err, &failure) && failure.Kind == CARFailureIntegrity {
			combined.Total += failure.Total
			for _, detail := range failure.Errors {
				if len(combined.Errors) < maxBackfillErrorDetails {
					combined.Errors = append(combined.Errors, detail)
				}
			}
			continue
		}
		combined.Total++
		if len(combined.Errors) < maxBackfillErrorDetails {
			combined.Errors = append(combined.Errors, err)
		}
	}
	if combined.Total == 0 {
		return nil
	}
	return combined
}

type boundedCARFailures struct {
	total int
	errs  []error
}

func (f *boundedCARFailures) add(err error) {
	if err == nil {
		return
	}
	f.total++
	if len(f.errs) < maxBackfillErrorDetails {
		f.errs = append(f.errs, err)
	}
}

func (f *boundedCARFailures) err() error {
	if f.total == 0 {
		return nil
	}
	return &CARFailure{Kind: CARFailureIntegrity, Total: f.total, Errors: f.errs}
}

// CARRecord represents a record extracted from a CAR file.
type CARRecord struct {
	URI        string
	CID        string
	Collection string
	RKey       string
	Value      []byte // CBOR bytes, will be converted to JSON
}

// GetRepo fetches an entire repo as a CAR file and extracts records.
// This is much more efficient than calling listRecords per collection.
// Uses a longer timeout (DefaultRepoTimeout) for large repos.
func (c *Client) GetRepo(ctx context.Context, pdsURL, did string, collections []string) ([]CARRecord, error) {
	parentCtx := ctx
	repoTimeout := c.repoTimeout
	if repoTimeout <= 0 {
		repoTimeout = DefaultRepoTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, repoTimeout)
	defer cancel()

	// Build collection filter set
	collectionSet := make(map[string]bool)
	for _, col := range collections {
		collectionSet[col] = true
	}

	// Fetch CAR file
	u := pdsURL + "/xrpc/com.atproto.sync.getRepo?did=" + url.QueryEscape(did)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Repo fetches use a retrying client without the shorter ordinary-request
	// timeout and rely on their dedicated context deadline instead. Both clients
	// share the same connection pool and retry policy.
	resp, err := c.repoHTTPClient.Do(req)
	if err != nil {
		if ok, contextErr := classifyRepoContextFailure(parentCtx, ctx, "fetch CAR", err); ok {
			return nil, contextErr
		}
		return nil, newCARFailure(CARFailureAvailability, fmt.Errorf("request failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxCARErrorBodyBytes))
		failure := fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		switch resp.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
			return nil, newCARFailure(CARFailureUnsupported, failure)
		default:
			return nil, newCARFailure(CARFailureAvailability, failure)
		}
	}

	// Read the CAR file using indigo's repo package
	r, err := repo.ReadRepoFromCar(ctx, resp.Body)
	if err != nil {
		if ok, contextErr := classifyRepoContextFailure(parentCtx, ctx, "read CAR body", err); ok {
			return nil, contextErr
		}
		return nil, newCARFailure(CARFailureIntegrity, fmt.Errorf("failed to parse CAR: %w", err))
	}

	// Extract every safe record while retaining a bounded aggregate of integrity
	// failures for invalid paths, missing blocks, and iteration failures.
	var records []CARRecord
	var failures boundedCARFailures

	err = r.ForEach(ctx, "", func(path string, recordCID cid.Cid) error {
		// Path format: "collection/rkey"
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			failures.add(fmt.Errorf("invalid record path %q", path))
			return nil
		}

		collection := parts[0]
		rkey := parts[1]

		// Filter by target collections
		if len(collectionSet) > 0 && !collectionSet[collection] {
			return nil // Skip collections we don't care about
		}

		// Get record bytes
		recCID, recordBytes, err := r.GetRecordBytes(ctx, path)
		if err != nil {
			failures.add(fmt.Errorf("read record block %s: %w", path, err))
			return nil
		}
		if recordBytes == nil {
			failures.add(fmt.Errorf("record block %s has no bytes", path))
			return nil
		}
		if !recordCID.Equals(recCID) {
			failures.add(fmt.Errorf("record block CID mismatch for %s: tree=%s block=%s", path, recordCID, recCID))
			return nil
		}

		records = append(records, CARRecord{
			URI:        "at://" + did + "/" + collection + "/" + rkey,
			CID:        recCID.String(),
			Collection: collection,
			RKey:       rkey,
			Value:      *recordBytes,
		})

		return nil
	})

	if err != nil {
		if ok, contextErr := classifyRepoContextFailure(parentCtx, ctx, "iterate CAR", err); ok {
			return records, contextErr
		}
		failures.add(fmt.Errorf("failed to iterate repo: %w", err))
	}

	return records, failures.err()
}

func classifyRepoContextFailure(parentCtx, repoCtx context.Context, operation string, err error) (bool, error) {
	if parentErr := parentCtx.Err(); parentErr != nil {
		return true, parentErr
	}
	if repoErr := repoCtx.Err(); repoErr != nil {
		return true, newCARFailure(CARFailureAvailability, fmt.Errorf("%s timed out: %w", operation, repoErr))
	}
	var timeoutErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeoutErr) && timeoutErr.Timeout()) {
		return true, newCARFailure(CARFailureAvailability, fmt.Errorf("%s timed out: %w", operation, err))
	}
	return false, nil
}

// CBORToJSON converts AT Protocol DAG-CBOR record bytes to their canonical JSON shape.
func CBORToJSON(data []byte) (string, error) {
	record, err := atdata.UnmarshalCBOR(data)
	if err != nil {
		return "", fmt.Errorf("failed to decode AT Protocol CBOR: %w", err)
	}

	jsonBytes, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("failed to encode JSON: %w", err)
	}

	return string(jsonBytes), nil
}
