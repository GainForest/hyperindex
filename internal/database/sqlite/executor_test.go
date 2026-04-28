package sqlite

import (
	"testing"

	"github.com/GainForest/hyperindex/internal/database"
)

func TestExecutor_Dialect(t *testing.T) {
	e := &Executor{}
	if got := e.Dialect(); got != database.SQLite {
		t.Errorf("Executor.Dialect() = %v, want %v", got, database.SQLite)
	}
}

func TestExecutor_Placeholder(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		index int
		want  string
	}{
		{1, "?"},
		{2, "?"},
		{10, "?"},
		{100, "?"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := e.Placeholder(tt.index)
			if got != tt.want {
				t.Errorf("Executor.Placeholder(%d) = %q, want %q", tt.index, got, tt.want)
			}
		})
	}
}

func TestExecutor_Placeholders(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		name       string
		count      int
		startIndex int
		want       string
	}{
		{
			name:       "zero count",
			count:      0,
			startIndex: 1,
			want:       "",
		},
		{
			name:       "negative count",
			count:      -1,
			startIndex: 1,
			want:       "",
		},
		{
			name:       "single placeholder",
			count:      1,
			startIndex: 1,
			want:       "?",
		},
		{
			name:       "multiple placeholders",
			count:      3,
			startIndex: 1,
			want:       "?, ?, ?",
		},
		{
			name:       "start index ignored",
			count:      3,
			startIndex: 5,
			want:       "?, ?, ?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Placeholders(tt.count, tt.startIndex)
			if got != tt.want {
				t.Errorf("Executor.Placeholders(%d, %d) = %q, want %q", tt.count, tt.startIndex, got, tt.want)
			}
		})
	}
}

func TestExecutor_JSONExtract(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		column string
		field  string
		want   string
	}{
		{"data", "name", "json_extract(data, '$.name')"},
		{"record", "type", "json_extract(record, '$.type')"},
		{"json_col", "nested_field", "json_extract(json_col, '$.nested_field')"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := e.JSONExtract(tt.column, tt.field)
			if got != tt.want {
				t.Errorf("Executor.JSONExtract(%q, %q) = %q, want %q", tt.column, tt.field, got, tt.want)
			}
		})
	}
}

func TestExecutor_JSONExtractPath(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		name   string
		column string
		path   []string
		want   string
	}{
		{
			name:   "empty path",
			column: "data",
			path:   []string{},
			want:   "json_extract(data, '$.')",
		},
		{
			name:   "single element path",
			column: "data",
			path:   []string{"name"},
			want:   "json_extract(data, '$.name')",
		},
		{
			name:   "two element path",
			column: "data",
			path:   []string{"user", "name"},
			want:   "json_extract(data, '$.user.name')",
		},
		{
			name:   "three element path",
			column: "data",
			path:   []string{"user", "profile", "avatar"},
			want:   "json_extract(data, '$.user.profile.avatar')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.JSONExtractPath(tt.column, tt.path)
			if got != tt.want {
				t.Errorf("Executor.JSONExtractPath(%q, %v) = %q, want %q", tt.column, tt.path, got, tt.want)
			}
		})
	}
}

func TestExecutor_Now(t *testing.T) {
	e := &Executor{}
	got := e.Now()
	if got != "datetime('now')" {
		t.Errorf("Executor.Now() = %q, want %q", got, "datetime('now')")
	}
}

func TestConvertParams(t *testing.T) {
	tests := []struct {
		name   string
		params []database.Value
		want   []any
	}{
		{
			name:   "nil params",
			params: nil,
			want:   nil,
		},
		{
			name:   "empty params",
			params: []database.Value{},
			want:   nil,
		},
		{
			name: "TextValue",
			params: []database.Value{
				database.TextValue("hello"),
			},
			want: []any{"hello"},
		},
		{
			name: "IntValue",
			params: []database.Value{
				database.IntValue(42),
			},
			want: []any{int64(42)},
		},
		{
			name: "FloatValue",
			params: []database.Value{
				database.FloatValue(3.14),
			},
			want: []any{float64(3.14)},
		},
		{
			name: "BoolValue true converts to 1",
			params: []database.Value{
				database.BoolValue(true),
			},
			want: []any{1},
		},
		{
			name: "BoolValue false converts to 0",
			params: []database.Value{
				database.BoolValue(false),
			},
			want: []any{0},
		},
		{
			name: "NullValue",
			params: []database.Value{
				database.NullValue{},
			},
			want: []any{nil},
		},
		{
			name: "BlobValue",
			params: []database.Value{
				database.BlobValue([]byte{1, 2, 3}),
			},
			want: []any{[]byte{1, 2, 3}},
		},
		{
			name: "TimestamptzValue",
			params: []database.Value{
				database.TimestamptzValue("2024-01-15T10:30:00Z"),
			},
			want: []any{"2024-01-15T10:30:00Z"},
		},
		{
			name: "mixed values",
			params: []database.Value{
				database.TextValue("name"),
				database.IntValue(42),
				database.BoolValue(true),
				database.NullValue{},
			},
			want: []any{"name", int64(42), 1, nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertParams(tt.params)

			if len(got) != len(tt.want) {
				t.Errorf("convertParams() length = %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				// Special handling for byte slices
				if gotBytes, ok := got[i].([]byte); ok {
					wantBytes, ok := tt.want[i].([]byte)
					if !ok {
						t.Errorf("convertParams()[%d] = %T, want %T", i, got[i], tt.want[i])
						continue
					}
					if string(gotBytes) != string(wantBytes) {
						t.Errorf("convertParams()[%d] = %v, want %v", i, gotBytes, wantBytes)
					}
					continue
				}

				if got[i] != tt.want[i] {
					t.Errorf("convertParams()[%d] = %v (%T), want %v (%T)", i, got[i], got[i], tt.want[i], tt.want[i])
				}
			}
		})
	}
}

func TestNewExecutor_MemoryDB(t *testing.T) {
	// Test creating an in-memory SQLite database
	e, err := NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}
	defer e.Close()

	// Verify dialect
	if e.Dialect() != database.SQLite {
		t.Errorf("Executor.Dialect() = %v, want %v", e.Dialect(), database.SQLite)
	}

	// Verify DB is not nil
	if e.DB() == nil {
		t.Error("Executor.DB() = nil, want non-nil")
	}

	// Verify we can ping
	if err := e.DB().Ping(); err != nil {
		t.Errorf("Executor.DB().Ping() error = %v", err)
	}
}

func TestNewExecutor_EmptyPath(t *testing.T) {
	// Test creating a database with empty path (should default to :memory:)
	e, err := NewExecutor("sqlite:")
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}
	defer e.Close()

	// Verify it works
	if err := e.DB().Ping(); err != nil {
		t.Errorf("Executor.DB().Ping() error = %v", err)
	}
}
