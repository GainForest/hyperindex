package schema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

func createdAtRegistry(t *testing.T) *lexicon.Registry {
	t.Helper()
	lexiconWith := func(id, createdAt, required string) string {
		return fmt.Sprintf(`{"lexicon":1,"id":%q,"defs":{"main":{"type":"record","key":"tid","record":{"type":"object","required":[%s],"properties":{"createdAt":%s}}}}}`, id, required, createdAt)
	}
	registry := lexicon.NewRegistry()
	for _, raw := range []string{
		lexiconWith("test.required", `{"type":"string","format":"datetime"}`, `"createdAt"`),
		lexiconWith("test.optional", `{"type":"string","format":"datetime"}`, ``),
		lexiconWith("test.plainstring", `{"type":"string"}`, `"createdAt"`),
	} {
		lex, err := lexicon.Parse(raw)
		if err != nil {
			t.Fatalf("parse lexicon: %v", err)
		}
		registry.Register(lex)
	}
	return registry
}

// Sorting keeps rows whose column is NULL, so it only needs a datetime
// createdAt; required and validated reads are not part of the condition.
func TestSortsByRecordCreatedAtColumn(t *testing.T) {
	registry := createdAtRegistry(t)
	tests := []struct {
		collection string
		want       bool
	}{
		{"test.required", true},
		{"test.optional", true},
		{"test.plainstring", false},
		{"test.unknown", false},
	}
	for _, tt := range tests {
		if got := sortsByRecordCreatedAtColumn(registry, tt.collection); got != tt.want {
			t.Errorf("sortsByRecordCreatedAtColumn(%s) = %v, want %v", tt.collection, got, tt.want)
		}
	}
	if sortsByRecordCreatedAtColumn(nil, "test.required") {
		t.Error("sortsByRecordCreatedAtColumn(nil registry) = true, want false")
	}
}

// Filters drop rows whose column is NULL, so they keep PR #142's narrower
// condition: required datetime and a validated read.
func TestFiltersByRecordCreatedAtColumn(t *testing.T) {
	registry := createdAtRegistry(t)
	tests := []struct {
		collection string
		validOnly  bool
		want       bool
	}{
		{"test.required", true, true},
		// Unvalidated reads can include rows whose createdAt never parsed.
		{"test.required", false, false},
		// Valid rows may omit an optional createdAt; the column would drop them.
		{"test.optional", true, false},
		{"test.plainstring", true, false},
		{"test.unknown", true, false},
	}
	for _, tt := range tests {
		if got := filtersByRecordCreatedAtColumn(registry, tt.collection, tt.validOnly); got != tt.want {
			t.Errorf("filtersByRecordCreatedAtColumn(%s, validOnly=%v) = %v, want %v", tt.collection, tt.validOnly, got, tt.want)
		}
	}
}

func TestNormalizeCreatedAtCursor(t *testing.T) {
	column := &repositories.SortOption{Field: repositories.SortFieldRecordCreatedAt, Direction: "DESC"}

	t.Run("raw JSON value is normalized to the stored form", func(t *testing.T) {
		values := []string{"2026-01-02T12:00:00.000500+02:00", "at://did:plc:a/test/1"}
		if err := normalizeCreatedAtCursor(column, values); err != nil {
			t.Fatalf("normalizeCreatedAtCursor() error = %v", err)
		}
		if values[0] != "2026-01-02T10:00:00.000Z" {
			t.Fatalf("cursor value = %q, want normalized UTC milliseconds", values[0])
		}
	})

	t.Run("empty value marks a record without record_created_at", func(t *testing.T) {
		values := []string{"", "at://did:plc:a/test/1"}
		if err := normalizeCreatedAtCursor(column, values); err != nil {
			t.Fatalf("normalizeCreatedAtCursor() error = %v", err)
		}
		if values[0] != "" {
			t.Fatalf("cursor value = %q, want it left empty", values[0])
		}
	})

	t.Run("non-datetime value asks the client to restart", func(t *testing.T) {
		err := normalizeCreatedAtCursor(column, []string{"not-a-date", "at://did:plc:a/test/1"})
		if err == nil || !strings.Contains(err.Error(), "restart pagination") {
			t.Fatalf("normalizeCreatedAtCursor() error = %v, want restart pagination guidance", err)
		}
	})

	t.Run("other sorts are untouched", func(t *testing.T) {
		values := []string{"not-a-date", "at://did:plc:a/test/1"}
		if err := normalizeCreatedAtCursor(&repositories.SortOption{Field: "title"}, values); err != nil || values[0] != "not-a-date" {
			t.Fatalf("normalizeCreatedAtCursor(title) = %v, %q; want untouched", err, values[0])
		}
	})
}
