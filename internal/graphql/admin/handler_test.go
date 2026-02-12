package admin

import (
	"context"
	"net/http"
	"testing"

	"github.com/graphql-go/graphql"
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

func TestValidAPIKey(t *testing.T) {
	tests := []struct {
		name        string
		adminAPIKey string
		authHeader  string
		want        bool
	}{
		{
			name:        "no key configured allows all",
			adminAPIKey: "",
			authHeader:  "",
			want:        true,
		},
		{
			name:        "valid key matches",
			adminAPIKey: "secret123",
			authHeader:  "Bearer secret123",
			want:        true,
		},
		{
			name:        "wrong key rejected",
			adminAPIKey: "secret123",
			authHeader:  "Bearer wrong",
			want:        false,
		},
		{
			name:        "missing auth header rejected",
			adminAPIKey: "secret123",
			authHeader:  "",
			want:        false,
		},
		{
			name:        "non-Bearer scheme rejected",
			adminAPIKey: "secret123",
			authHeader:  "Basic secret123",
			want:        false,
		},
		{
			name:        "Bearer prefix only rejected",
			adminAPIKey: "secret123",
			authHeader:  "Bearer ",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{adminAPIKey: tt.adminAPIKey}
			req, _ := http.NewRequest("POST", "/admin/graphql", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if got := h.validAPIKey(req); got != tt.want {
				t.Errorf("validAPIKey() = %v, want %v", got, tt.want)
			}
		})
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
