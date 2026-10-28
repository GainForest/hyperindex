package schema

import (
	"fmt"
	"testing"

	"github.com/GainForest/hyperindex/internal/lexicon"
)

func TestUsesRecordCreatedAtColumn(t *testing.T) {
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
		if got := usesRecordCreatedAtColumn(registry, tt.collection, tt.validOnly); got != tt.want {
			t.Errorf("usesRecordCreatedAtColumn(%s, validOnly=%v) = %v, want %v", tt.collection, tt.validOnly, got, tt.want)
		}
	}
}
