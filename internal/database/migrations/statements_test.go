package migrations

import (
	"reflect"
	"testing"
)

func TestRunsOutsideTransaction(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{"marker first line", "-- hyperindex:no-transaction\nCREATE INDEX CONCURRENTLY i ON t (c);", true},
		{"marker after blank lines", "\n\n  -- hyperindex:no-transaction  \nDROP INDEX CONCURRENTLY i;", true},
		{"other comment first", "-- explanation\n-- hyperindex:no-transaction\nDROP INDEX CONCURRENTLY i;", false},
		{"marker mentioned in statement", "SELECT 1; -- hyperindex:no-transaction", false},
		{"no marker", "CREATE INDEX i ON t (c);", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runsOutsideTransaction(tt.sql); got != tt.want {
				t.Fatalf("runsOutsideTransaction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "comments dropped and statements split",
			sql: "-- hyperindex:no-transaction\n" +
				"-- a note; with a semicolon\n" +
				"CREATE INDEX CONCURRENTLY IF NOT EXISTS a ON t (c); /* inline; */\n\n" +
				"DROP INDEX CONCURRENTLY IF EXISTS b;\n",
			want: []string{
				"CREATE INDEX CONCURRENTLY IF NOT EXISTS a ON t (c)",
				"DROP INDEX CONCURRENTLY IF EXISTS b",
			},
		},
		{
			name: "semicolons and comment markers inside quotes",
			sql:  `INSERT INTO t VALUES ('a;b', 'it''s -- not a comment'); SELECT "odd;name" FROM t;`,
			want: []string{
				`INSERT INTO t VALUES ('a;b', 'it''s -- not a comment')`,
				`SELECT "odd;name" FROM t`,
			},
		},
		{
			name: "dollar quoted body",
			sql:  "DO $$ BEGIN PERFORM 1; PERFORM 2; END $$; DO $tag$ SELECT ';' $tag$;",
			want: []string{
				"DO $$ BEGIN PERFORM 1; PERFORM 2; END $$",
				"DO $tag$ SELECT ';' $tag$",
			},
		},
		{
			name: "final statement without semicolon and comment-only input",
			sql:  "SELECT 1;\nSELECT 2",
			want: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name: "only comments",
			sql:  "-- nothing to restore\n/* really */\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitStatements(tt.sql); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitStatements() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConcurrentIndexNames(t *testing.T) {
	statements := splitStatements(`
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_a ON record (collection);
		create unique index concurrently idx_b
			ON record (uri);
		CREATE INDEX idx_plain ON record (did);
		DROP INDEX CONCURRENTLY IF EXISTS idx_c;`)
	want := []string{"idx_a", "idx_b"}
	if got := concurrentIndexNames(statements); !reflect.DeepEqual(got, want) {
		t.Fatalf("concurrentIndexNames() = %v, want %v", got, want)
	}
}
