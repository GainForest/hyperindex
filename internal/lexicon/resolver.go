// Package lexicon provides AT Protocol lexicon resolution utilities.
package lexicon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ResolvedLexicon contains the resolved lexicon schema and metadata.
type ResolvedLexicon struct {
	NSID   string          `json:"nsid"`
	DID    string          `json:"did"`
	PDSUrl string          `json:"pdsUrl"`
	Schema json.RawMessage `json:"schema"`
}

// Resolver resolves lexicon NSIDs to their schemas via DNS and PDS.
type Resolver struct {
	httpClient *http.Client
	plcURL     string
	dnsURL     string
}

// NewResolver creates a new lexicon resolver.
func NewResolver() *Resolver {
	return &Resolver{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		plcURL:     "https://plc.directory",
		dnsURL:     "https://dns.google/resolve",
	}
}

// ResolveLexicon resolves an NSID to its full lexicon schema.
// It performs DNS resolution to find the authority DID, then fetches the schema from the PDS.
func (r *Resolver) ResolveLexicon(ctx context.Context, nsid string) (*ResolvedLexicon, error) {
	// Validate NSID format
	parts := strings.Split(nsid, ".")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid NSID format: must have at least 3 segments")
	}

	// Resolve NSID to DID via DNS
	did, err := r.resolveNSIDToDID(ctx, nsid)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve NSID to DID: %w", err)
	}

	// Resolve DID to PDS URL
	pdsURL, err := r.resolvePDS(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve PDS: %w", err)
	}

	// Fetch lexicon schema from PDS
	schema, err := r.fetchLexiconSchema(ctx, pdsURL, did, nsid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch lexicon schema: %w", err)
	}

	return &ResolvedLexicon{
		NSID:   nsid,
		DID:    did,
		PDSUrl: pdsURL,
		Schema: schema,
	}, nil
}

// resolveNSIDToDID resolves an NSID to its authority DID via DNS TXT record.
// The authority is derived from the NSID by taking all segments except the last
// and reversing them to form a domain, then querying _lexicon.{domain}.
func (r *Resolver) resolveNSIDToDID(ctx context.Context, nsid string) (string, error) {
	parts := strings.Split(nsid, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid NSID: must have at least 3 segments")
	}

	// Authority is all segments except the last, reversed to form domain
	// e.g., "org.hypercerts.claim.activity" -> authority "org.hypercerts.claim" -> domain "claim.hypercerts.org"
	authority := parts[:len(parts)-1]
	reversed := make([]string, len(authority))
	for i, part := range authority {
		reversed[len(authority)-1-i] = part
	}
	domain := strings.Join(reversed, ".")

	// Query _lexicon.{domain} TXT record via DNS-over-HTTPS
	dnsQuery := fmt.Sprintf("_lexicon.%s", domain)

	reqURL, err := url.Parse(r.dnsURL)
	if err != nil {
		return "", err
	}
	q := reqURL.Query()
	q.Set("name", dnsQuery)
	q.Set("type", "TXT")
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return "", err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("DNS query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DNS query returned status %d", resp.StatusCode)
	}

	var dnsResp struct {
		Status int `json:"Status"`
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&dnsResp); err != nil {
		return "", fmt.Errorf("failed to decode DNS response: %w", err)
	}

	if dnsResp.Status == 3 || len(dnsResp.Answer) == 0 {
		return "", fmt.Errorf("no _lexicon TXT record found for %s", domain)
	}

	// Parse DID from TXT record (format: "did=did:plc:xxxxx")
	for _, answer := range dnsResp.Answer {
		txt := strings.Trim(answer.Data, "\"")
		if strings.HasPrefix(txt, "did=") {
			return strings.TrimPrefix(txt, "did="), nil
		}
	}

	return "", fmt.Errorf("no DID found in _lexicon TXT record for %s", domain)
}

// resolvePDS resolves a DID to its PDS URL via plc.directory.
func (r *Resolver) resolvePDS(ctx context.Context, did string) (string, error) {
	reqURL := fmt.Sprintf("%s/%s", r.plcURL, did)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("PLC directory query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PLC directory returned status %d", resp.StatusCode)
	}

	var doc struct {
		Service []struct {
			ID              string `json:"id"`
			Type            string `json:"type"`
			ServiceEndpoint string `json:"serviceEndpoint"`
		} `json:"service"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", fmt.Errorf("failed to decode DID document: %w", err)
	}

	// Find the #atproto_pds service
	for _, svc := range doc.Service {
		if svc.ID == "#atproto_pds" || svc.Type == "AtprotoPersonalDataServer" {
			return svc.ServiceEndpoint, nil
		}
	}

	return "", fmt.Errorf("no PDS service found in DID document")
}

// fetchLexiconSchema fetches a lexicon schema from a PDS.
func (r *Resolver) fetchLexiconSchema(ctx context.Context, pdsURL, did, nsid string) (json.RawMessage, error) {
	reqURL, err := url.Parse(fmt.Sprintf("%s/xrpc/com.atproto.repo.getRecord", pdsURL))
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	q.Set("repo", did)
	q.Set("collection", "com.atproto.lexicon.schema")
	q.Set("rkey", nsid)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PDS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("lexicon %s not found on PDS", nsid)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PDS returned status %d: %s", resp.StatusCode, string(body))
	}

	var record struct {
		Value json.RawMessage `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return nil, fmt.Errorf("failed to decode record: %w", err)
	}

	return record.Value, nil
}
