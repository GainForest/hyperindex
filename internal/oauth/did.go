// Package oauth provides AT Protocol OAuth implementation.
// DID resolution for did:plc and did:web methods.
package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// DefaultPLCDirectoryURL is the default PLC directory URL.
const DefaultPLCDirectoryURL = "https://plc.directory"

// DIDDocument represents a DID document.
type DIDDocument struct {
	ID          string    `json:"id"`
	AlsoKnownAs []string  `json:"alsoKnownAs,omitempty"`
	Service     []Service `json:"service,omitempty"`
}

// Service represents a service in a DID document.
type Service struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

// DIDResolver handles DID resolution.
type DIDResolver struct {
	plcDirectoryURL string
	httpClient      *http.Client
}

// DIDResolverOption configures the DID resolver.
type DIDResolverOption func(*DIDResolver)

// WithPLCDirectoryURL sets the PLC directory URL.
func WithPLCDirectoryURL(plcURL string) DIDResolverOption {
	return func(r *DIDResolver) {
		r.plcDirectoryURL = plcURL
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) DIDResolverOption {
	return func(r *DIDResolver) {
		r.httpClient = client
	}
}

// NewDIDResolver creates a new DID resolver.
func NewDIDResolver(opts ...DIDResolverOption) *DIDResolver {
	r := &DIDResolver{
		plcDirectoryURL: DefaultPLCDirectoryURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ResolveDID resolves a DID to its document.
// Supports did:plc and did:web methods.
func (r *DIDResolver) ResolveDID(did string) (*DIDDocument, error) {
	switch {
	case strings.HasPrefix(did, "did:plc:"):
		return r.resolvePLCDID(did)
	case strings.HasPrefix(did, "did:web:"):
		return r.resolveWebDID(did)
	default:
		return nil, fmt.Errorf("unsupported DID method: %s", did)
	}
}

// resolvePLCDID resolves a did:plc DID using the PLC directory.
func (r *DIDResolver) resolvePLCDID(did string) (*DIDDocument, error) {
	resolveURL := r.plcDirectoryURL + "/" + did

	resp, err := r.httpClient.Get(resolveURL)
	if err != nil {
		return nil, fmt.Errorf("PLC request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("DID not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PLC resolution failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseDIDDocument(body)
}

// resolveWebDID resolves a did:web DID.
func (r *DIDResolver) resolveWebDID(did string) (*DIDDocument, error) {
	// Extract domain from did:web:domain
	// Handle percent-encoded colons for ports (did:web:localhost%3A3000)
	domain := strings.TrimPrefix(did, "did:web:")
	domain, err := url.PathUnescape(domain)
	if err != nil {
		return nil, fmt.Errorf("invalid did:web encoding: %w", err)
	}

	// Build the .well-known URL
	didURL := "https://" + domain + "/.well-known/did.json"

	resp, err := r.httpClient.Get(didURL)
	if err != nil {
		return nil, fmt.Errorf("web DID request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("DID not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web DID resolution failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseDIDDocument(body)
}

// ResolveHandleToDID resolves a handle to a DID using the AT Protocol identity resolution.
func (r *DIDResolver) ResolveHandleToDID(handle string) (string, error) {
	// Use the standard AT Protocol handle resolution endpoint
	resolveURL := "https://bsky.social/xrpc/com.atproto.identity.resolveHandle?handle=" + url.QueryEscape(handle)

	resp, err := r.httpClient.Get(resolveURL)
	if err != nil {
		return "", fmt.Errorf("handle resolution request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("handle resolution failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		DID string `json:"did"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.DID == "" {
		return "", errors.New("no DID in response")
	}

	return result.DID, nil
}

// GetPDSEndpoint extracts the PDS endpoint from a DID document.
func (doc *DIDDocument) GetPDSEndpoint() string {
	for _, svc := range doc.Service {
		if svc.Type == "AtprotoPersonalDataServer" {
			return svc.ServiceEndpoint
		}
	}
	return ""
}

// GetHandle extracts the handle from a DID document's alsoKnownAs field.
// Returns the first at:// URI stripped of the at:// prefix.
func (doc *DIDDocument) GetHandle() string {
	for _, aka := range doc.AlsoKnownAs {
		if strings.HasPrefix(aka, "at://") {
			return strings.TrimPrefix(aka, "at://")
		}
	}
	return ""
}

// parseDIDDocument parses a DID document from JSON.
func parseDIDDocument(data []byte) (*DIDDocument, error) {
	var doc DIDDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse DID document: %w", err)
	}
	if doc.ID == "" {
		return nil, errors.New("invalid DID document: missing id")
	}
	return &doc, nil
}

// IsValidDID checks whether a string is a syntactically valid did:plc or did:web identifier.
func IsValidDID(raw string) bool {
	did, err := syntax.ParseDID(raw)
	if err != nil {
		return false
	}
	method := did.Method()
	return method == "plc" || method == "web"
}

// IsDIDPLC checks if a DID uses the plc method.
func IsDIDPLC(did string) bool {
	return strings.HasPrefix(did, "did:plc:")
}

// IsDIDWeb checks if a DID uses the web method.
func IsDIDWeb(did string) bool {
	return strings.HasPrefix(did, "did:web:")
}
