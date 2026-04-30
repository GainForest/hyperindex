//go:build api_smoke || !api_smoke

package apismoke

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	smokeURLEnv          = "HYPERINDEX_SMOKE_URL"
	smokeExpectationsEnv = "HYPERINDEX_SMOKE_EXPECTATIONS"
)

type smokeConfig struct {
	baseURL      string
	expectations expectations
	httpClient   *http.Client
}

type expectations struct {
	RequiredNSIDs          []string                 `json:"requiredNSIDs"`
	TypedQueryFields       map[string]string        `json:"typedQueryFields"`
	NonRecordNSIDs         []string                 `json:"nonRecordNSIDs"`
	DataBearingCollections []dataBearingExpectation `json:"dataBearingCollections"`
	PaginationCollections  []paginationExpectation  `json:"paginationCollections"`
	Search                 searchExpectation        `json:"search"`
}

type dataBearingExpectation struct {
	NSID           string `json:"nsid"`
	MinimumRecords int    `json:"minimumRecords"`
}

type paginationExpectation struct {
	NSID     string `json:"nsid"`
	PageSize int    `json:"pageSize"`
}

type searchExpectation struct {
	Query string `json:"query"`
	First int    `json:"first"`
}

func loadSmokeConfig(t testing.TB) smokeConfig {
	t.Helper()

	baseURL, err := parseSmokeBaseURL(os.Getenv(smokeURLEnv))
	if err != nil {
		t.Fatalf("load smoke config: %v", err)
	}

	expectationsPath := os.Getenv(smokeExpectationsEnv)
	if expectationsPath == "" {
		expectationsPath = defaultExpectationsPath(t)
	}

	loadedExpectations, err := loadExpectations(expectationsPath)
	if err != nil {
		t.Fatalf("load smoke config: %v", err)
	}

	return smokeConfig{
		baseURL:      baseURL,
		expectations: loadedExpectations,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func parseSmokeBaseURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("%s is required and must point to the public Hyperindex API", smokeURLEnv)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", smokeURLEnv, err)
	}
	if !parsedURL.IsAbs() || parsedURL.Host == "" {
		return "", fmt.Errorf("%s must be an absolute http or https URL", smokeURLEnv)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("%s must use http or https, got %q", smokeURLEnv, parsedURL.Scheme)
	}

	return strings.TrimRight(parsedURL.String(), "/"), nil
}

func defaultExpectationsPath(t testing.TB) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("load smoke config: cannot locate api smoke package directory")
	}

	return filepath.Join(filepath.Dir(currentFile), "expectations.json")
}

func loadExpectations(path string) (expectations, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return expectations{}, fmt.Errorf("read expectations file %q: %w", path, err)
	}

	var loaded expectations
	if err := json.Unmarshal(content, &loaded); err != nil {
		return expectations{}, fmt.Errorf("decode expectations file %q: %w", path, err)
	}
	if err := loaded.validate(); err != nil {
		return expectations{}, fmt.Errorf("validate expectations file %q: %w", path, err)
	}

	return loaded, nil
}

func (e expectations) validate() error {
	requiredNSIDs := makeSet(e.RequiredNSIDs)
	nonRecordNSIDs := makeSet(e.NonRecordNSIDs)

	if len(requiredNSIDs) == 0 {
		return fmt.Errorf("requiredNSIDs must include at least one NSID")
	}
	for nsid, queryField := range e.TypedQueryFields {
		if nsid == "" || queryField == "" {
			return fmt.Errorf("typedQueryFields must not contain empty NSIDs or field names")
		}
		if nonRecordNSIDs[nsid] {
			return fmt.Errorf("typedQueryFields must exclude non-record NSID %q", nsid)
		}
		if !requiredNSIDs[nsid] {
			return fmt.Errorf("typedQueryFields contains %q, which is missing from requiredNSIDs", nsid)
		}
	}

	dataBearingNSIDs := make(map[string]bool, len(e.DataBearingCollections))
	for _, collection := range e.DataBearingCollections {
		if collection.NSID == "" {
			return fmt.Errorf("dataBearingCollections must not contain an empty NSID")
		}
		if collection.MinimumRecords < 1 {
			return fmt.Errorf("data-bearing collection %q must expect at least one record", collection.NSID)
		}
		if !requiredNSIDs[collection.NSID] {
			return fmt.Errorf("data-bearing collection %q is missing from requiredNSIDs", collection.NSID)
		}
		if e.TypedQueryFields[collection.NSID] == "" {
			return fmt.Errorf("data-bearing collection %q is missing from typedQueryFields", collection.NSID)
		}
		if nonRecordNSIDs[collection.NSID] {
			return fmt.Errorf("data-bearing collection %q cannot be listed in nonRecordNSIDs", collection.NSID)
		}
		dataBearingNSIDs[collection.NSID] = true
	}

	for _, collection := range e.PaginationCollections {
		if collection.NSID == "" {
			return fmt.Errorf("paginationCollections must not contain an empty NSID")
		}
		if collection.PageSize < 1 {
			return fmt.Errorf("pagination collection %q must use a positive pageSize", collection.NSID)
		}
		if !dataBearingNSIDs[collection.NSID] {
			return fmt.Errorf("pagination collection %q must also be listed in dataBearingCollections", collection.NSID)
		}
	}

	if e.Search.Query == "" {
		return fmt.Errorf("search.query is required")
	}
	if e.Search.First < 1 {
		return fmt.Errorf("search.first must be positive")
	}

	return nil
}

func makeSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func TestConfig(t *testing.T) {
	t.Setenv(smokeURLEnv, "http://127.0.0.1:1/")
	t.Setenv(smokeExpectationsEnv, "")

	config := loadSmokeConfig(t)
	if config.baseURL != "http://127.0.0.1:1" {
		t.Fatalf("baseURL = %q, want trailing slash trimmed", config.baseURL)
	}
	if config.httpClient == nil || config.httpClient.Timeout != 10*time.Second {
		t.Fatalf("httpClient timeout = %v, want 10s", config.httpClient)
	}
	if len(config.expectations.RequiredNSIDs) != 21 {
		t.Fatalf("required NSID count = %d, want 21", len(config.expectations.RequiredNSIDs))
	}
}

func TestExpectationsValidationRejectsDataBearingCollectionMissingTypedField(t *testing.T) {
	loaded, err := loadExpectations(defaultExpectationsPath(t))
	if err != nil {
		t.Fatal(err)
	}
	delete(loaded.TypedQueryFields, "org.hypercerts.claim.activity")

	err = loaded.validate()
	if err == nil || !strings.Contains(err.Error(), "missing from typedQueryFields") {
		t.Fatalf("validate() error = %v, want missing typedQueryFields error", err)
	}
}

func TestExpectationsValidationRejectsDataBearingCollectionMissingRequiredNSID(t *testing.T) {
	loaded, err := loadExpectations(defaultExpectationsPath(t))
	if err != nil {
		t.Fatal(err)
	}
	loaded.RequiredNSIDs = removeString(loaded.RequiredNSIDs, "org.hypercerts.claim.activity")

	err = loaded.validate()
	if err == nil || !strings.Contains(err.Error(), "missing from requiredNSIDs") {
		t.Fatalf("validate() error = %v, want missing requiredNSIDs error", err)
	}
}

func removeString(values []string, unwanted string) []string {
	kept := values[:0]
	for _, value := range values {
		if value != unwanted {
			kept = append(kept, value)
		}
	}
	return kept
}
