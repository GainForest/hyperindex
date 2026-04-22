package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/database/sqlite"
)

// Helper to check if enum has a value with the given name
func enumHasValue(enum *graphql.Enum, name string) bool {
	for _, v := range enum.Values() {
		if v.Name == name {
			return true
		}
	}
	return false
}

func newTestAdminHandler(t *testing.T, adminDIDs, adminAPIKey string) *Handler {
	t.Helper()

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite executor: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Close()
	})

	if _, err := exec.DB().Exec(`CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("failed to create config table: %v", err)
	}

	if _, err := exec.DB().Exec(`INSERT INTO config (key, value) VALUES ('admin_dids', ?)`, adminDIDs); err != nil {
		t.Fatalf("failed to seed admin_dids config: %v", err)
	}

	configRepo := repositories.NewConfigRepository(exec)
	handler, err := NewHandler(&Repositories{}, nil, configRepo, "did:web:example.com", adminAPIKey)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	return handler
}

type currentSessionResponse struct {
	Data struct {
		CurrentSession *struct {
			DID     string `json:"did"`
			Handle  string `json:"handle"`
			IsAdmin bool   `json:"isAdmin"`
		} `json:"currentSession"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func decodeCurrentSessionResponse(t *testing.T, rr *httptest.ResponseRecorder) *currentSessionResponse {
	t.Helper()

	var payload currentSessionResponse

	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	return &payload
}

func TestIsValidBearerToken(t *testing.T) {
	t.Parallel()

	const expected = "super-secret-key"

	tests := []struct {
		name                string
		authorizationHeader string
		expectedToken       string
		want                bool
	}{
		{name: "valid bearer token", authorizationHeader: "Bearer super-secret-key", expectedToken: expected, want: true},
		{name: "wrong token", authorizationHeader: "Bearer wrong-key", expectedToken: expected, want: false},
		{name: "missing header", authorizationHeader: "", expectedToken: expected, want: false},
		{name: "wrong scheme", authorizationHeader: "Token super-secret-key", expectedToken: expected, want: false},
		{name: "empty bearer value", authorizationHeader: "Bearer ", expectedToken: expected, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isValidBearerToken(tt.authorizationHeader, tt.expectedToken)
			if got != tt.want {
				t.Fatalf("isValidBearerToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandlerServeHTTP_AuthViaXUserDIDWithValidBearer(t *testing.T) {
	handler := newTestAdminHandler(t, "did:plc:admin1", "super-secret-key")

	body := bytes.NewBufferString(`{"query":"{ currentSession { did handle isAdmin } }"}`)
	req := httptest.NewRequest(http.MethodPost, "/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer super-secret-key")
	req.Header.Set("X-User-DID", "did:plc:admin1")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	payload := decodeCurrentSessionResponse(t, rr)
	if len(payload.Errors) != 0 {
		t.Fatalf("unexpected GraphQL errors: %+v", payload.Errors)
	}
	if payload.Data.CurrentSession == nil {
		t.Fatal("expected currentSession data, got nil")
	}
	if got := payload.Data.CurrentSession.DID; got != "did:plc:admin1" {
		t.Fatalf("did = %q, want %q", got, "did:plc:admin1")
	}
	if got := payload.Data.CurrentSession.Handle; got != "" {
		t.Fatalf("handle = %q, want empty string", got)
	}
	if !payload.Data.CurrentSession.IsAdmin {
		t.Fatal("expected isAdmin to be true")
	}
}

func TestHandlerServeHTTP_IgnoresXUserDIDWithoutValidBearer(t *testing.T) {
	handler := newTestAdminHandler(t, "did:plc:admin1", "super-secret-key")

	tests := []struct {
		name string
		auth string
	}{
		{name: "wrong bearer token", auth: "Bearer wrong-key"},
		{name: "missing bearer token", auth: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBufferString(`{"query":"{ currentSession { did handle isAdmin } }"}`)
			req := httptest.NewRequest(http.MethodPost, "/graphql", body)
			req.Header.Set("Content-Type", "application/json")
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			req.Header.Set("X-User-DID", "did:plc:admin1")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}

			payload := decodeCurrentSessionResponse(t, rr)
			if len(payload.Errors) != 0 {
				t.Fatalf("unexpected GraphQL errors: %+v", payload.Errors)
			}
			if payload.Data.CurrentSession != nil {
				t.Fatalf("expected currentSession to be nil, got %+v", payload.Data.CurrentSession)
			}
		})
	}
}

func TestEnumTypes(t *testing.T) {
	// Test TimeRangeEnum
	if TimeRangeEnum == nil {
		t.Error("TimeRangeEnum is nil")
	}

	expectedTimeRanges := []string{"ONE_HOUR", "THREE_HOURS", "SIX_HOURS", "ONE_DAY", "SEVEN_DAYS"}
	for _, name := range expectedTimeRanges {
		if !enumHasValue(TimeRangeEnum, name) {
			t.Errorf("Expected TimeRange value %s not found", name)
		}
	}

	// Test LabelSeverityEnum
	if LabelSeverityEnum == nil {
		t.Error("LabelSeverityEnum is nil")
	}

	expectedSeverities := []string{"INFORM", "ALERT", "TAKEDOWN"}
	for _, name := range expectedSeverities {
		if !enumHasValue(LabelSeverityEnum, name) {
			t.Errorf("Expected LabelSeverity value %s not found", name)
		}
	}

	// Test LabelVisibilityEnum
	if LabelVisibilityEnum == nil {
		t.Error("LabelVisibilityEnum is nil")
	}

	expectedVisibilities := []string{"IGNORE", "SHOW", "WARN", "HIDE"}
	for _, name := range expectedVisibilities {
		if !enumHasValue(LabelVisibilityEnum, name) {
			t.Errorf("Expected LabelVisibility value %s not found", name)
		}
	}

	// Test ReportReasonTypeEnum
	if ReportReasonTypeEnum == nil {
		t.Error("ReportReasonTypeEnum is nil")
	}

	expectedReasons := []string{"SPAM", "VIOLATION", "MISLEADING", "SEXUAL", "RUDE", "OTHER"}
	for _, name := range expectedReasons {
		if !enumHasValue(ReportReasonTypeEnum, name) {
			t.Errorf("Expected ReportReasonType value %s not found", name)
		}
	}

	// Test ReportStatusEnum
	if ReportStatusEnum == nil {
		t.Error("ReportStatusEnum is nil")
	}

	expectedStatuses := []string{"PENDING", "RESOLVED", "DISMISSED"}
	for _, name := range expectedStatuses {
		if !enumHasValue(ReportStatusEnum, name) {
			t.Errorf("Expected ReportStatus value %s not found", name)
		}
	}

	// Test ReportActionEnum
	if ReportActionEnum == nil {
		t.Error("ReportActionEnum is nil")
	}

	expectedActions := []string{"APPLY_LABEL", "DISMISS"}
	for _, name := range expectedActions {
		if !enumHasValue(ReportActionEnum, name) {
			t.Errorf("Expected ReportAction value %s not found", name)
		}
	}
}

func TestObjectTypes(t *testing.T) {
	// Test that all object types are defined correctly
	types := []struct {
		name string
		obj  interface{}
	}{
		{"StatisticsType", StatisticsType},
		{"CurrentSessionType", CurrentSessionType},
		{"SettingsType", SettingsType},
		{"PurgePreviewType", PurgePreviewType},
		{"ActivityBucketType", ActivityBucketType},
		{"ActivityEntryType", ActivityEntryType},
		{"LexiconType", LexiconType},
		{"OAuthClientType", OAuthClientType},
		{"LabelDefinitionType", LabelDefinitionType},
		{"LabelPreferenceType", LabelPreferenceType},
		{"LabelType", LabelType},
		{"ReportType", ReportType},
		{"PageInfoType", PageInfoType},
		{"LabelEdgeType", LabelEdgeType},
		{"LabelConnectionType", LabelConnectionType},
		{"ReportEdgeType", ReportEdgeType},
		{"ReportConnectionType", ReportConnectionType},
	}

	for _, tc := range types {
		if tc.obj == nil {
			t.Errorf("%s is nil", tc.name)
		}
	}
}

func TestStatisticsTypeFields(t *testing.T) {
	expectedFields := []string{"recordCount", "actorCount", "lexiconCount"}

	fields := StatisticsType.Fields()
	for _, name := range expectedFields {
		if fields[name] == nil {
			t.Errorf("Expected field %s not found in StatisticsType", name)
		}
	}
}

func TestSettingsTypeFields(t *testing.T) {
	expectedFields := []string{
		"id",
		"domainAuthority",
		"adminDids",
		"relayUrl",
		"plcDirectoryUrl",
		"jetstreamUrl",
		"oauthSupportedScopes",
	}

	fields := SettingsType.Fields()
	for _, name := range expectedFields {
		if fields[name] == nil {
			t.Errorf("Expected field %s not found in SettingsType", name)
		}
	}
}

func TestLabelTypeFields(t *testing.T) {
	expectedFields := []string{"id", "src", "uri", "cid", "val", "neg", "cts", "exp"}

	fields := LabelType.Fields()
	for _, name := range expectedFields {
		if fields[name] == nil {
			t.Errorf("Expected field %s not found in LabelType", name)
		}
	}
}

func TestReportTypeFields(t *testing.T) {
	expectedFields := []string{
		"id",
		"reporterDid",
		"subjectUri",
		"reasonType",
		"reason",
		"status",
		"resolvedBy",
		"resolvedAt",
		"createdAt",
	}

	fields := ReportType.Fields()
	for _, name := range expectedFields {
		if fields[name] == nil {
			t.Errorf("Expected field %s not found in ReportType", name)
		}
	}
}

func TestContextWithAuth(t *testing.T) {
	ctx := context.Background()

	// Add auth info
	ctx = ContextWithAuth(ctx, "did:plc:user123", "user.handle", true, []string{"did:plc:admin1", "did:plc:admin2"})

	// Verify values
	userDID := ctx.Value(contextKeyUserDID).(string)
	if userDID != "did:plc:user123" {
		t.Errorf("Expected userDID to be 'did:plc:user123', got '%s'", userDID)
	}

	handle := ctx.Value(contextKeyHandle).(string)
	if handle != "user.handle" {
		t.Errorf("Expected handle to be 'user.handle', got '%s'", handle)
	}

	isAdmin := ctx.Value(contextKeyIsAdmin).(bool)
	if !isAdmin {
		t.Error("Expected isAdmin to be true")
	}

	adminDIDs := ctx.Value(contextKeyAdminDIDs).([]string)
	if len(adminDIDs) != 2 {
		t.Errorf("Expected 2 admin DIDs, got %d", len(adminDIDs))
	}
}

func TestRequireAdmin(t *testing.T) {
	// Test with admin context
	adminCtx := ContextWithAuth(context.Background(), "did:plc:admin", "admin.handle", true, nil)
	if err := requireAdmin(adminCtx); err != nil {
		t.Errorf("Expected no error for admin, got %v", err)
	}

	// Test with non-admin context
	userCtx := ContextWithAuth(context.Background(), "did:plc:user", "user.handle", false, nil)
	if err := requireAdmin(userCtx); err == nil {
		t.Error("Expected error for non-admin, got nil")
	}

	// Test with empty context
	emptyCtx := context.Background()
	if err := requireAdmin(emptyCtx); err == nil {
		t.Error("Expected error for empty context, got nil")
	}
}

func TestContextKeysAreUnique(t *testing.T) {
	// Ensure context keys are unique
	keys := []contextKey{
		contextKeyUserDID,
		contextKeyHandle,
		contextKeyIsAdmin,
		contextKeyAdminDIDs,
	}

	seen := make(map[contextKey]bool)
	for _, key := range keys {
		if seen[key] {
			t.Errorf("Duplicate context key: %v", key)
		}
		seen[key] = true
	}
}
