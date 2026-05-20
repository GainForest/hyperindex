//go:build api_smoke

package apismoke

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	smokeWriteThroughEnv         = "HYPERINDEX_SMOKE_WRITE_THROUGH"
	smokeWritePDSURLEnv          = "HYPERINDEX_SMOKE_ATPROTO_PDS_URL"
	smokeWriteIdentifierEnv      = "HYPERINDEX_SMOKE_ATPROTO_IDENTIFIER"
	smokeWritePasswordEnv        = "HYPERINDEX_SMOKE_ATPROTO_PASSWORD"
	smokeWritePollTimeoutEnv     = "HYPERINDEX_SMOKE_WRITE_POLL_TIMEOUT"
	smokeWritePollIntervalEnv    = "HYPERINDEX_SMOKE_WRITE_POLL_INTERVAL"
	smokeWriteDefaultPollTimeout = 60 * time.Second
	smokeWriteDefaultPollEvery   = 2 * time.Second

	profileCollection  = "app.certified.actor.profile"
	activityCollection = "org.hypercerts.claim.activity"

	writeThroughMarker = "Hyperindex write-through smoke test"
)

type writeThroughConfig struct {
	pdsURL       string
	identifier   string
	password     string
	pollTimeout  time.Duration
	pollInterval time.Duration
}

type writeRecordSpec struct {
	collection  string
	typedField  string
	rkey        string
	queryFields []string
}

type atprotoClient struct {
	baseURL    string
	accessJWT  string
	httpClient *http.Client
}

type atprotoSession struct {
	DID       string `json:"did"`
	Handle    string `json:"handle"`
	AccessJWT string `json:"accessJwt"`
}

type atprotoRecordRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type atprotoFetchedRecord struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

type xrpcError struct {
	procedure  string
	statusCode int
	errorName  string
	message    string
	body       string
}

func (e *xrpcError) Error() string {
	details := strings.TrimSpace(e.message)
	if details == "" {
		details = strings.TrimSpace(e.body)
	}
	if e.errorName != "" && details != "" {
		details = e.errorName + ": " + details
	} else if e.errorName != "" {
		details = e.errorName
	}
	if details == "" {
		return fmt.Sprintf("%s returned HTTP %d", e.procedure, e.statusCode)
	}
	return fmt.Sprintf("%s returned HTTP %d: %s", e.procedure, e.statusCode, details)
}

func TestWriteThroughRecordLifecycle(t *testing.T) {
	loadSmokeDotEnv(t)
	if !smokeWriteThroughEnabled(t) {
		return
	}

	config := loadSmokeConfig(t)
	writeConfig := loadWriteThroughConfig(t)
	client := &atprotoClient{
		baseURL:    writeConfig.pdsURL,
		httpClient: config.httpClient,
	}

	ctx := context.Background()
	session, err := client.createSession(ctx, writeConfig.identifier, writeConfig.password)
	if err != nil {
		t.Fatalf("write-through smoke: create ATProto session: %v", err)
	}
	if session.DID == "" || session.AccessJWT == "" {
		t.Fatalf("write-through smoke: create ATProto session returned did=%q accessJwt set=%t", session.DID, session.AccessJWT != "")
	}
	client.accessJWT = session.AccessJWT

	runID := "smoke-" + time.Now().UTC().Format("20060102T150405Z") + "-" + randomHex(t, 4)
	createdAt := time.Now().UTC().Format(time.RFC3339)
	profileSpec := writeRecordSpec{
		collection:  profileCollection,
		typedField:  "appCertifiedActorProfile",
		rkey:        "self",
		queryFields: []string{"displayName", "description", "createdAt"},
	}
	activitySpec := writeRecordSpec{
		collection:  activityCollection,
		typedField:  "orgHypercertsClaimActivity",
		rkey:        runID,
		queryFields: []string{"title", "shortDescription", "createdAt"},
	}

	createdProfile := false
	createdActivity := false
	var originalProfile map[string]any
	t.Cleanup(func() {
		if createdActivity {
			cleanupSmokeRecord(t, client, session.DID, activitySpec.collection, activitySpec.rkey)
		}
		if createdProfile {
			cleanupSmokeRecord(t, client, session.DID, profileSpec.collection, profileSpec.rkey)
		}
		if originalProfile != nil {
			restoreSmokeProfile(t, client, session.DID, originalProfile)
		}
	})

	lifecycleStart := time.Now()
	smokeLog("Running write-through smoke checks with ATProto DID %s", session.DID)
	originalProfile = prepareSmokeProfileSlot(t, client, session.DID)

	profileCreate := map[string]any{
		"$type":       profileCollection,
		"displayName": "Hyperindex Smoke Profile " + runID,
		"description": writeThroughMarker + " create " + runID,
		"createdAt":   createdAt,
	}
	profileCreateRef, err := client.createRecord(ctx, session.DID, profileSpec.collection, profileSpec.rkey, profileCreate)
	if err != nil {
		t.Fatalf("write-through smoke: create %s/%s: %v", profileSpec.collection, profileSpec.rkey, err)
	}
	createdProfile = true
	profileCreateDuration, profileCreatePolls := waitForIndexedRecord(t, config, profileSpec, profileCreateRef.URI, profileCreateRef.CID, map[string]string{
		"displayName": stringField(profileCreate, "displayName"),
		"description": stringField(profileCreate, "description"),
		"createdAt":   createdAt,
	}, writeConfig)
	smokeLog("✓ %s create ingested in %s (%d polls)", profileSpec.collection, formatSmokeDuration(profileCreateDuration), profileCreatePolls)

	profileUpdate := map[string]any{
		"$type":       profileCollection,
		"displayName": "Hyperindex Smoke Profile Updated " + runID,
		"description": writeThroughMarker + " update " + runID,
		"createdAt":   createdAt,
	}
	profileUpdateRef, err := client.putRecord(ctx, session.DID, profileSpec.collection, profileSpec.rkey, profileUpdate)
	if err != nil {
		t.Fatalf("write-through smoke: update %s/%s: %v", profileSpec.collection, profileSpec.rkey, err)
	}
	profileUpdateDuration, profileUpdatePolls := waitForIndexedRecord(t, config, profileSpec, profileUpdateRef.URI, profileUpdateRef.CID, map[string]string{
		"displayName": stringField(profileUpdate, "displayName"),
		"description": stringField(profileUpdate, "description"),
		"createdAt":   createdAt,
	}, writeConfig)
	smokeLog("✓ %s update ingested in %s (%d polls)", profileSpec.collection, formatSmokeDuration(profileUpdateDuration), profileUpdatePolls)

	activityCreate := map[string]any{
		"$type":            activityCollection,
		"title":            "Hyperindex smoke activity " + runID,
		"shortDescription": writeThroughMarker + " activity create " + runID,
		"createdAt":        createdAt,
	}
	activityCreateRef, err := client.createRecord(ctx, session.DID, activitySpec.collection, activitySpec.rkey, activityCreate)
	if err != nil {
		t.Fatalf("write-through smoke: create %s/%s: %v", activitySpec.collection, activitySpec.rkey, err)
	}
	createdActivity = true
	activityCreateDuration, activityCreatePolls := waitForIndexedRecord(t, config, activitySpec, activityCreateRef.URI, activityCreateRef.CID, map[string]string{
		"title":            stringField(activityCreate, "title"),
		"shortDescription": stringField(activityCreate, "shortDescription"),
		"createdAt":        createdAt,
	}, writeConfig)
	smokeLog("✓ %s create ingested in %s (%d polls)", activitySpec.collection, formatSmokeDuration(activityCreateDuration), activityCreatePolls)

	activityUpdate := map[string]any{
		"$type":            activityCollection,
		"title":            "Hyperindex smoke activity updated " + runID,
		"shortDescription": writeThroughMarker + " activity update " + runID,
		"createdAt":        createdAt,
	}
	activityUpdateRef, err := client.putRecord(ctx, session.DID, activitySpec.collection, activitySpec.rkey, activityUpdate)
	if err != nil {
		t.Fatalf("write-through smoke: update %s/%s: %v", activitySpec.collection, activitySpec.rkey, err)
	}
	activityUpdateDuration, activityUpdatePolls := waitForIndexedRecord(t, config, activitySpec, activityUpdateRef.URI, activityUpdateRef.CID, map[string]string{
		"title":            stringField(activityUpdate, "title"),
		"shortDescription": stringField(activityUpdate, "shortDescription"),
		"createdAt":        createdAt,
	}, writeConfig)
	smokeLog("✓ %s update ingested in %s (%d polls)", activitySpec.collection, formatSmokeDuration(activityUpdateDuration), activityUpdatePolls)

	if err := client.deleteRecord(ctx, session.DID, activitySpec.collection, activitySpec.rkey); err != nil {
		t.Fatalf("write-through smoke: delete %s/%s: %v", activitySpec.collection, activitySpec.rkey, err)
	}
	createdActivity = false
	activityDeleteDuration, activityDeletePolls := waitForIndexedRecordDeleted(t, config, activitySpec, activityUpdateRef.URI, writeConfig)
	smokeLog("✓ %s delete ingested in %s (%d polls)", activitySpec.collection, formatSmokeDuration(activityDeleteDuration), activityDeletePolls)

	if err := client.deleteRecord(ctx, session.DID, profileSpec.collection, profileSpec.rkey); err != nil {
		t.Fatalf("write-through smoke: delete %s/%s: %v", profileSpec.collection, profileSpec.rkey, err)
	}
	createdProfile = false
	profileDeleteDuration, profileDeletePolls := waitForIndexedRecordDeleted(t, config, profileSpec, profileUpdateRef.URI, writeConfig)
	smokeLog("✓ %s delete ingested in %s (%d polls)", profileSpec.collection, formatSmokeDuration(profileDeleteDuration), profileDeletePolls)
	smokeLog("✓ Write-through smoke lifecycle completed in %s", formatSmokeDuration(time.Since(lifecycleStart)))
}

func smokeWriteThroughEnabled(t testing.TB) bool {
	t.Helper()

	enabled, ok := parseSmokeBool(os.Getenv(smokeWriteThroughEnv))
	if !ok {
		t.Fatalf("%s must be a boolean value such as 1, true, 0, or false", smokeWriteThroughEnv)
	}
	return enabled
}

func parseSmokeBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "0", "false", "no", "off":
		return false, true
	case "1", "true", "yes", "on":
		return true, true
	default:
		return false, false
	}
}

func loadWriteThroughConfig(t testing.TB) writeThroughConfig {
	t.Helper()

	return writeThroughConfig{
		pdsURL:       parseRequiredHTTPURL(t, smokeWritePDSURLEnv),
		identifier:   requiredSmokeEnv(t, smokeWriteIdentifierEnv),
		password:     requiredSmokeEnv(t, smokeWritePasswordEnv),
		pollTimeout:  smokeDurationEnv(t, smokeWritePollTimeoutEnv, smokeWriteDefaultPollTimeout),
		pollInterval: smokeDurationEnv(t, smokeWritePollIntervalEnv, smokeWriteDefaultPollEvery),
	}
}

func requiredSmokeEnv(t testing.TB, name string) string {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required when %s=1", name, smokeWriteThroughEnv)
	}
	return value
}

func parseRequiredHTTPURL(t testing.TB, name string) string {
	t.Helper()

	rawValue := requiredSmokeEnv(t, name)
	parsedURL, err := url.Parse(rawValue)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	if !parsedURL.IsAbs() || parsedURL.Host == "" {
		t.Fatalf("%s must be an absolute http or https URL", name)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		t.Fatalf("%s must use http or https, got %q", name, parsedURL.Scheme)
	}
	return strings.TrimRight(parsedURL.String(), "/")
}

func smokeDurationEnv(t testing.TB, name string, fallback time.Duration) time.Duration {
	t.Helper()

	rawValue := strings.TrimSpace(os.Getenv(name))
	if rawValue == "" {
		return fallback
	}
	duration, err := time.ParseDuration(rawValue)
	if err != nil {
		t.Fatalf("parse %s duration %q: %v", name, rawValue, err)
	}
	if duration <= 0 {
		t.Fatalf("%s duration must be positive, got %s", name, duration)
	}
	return duration
}

func waitForIndexedRecord(t testing.TB, config smokeConfig, spec writeRecordSpec, uri string, cid string, fields map[string]string, writeConfig writeThroughConfig) (time.Duration, int) {
	t.Helper()

	startedAt := time.Now()
	deadline := startedAt.Add(writeConfig.pollTimeout)
	attempts := 0
	var lastObservation string
	for {
		attempts++
		record := fetchTypedWriteRecord(t, context.Background(), config, spec, uri)
		if recordMatches(record, uri, cid, fields) {
			return time.Since(startedAt), attempts
		}
		lastObservation = describeIndexedRecord(record)

		if !sleepBeforeNextPoll(deadline, writeConfig.pollInterval) {
			break
		}
	}

	t.Fatalf("timed out after %s waiting for %s(%q) to have cid=%q fields=%s; last observation: %s", writeConfig.pollTimeout, spec.typedField+"ByUri", uri, cid, formatExpectedFields(fields), lastObservation)
	return 0, attempts
}

func waitForIndexedRecordDeleted(t testing.TB, config smokeConfig, spec writeRecordSpec, uri string, writeConfig writeThroughConfig) (time.Duration, int) {
	t.Helper()

	startedAt := time.Now()
	deadline := startedAt.Add(writeConfig.pollTimeout)
	attempts := 0
	var lastObservation string
	for {
		attempts++
		record := fetchTypedWriteRecord(t, context.Background(), config, spec, uri)
		if record == nil {
			return time.Since(startedAt), attempts
		}
		lastObservation = describeIndexedRecord(record)

		if !sleepBeforeNextPoll(deadline, writeConfig.pollInterval) {
			break
		}
	}

	t.Fatalf("timed out after %s waiting for %s(%q) to be deleted; last observation: %s", writeConfig.pollTimeout, spec.typedField+"ByUri", uri, lastObservation)
	return 0, attempts
}

func sleepBeforeNextPoll(deadline time.Time, interval time.Duration) bool {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return false
	}
	if interval > remaining {
		interval = remaining
	}
	time.Sleep(interval)
	return true
}

func formatSmokeDuration(duration time.Duration) string {
	if duration >= time.Millisecond {
		return duration.Round(time.Millisecond).String()
	}
	return duration.String()
}

func fetchTypedWriteRecord(t testing.TB, ctx context.Context, config smokeConfig, spec writeRecordSpec, uri string) map[string]any {
	t.Helper()

	fieldName := spec.typedField + "ByUri"
	query := fmt.Sprintf(`
query WriteThroughSmokeByUri($uri: String!) {
  %s(uri: $uri) {
    uri
    cid
    did
    rkey
    %s
  }
}`, fieldName, strings.Join(spec.queryFields, "\n    "))

	response := postGraphQL(t, ctx, config, "WriteThroughSmokeByUri", query, map[string]any{
		"uri": uri,
	})

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode WriteThroughSmokeByUri data for %s(%q): %v", fieldName, uri, err)
	}

	rawRecord, ok := decoded[fieldName]
	if !ok || bytes.Equal(bytes.TrimSpace(rawRecord), []byte("null")) {
		return nil
	}

	var record map[string]any
	if err := json.Unmarshal(rawRecord, &record); err != nil {
		t.Fatalf("decode WriteThroughSmokeByUri record for %s(%q): %v", fieldName, uri, err)
	}
	return record
}

func recordMatches(record map[string]any, uri string, cid string, fields map[string]string) bool {
	if record == nil {
		return false
	}
	if stringValue(record["uri"]) != uri || stringValue(record["cid"]) != cid {
		return false
	}
	for field, want := range fields {
		if stringValue(record[field]) != want {
			return false
		}
	}
	return true
}

func describeIndexedRecord(record map[string]any) string {
	if record == nil {
		return "null"
	}
	fields := make([]string, 0, len(record))
	for key, value := range record {
		fields = append(fields, fmt.Sprintf("%s=%q", key, stringValue(value)))
	}
	sort.Strings(fields)
	return strings.Join(fields, " ")
}

func formatExpectedFields(fields map[string]string) string {
	parts := make([]string, 0, len(fields))
	for key, value := range fields {
		parts = append(parts, fmt.Sprintf("%s=%q", key, value))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func prepareSmokeProfileSlot(t testing.TB, client *atprotoClient, repo string) map[string]any {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existing, err := client.getRecord(ctx, repo, profileCollection, "self")
	if err != nil {
		t.Fatalf("write-through smoke: inspect existing %s/self: %v", profileCollection, err)
	}
	if existing == nil {
		return nil
	}

	var originalProfile map[string]any
	if looksLikeSmokeRecord(existing.Value) {
		smokeLog("Removing stale write-through smoke %s/self before run", profileCollection)
	} else {
		smokeLog("Temporarily replacing existing %s/self; it will be restored after the smoke run", profileCollection)
		originalProfile = cloneRecordValue(existing.Value)
	}
	if err := client.deleteRecord(ctx, repo, profileCollection, "self"); err != nil && !isXRPCNotFound(err) {
		t.Fatalf("write-through smoke: delete existing %s/self before run: %v", profileCollection, err)
	}
	return originalProfile
}

func looksLikeSmokeRecord(value map[string]any) bool {
	for _, field := range []string{"displayName", "description", "title", "shortDescription"} {
		if strings.Contains(stringValue(value[field]), writeThroughMarker) {
			return true
		}
	}
	return strings.HasPrefix(stringValue(value["displayName"]), "Hyperindex Smoke Profile") || strings.HasPrefix(stringValue(value["title"]), "Hyperindex smoke activity")
}

func cleanupSmokeRecord(t testing.TB, client *atprotoClient, repo string, collection string, rkey string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.deleteRecord(ctx, repo, collection, rkey); err != nil && !isXRPCNotFound(err) {
		t.Logf("write-through smoke cleanup: delete %s/%s: %v", collection, rkey, err)
	}
}

func restoreSmokeProfile(t testing.TB, client *atprotoClient, repo string, originalProfile map[string]any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.putRecord(ctx, repo, profileCollection, "self", originalProfile); err != nil {
		t.Errorf("write-through smoke cleanup: restore original %s/self: %v", profileCollection, err)
		return
	}
	smokeLog("✓ Restored original %s/self", profileCollection)
}

func cloneRecordValue(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func (c *atprotoClient) createSession(ctx context.Context, identifier string, password string) (*atprotoSession, error) {
	var session atprotoSession
	if err := c.doXRPC(ctx, http.MethodPost, "com.atproto.server.createSession", nil, map[string]any{
		"identifier": identifier,
		"password":   password,
	}, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *atprotoClient) createRecord(ctx context.Context, repo string, collection string, rkey string, record map[string]any) (atprotoRecordRef, error) {
	var ref atprotoRecordRef
	err := c.doXRPC(ctx, http.MethodPost, "com.atproto.repo.createRecord", nil, map[string]any{
		"repo":       repo,
		"collection": collection,
		"rkey":       rkey,
		"validate":   false,
		"record":     record,
	}, &ref)
	return ref, err
}

func (c *atprotoClient) putRecord(ctx context.Context, repo string, collection string, rkey string, record map[string]any) (atprotoRecordRef, error) {
	var ref atprotoRecordRef
	err := c.doXRPC(ctx, http.MethodPost, "com.atproto.repo.putRecord", nil, map[string]any{
		"repo":       repo,
		"collection": collection,
		"rkey":       rkey,
		"validate":   false,
		"record":     record,
	}, &ref)
	return ref, err
}

func (c *atprotoClient) deleteRecord(ctx context.Context, repo string, collection string, rkey string) error {
	err := c.doXRPC(ctx, http.MethodPost, "com.atproto.repo.deleteRecord", nil, map[string]any{
		"repo":       repo,
		"collection": collection,
		"rkey":       rkey,
	}, nil)
	if isXRPCNotFound(err) {
		return nil
	}
	return err
}

func (c *atprotoClient) getRecord(ctx context.Context, repo string, collection string, rkey string) (*atprotoFetchedRecord, error) {
	query := url.Values{}
	query.Set("repo", repo)
	query.Set("collection", collection)
	query.Set("rkey", rkey)

	var record atprotoFetchedRecord
	if err := c.doXRPC(ctx, http.MethodGet, "com.atproto.repo.getRecord", query, nil, &record); err != nil {
		if isXRPCNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (c *atprotoClient) doXRPC(ctx context.Context, method string, procedure string, query url.Values, input any, output any) error {
	endpoint := c.baseURL + "/xrpc/" + procedure
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("marshal %s request: %w", procedure, err)
		}
		body = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", procedure, err)
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.accessJWT != "" {
		request.Header.Set("Authorization", "Bearer "+c.accessJWT)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send %s request: %w", procedure, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", procedure, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return newXRPCError(procedure, response.StatusCode, responseBody)
	}
	if output == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode %s response: %w; response %q", procedure, err, responseSnippet(responseBody))
	}
	return nil
}

func newXRPCError(procedure string, statusCode int, body []byte) error {
	var decoded struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &decoded)
	return &xrpcError{
		procedure:  procedure,
		statusCode: statusCode,
		errorName:  decoded.Error,
		message:    decoded.Message,
		body:       responseSnippet(body),
	}
}

func isXRPCNotFound(err error) bool {
	if err == nil {
		return false
	}
	var xerr *xrpcError
	if !errors.As(err, &xerr) {
		return false
	}
	if xerr.statusCode == http.StatusNotFound {
		return true
	}
	errorName := strings.ToLower(xerr.errorName)
	message := strings.ToLower(xerr.message + " " + xerr.body)
	return strings.Contains(errorName, "notfound") || strings.Contains(errorName, "not_found") || strings.Contains(message, "not found") || strings.Contains(message, "recordnotfound")
}

func stringField(record map[string]any, key string) string {
	return stringValue(record[key])
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	text, ok := value.(string)
	if ok {
		return text
	}
	return fmt.Sprint(value)
}

func randomHex(t testing.TB, byteCount int) string {
	t.Helper()

	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("generate smoke test suffix: %v", err)
	}
	return hex.EncodeToString(buffer)
}
