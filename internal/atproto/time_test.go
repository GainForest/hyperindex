package atproto

import (
	"testing"
	"time"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantZero bool
		wantYear int // Check year/month/day/hour/min to confirm parse worked
		wantMon  time.Month
		wantDay  int
		wantHour int
	}{
		{
			name: "RFC3339", input: "2024-01-15T10:30:00Z",
			wantYear: 2024, wantMon: 1, wantDay: 15, wantHour: 10,
		},
		{
			name: "RFC3339 with offset", input: "2024-01-15T10:30:00+05:00",
			wantYear: 2024, wantMon: 1, wantDay: 15, wantHour: 10,
		},
		{
			name: "RFC3339Nano", input: "2024-01-15T10:30:00.123456789Z",
			wantYear: 2024, wantMon: 1, wantDay: 15, wantHour: 10,
		},
		{
			name: "ISO without timezone", input: "2024-01-15T10:30:00",
			wantYear: 2024, wantMon: 1, wantDay: 15, wantHour: 10,
		},
		{
			name: "SQLite format", input: "2024-01-15 10:30:00",
			wantYear: 2024, wantMon: 1, wantDay: 15, wantHour: 10,
		},
		{
			name: "PostgreSQL with microseconds", input: "2024-01-15 10:30:00.123456",
			wantYear: 2024, wantMon: 1, wantDay: 15, wantHour: 10,
		},
		{name: "empty string", input: "", wantZero: true},
		{name: "invalid", input: "not-a-timestamp", wantZero: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTimestamp(tt.input)
			if tt.wantZero {
				if !got.IsZero() {
					t.Errorf("ParseTimestamp(%q) = %v, want zero time", tt.input, got)
				}
				return
			}
			if got.IsZero() {
				t.Fatalf("ParseTimestamp(%q) returned zero time", tt.input)
			}
			if got.Year() != tt.wantYear || got.Month() != tt.wantMon || got.Day() != tt.wantDay || got.Hour() != tt.wantHour {
				t.Errorf("ParseTimestamp(%q) = %v, want %d-%02d-%02dT%02d:*",
					tt.input, got, tt.wantYear, tt.wantMon, tt.wantDay, tt.wantHour)
			}
		})
	}
}

func TestNormalizeRecordCreatedAt(t *testing.T) {
	tests := []struct {
		name       string
		recordJSON string
		want       string
		wantOK     bool
	}{
		{
			name:       "createdAt field normalizes to UTC milliseconds",
			recordJSON: `{"createdAt":"2026-01-15T10:30:00.123456789+05:30"}`,
			want:       "2026-01-15T05:00:00.123Z",
			wantOK:     true,
		},
		{
			name:       "createdAt without fractional seconds",
			recordJSON: `{"createdAt":"2026-01-15T10:30:00Z"}`,
			want:       "2026-01-15T10:30:00.000Z",
			wantOK:     true,
		},
		{
			name:       "createdAt missing timezone is rejected",
			recordJSON: `{"createdAt":"2026-01-15T10:30:00"}`,
		},
		{
			name:       "createdAt with unknown UTC offset is rejected",
			recordJSON: `{"createdAt":"2026-01-15T10:30:00-00:00"}`,
		},
		{
			name:       "non-string createdAt is rejected",
			recordJSON: `{"createdAt":12345}`,
		},
		{
			name:       "missing createdAt is rejected",
			recordJSON: `{"indexedAt":"2026-01-15T10:30:00Z"}`,
		},
		{
			name:       "invalid JSON is rejected",
			recordJSON: `{invalid`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeRecordCreatedAt(tt.recordJSON)
			if ok != tt.wantOK {
				t.Fatalf("NormalizeRecordCreatedAt() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("NormalizeRecordCreatedAt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractCreatedAt(t *testing.T) {
	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		recordJSON string
		want       time.Time
	}{
		{
			name:       "createdAt field",
			recordJSON: `{"createdAt": "2024-01-15T10:30:00Z", "title": "test"}`,
			want:       time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:       "$createdAt field",
			recordJSON: `{"$createdAt": "2024-06-01T12:00:00Z"}`,
			want:       time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name:       "timestamp field",
			recordJSON: `{"timestamp": "2024-03-20T08:00:00Z"}`,
			want:       time.Date(2024, 3, 20, 8, 0, 0, 0, time.UTC),
		},
		{
			name:       "no timestamp field",
			recordJSON: `{"title": "no time here"}`,
			want:       fallback,
		},
		{
			name:       "invalid JSON",
			recordJSON: `{invalid`,
			want:       fallback,
		},
		{
			name:       "empty JSON",
			recordJSON: `{}`,
			want:       fallback,
		},
		{
			name:       "non-string timestamp",
			recordJSON: `{"createdAt": 12345}`,
			want:       fallback,
		},
		{
			name:       "unparseable timestamp string",
			recordJSON: `{"createdAt": "not-a-time"}`,
			want:       fallback,
		},
		{
			name:       "ISO without timezone",
			recordJSON: `{"createdAt": "2024-01-15T10:30:00"}`,
			want:       time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCreatedAt(tt.recordJSON, fallback)
			if !got.Equal(tt.want) {
				t.Errorf("ExtractCreatedAt(%q, fallback) = %v, want %v", tt.recordJSON, got, tt.want)
			}
		})
	}
}
