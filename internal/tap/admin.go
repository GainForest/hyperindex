// Package tap provides a client for the Tap sidecar HTTP and WebSocket APIs.
package tap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultHTTPTimeout is the timeout for all Tap admin HTTP requests.
	defaultHTTPTimeout = 30 * time.Second
)

// AdminClient communicates with Tap's HTTP API for repo management.
type AdminClient struct {
	baseURL  string // e.g., "http://localhost:2480"
	password string // Basic auth password (empty = no auth)
	client   *http.Client
}

// NewAdminClient creates a new Tap admin HTTP client.
func NewAdminClient(baseURL, password string) *AdminClient {
	return &AdminClient{
		baseURL:  baseURL,
		password: password,
		client: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

// RepoInfoResponse holds the response from GET /info/:did.
type RepoInfoResponse struct {
	DID    string `json:"did"`
	Status string `json:"status"`
	Rev    string `json:"rev,omitempty"`
}

// didsBody is the JSON body for /repos/add and /repos/remove.
type didsBody struct {
	DIDs []string `json:"dids"`
}

// healthResponse is the expected JSON body from GET /health.
type healthResponse struct {
	Status string `json:"status"`
}

// AddRepos adds DIDs to Tap's tracking list, triggering backfill.
// POST /repos/add with body {"dids": ["did:plc:abc", ...]}
func (c *AdminClient) AddRepos(ctx context.Context, dids []string) error {
	return c.postDIDs(ctx, "/repos/add", dids)
}

// RemoveRepos removes DIDs from Tap's tracking list.
// POST /repos/remove with body {"dids": ["did:plc:abc", ...]}
func (c *AdminClient) RemoveRepos(ctx context.Context, dids []string) error {
	return c.postDIDs(ctx, "/repos/remove", dids)
}

// Health checks if Tap is healthy.
// GET /health — expects {"status":"ok"}
func (c *AdminClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create health request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("health request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned HTTP %d", resp.StatusCode)
	}

	var body healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode health response: %w", err)
	}

	if body.Status != "ok" {
		return fmt.Errorf("tap reported unhealthy status: %q", body.Status)
	}

	return nil
}

// RepoInfo gets info about a tracked repo.
// GET /info/:did
func (c *AdminClient) RepoInfo(ctx context.Context, did string) (*RepoInfoResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/info/"+did, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create repo info request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("repo info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repo info returned HTTP %d", resp.StatusCode)
	}

	var info RepoInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode repo info response: %w", err)
	}

	return &info, nil
}

// postDIDs sends a POST request with a {"dids": [...]} body to the given path.
func (c *AdminClient) postDIDs(ctx context.Context, path string, dids []string) error {
	body, err := json.Marshal(didsBody{DIDs: dids})
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request to %s returned HTTP %d", path, resp.StatusCode)
	}

	return nil
}

// setAuth adds a Basic auth header if a password is configured.
// The username is always "admin".
func (c *AdminClient) setAuth(req *http.Request) {
	if c.password == "" {
		return
	}
	creds := base64.StdEncoding.EncodeToString([]byte("admin:" + c.password))
	req.Header.Set("Authorization", "Basic "+creds)
}
