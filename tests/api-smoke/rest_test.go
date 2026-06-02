//go:build api_smoke

package apismoke

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	config := loadSmokeConfig(t)
	smokeLog("Checking deployed Hyperindex API: %s", config.baseURL)
	response := getRESTObject(t, context.Background(), config, "/health")

	status, ok := response["status"]
	if !ok {
		t.Fatal("REST /health: missing status field")
	}
	if status != "ok" {
		t.Fatalf("REST /health: status = %v, want ok", status)
	}

	if healthTime, ok := response["time"]; ok {
		if _, ok := healthTime.(string); !ok {
			t.Fatalf("REST /health: time field type = %T, want string", healthTime)
		}
	}

	smokeLog("✓ Health endpoint is OK")
}

func TestReadyEndpoint(t *testing.T) {
	config := loadSmokeConfig(t)
	response := getRESTObject(t, context.Background(), config, "/ready")

	status, ok := response["status"]
	if !ok {
		t.Fatal("REST /ready: missing status field")
	}
	if status != "ok" {
		t.Fatalf("REST /ready: status = %v, want ok", status)
	}

	smokeLog("✓ Ready endpoint is OK")
}

func TestStatsEndpoint(t *testing.T) {
	config := loadSmokeConfig(t)
	response := getRESTObject(t, context.Background(), config, "/stats")

	records := requireNonNegativeIntegerField(t, response, "/stats", "records")
	actors := requireNonNegativeIntegerField(t, response, "/stats", "actors")
	lexicons := requireNonNegativeIntegerField(t, response, "/stats", "lexicons")

	if records < 1 {
		t.Fatalf("REST /stats: records = %v, want at least 1", records)
	}
	if actors < 1 {
		t.Fatalf("REST /stats: actors = %v, want at least 1", actors)
	}

	smokeLog("✓ Stats endpoint returned indexed data: records=%.0f actors=%.0f lexicons=%.0f", records, actors, lexicons)
}

func getRESTObject(t testing.TB, ctx context.Context, config smokeConfig, path string) map[string]any {
	t.Helper()

	url := config.baseURL + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("REST %s: build request: %v", path, err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := config.httpClient.Do(request)
	if err != nil {
		t.Fatalf("REST %s: request failed: %v", path, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("REST %s: read HTTP %d response: %v", path, response.StatusCode, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("REST %s: HTTP %d; response %q", path, response.StatusCode, responseSnippet(body))
	}

	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("REST %s: decode HTTP %d response: %v; response %q", path, response.StatusCode, err, responseSnippet(body))
	}
	if decoded == nil {
		t.Fatalf("REST %s: decode HTTP %d response: expected JSON object; response %q", path, response.StatusCode, responseSnippet(body))
	}
	if config.debug {
		smokeLog("REST method=%s path=%s status=%d bodyBytes=%d", http.MethodGet, path, response.StatusCode, len(body))
	}

	return decoded
}

func requireNonNegativeIntegerField(t testing.TB, response map[string]any, path string, field string) float64 {
	t.Helper()

	value, ok := response[field]
	if !ok {
		t.Fatalf("REST %s: missing %s field", path, field)
	}

	number, ok := value.(json.Number)
	if !ok {
		t.Fatalf("REST %s: %s field type = %T, want JSON number", path, field, value)
	}

	parsed, err := number.Float64()
	if err != nil {
		t.Fatalf("REST %s: %s field value %q is not a valid JSON number: %v", path, field, number, err)
	}
	if parsed < 0 {
		t.Fatalf("REST %s: %s = %v, want non-negative", path, field, parsed)
	}
	if math.Trunc(parsed) != parsed {
		t.Fatalf("REST %s: %s = %v, want integer-compatible number", path, field, parsed)
	}

	return parsed
}
