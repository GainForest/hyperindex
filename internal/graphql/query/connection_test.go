// Package query provides GraphQL query type building.
package query

import (
	"testing"
)

// TestClampPageSize verifies that ClampPageSize returns a valid page size
// within [1, MaxPageSize], defaulting to DefaultPageSize for non-positive inputs.
func TestClampPageSize(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{
			name:  "zero returns default",
			input: 0,
			want:  DefaultPageSize,
		},
		{
			name:  "negative returns default",
			input: -1,
			want:  DefaultPageSize,
		},
		{
			name:  "large negative returns default",
			input: -100,
			want:  DefaultPageSize,
		},
		{
			name:  "one returns one",
			input: 1,
			want:  1,
		},
		{
			name:  "default page size returns default page size",
			input: DefaultPageSize,
			want:  DefaultPageSize,
		},
		{
			name:  "50 returns 50",
			input: 50,
			want:  50,
		},
		{
			name:  "max page size returns max page size",
			input: MaxPageSize,
			want:  MaxPageSize,
		},
		{
			name:  "one over max returns max",
			input: MaxPageSize + 1,
			want:  MaxPageSize,
		},
		{
			name:  "200 returns max",
			input: 200,
			want:  MaxPageSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClampPageSize(tt.input)
			if got != tt.want {
				t.Errorf("ClampPageSize(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
