// Package backfill provides historical data fetching from AT Protocol relays and PDS servers.
package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	httpClient *http.Client
	relayURL   string
	plcURL     string
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

	// Create retryable client with exponential backoff
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 100 * time.Millisecond
	retryClient.RetryWaitMax = 2 * time.Second
	retryClient.CheckRetry = retryPolicy
	retryClient.Logger = leveledLogger{}
	retryClient.HTTPClient = &http.Client{
		Timeout:   DefaultTimeout,
		Transport: transport,
	}

	return &Client{
		httpClient: retryClient.StandardClient(),
		relayURL:   relayURL,
		plcURL:     plcURL,
	}
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
		DID:    did,
		Handle: did,                   // Default to DID if no handle found
		PDS:    "https://bsky.social", // Default PDS
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
	// Use longer timeout for repo fetches (large repos can take a while)
	ctx, cancel := context.WithTimeout(ctx, DefaultRepoTimeout)
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Read the CAR file using indigo's repo package
	r, err := repo.ReadRepoFromCar(ctx, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CAR: %w", err)
	}

	// Extract records from the repo
	var records []CARRecord

	err = r.ForEach(ctx, "", func(path string, recordCid cid.Cid) error {
		// Path format: "collection/rkey"
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			return nil // Skip invalid paths
		}

		collection := parts[0]
		rkey := parts[1]

		// Filter by target collections
		if len(collectionSet) > 0 && !collectionSet[collection] {
			return nil // Skip collections we don't care about
		}

		// Get record bytes
		recCid, recordBytes, err := r.GetRecordBytes(ctx, path)
		if err != nil {
			return nil //nolint:nilerr // skip unreadable records, continue iteration
		}
		if recordBytes == nil {
			return nil
		}

		records = append(records, CARRecord{
			URI:        "at://" + did + "/" + collection + "/" + rkey,
			CID:        recCid.String(),
			Collection: collection,
			RKey:       rkey,
			Value:      *recordBytes,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate repo: %w", err)
	}

	return records, nil
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
