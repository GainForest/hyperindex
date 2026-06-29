// Package atproto provides shared utilities for AT Protocol record processing.
package atproto

import (
	"encoding/json"
	"time"
)

// TimestampFormats lists all timestamp formats recognized by ParseTimestamp,
// ordered from most specific to least specific.
var TimestampFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999Z07:00", // ISO with microseconds
	"2006-01-02T15:04:05.999999Z",      // ISO with microseconds, UTC literal
	"2006-01-02T15:04:05",              // ISO without timezone
	"2006-01-02 15:04:05.999999-07",    // PostgreSQL with microseconds and timezone
	"2006-01-02 15:04:05.999999+00",    // PostgreSQL with microseconds UTC
	"2006-01-02 15:04:05.999999",       // PostgreSQL with microseconds no TZ
	"2006-01-02 15:04:05-07",           // PostgreSQL with timezone
	"2006-01-02 15:04:05+00",           // PostgreSQL UTC
	"2006-01-02 15:04:05",              // SQLite format
}

// createdAtFields lists the JSON field names to check when extracting
// a creation timestamp from an AT Protocol record.
var createdAtFields = []string{
	"createdAt",
	"$createdAt",
	"created_at",
	"timestamp",
	"indexedAt",
}

// ParseTimestamp tries multiple common timestamp formats and returns the
// parsed time. Returns zero time if no format matches.
func ParseTimestamp(s string) time.Time {
	for _, format := range TimestampFormats {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ExtractCreatedAt extracts the createdAt timestamp from a record's JSON.
// It tries common field names (createdAt, $createdAt, created_at, timestamp,
// indexedAt) and multiple timestamp formats. Returns fallback if no timestamp
// is found or the JSON is invalid.
func ExtractCreatedAt(recordJSON string, fallback time.Time) time.Time {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(recordJSON), &data); err != nil {
		return fallback
	}

	for _, field := range createdAtFields {
		if val, ok := data[field].(string); ok {
			if t := ParseTimestamp(val); !t.IsZero() {
				return t
			}
		}
	}

	return fallback
}

// NormalizeRecordCreatedAt extracts a record JSON's top-level createdAt string
// and returns a millisecond-precision UTC timestamp for creation-time record
// timelines. Missing, non-string, malformed, or non-RFC3339 values return false
// so callers can store NULL and exclude the record from creation-time timelines.
func NormalizeRecordCreatedAt(recordJSON string) (string, bool) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(recordJSON), &data); err != nil {
		return "", false
	}

	raw, ok := data["createdAt"]
	if !ok {
		return "", false
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return "", false
	}

	createdAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", false
	}

	return createdAt.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z"), true
}
