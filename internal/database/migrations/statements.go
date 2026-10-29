package migrations

import (
	"regexp"
	"strings"
)

// noTransactionMarker opts a migration file out of the runner's transaction.
// It must be the first non-blank line of the file.
//
// PostgreSQL rejects CREATE/DROP INDEX CONCURRENTLY inside a transaction block,
// and it also treats a multi-statement simple query as one implicit
// transaction. A marked file is therefore executed one statement at a time on
// the connection pool, and the schema_migrations row is written afterwards.
// That gives up atomicity, so every statement in a marked file must be safe to
// repeat (IF [NOT] EXISTS): a crash between the last statement and the version
// row makes the next startup run the file again.
const noTransactionMarker = "-- hyperindex:no-transaction"

// runsOutsideTransaction reports whether a migration file carries
// noTransactionMarker as its first non-blank line.
func runsOutsideTransaction(migrationSQL string) bool {
	for _, line := range strings.Split(migrationSQL, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		return trimmed == noTransactionMarker
	}
	return false
}

// splitStatements splits migration SQL on top-level semicolons and drops
// comments. It understands -- and /* */ comments, single-quoted strings,
// double-quoted identifiers and dollar-quoted bodies, which is enough for DDL.
// Empty statements are omitted.
func splitStatements(migrationSQL string) []string {
	var statements []string
	var current strings.Builder
	flush := func() {
		if statement := strings.TrimSpace(current.String()); statement != "" {
			statements = append(statements, statement)
		}
		current.Reset()
	}

	for i := 0; i < len(migrationSQL); {
		rest := migrationSQL[i:]
		switch {
		case strings.HasPrefix(rest, "--"):
			end := strings.IndexByte(rest, '\n')
			if end < 0 {
				i = len(migrationSQL)
			} else {
				i += end // keep the newline so tokens on either side stay apart
			}
		case strings.HasPrefix(rest, "/*"):
			end := strings.Index(rest[2:], "*/")
			if end < 0 {
				i = len(migrationSQL)
			} else {
				i += 2 + end + 2
			}
			current.WriteByte(' ')
		case rest[0] == '\'' || rest[0] == '"':
			length := quotedLength(rest)
			current.WriteString(rest[:length])
			i += length
		case rest[0] == '$':
			if tag := dollarQuoteTag.FindString(rest); tag != "" {
				length := dollarQuotedLength(rest, tag)
				current.WriteString(rest[:length])
				i += length
				continue
			}
			current.WriteByte(rest[0])
			i++
		case rest[0] == ';':
			flush()
			i++
		default:
			current.WriteByte(rest[0])
			i++
		}
	}
	flush()
	return statements
}

var dollarQuoteTag = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$`)

// quotedLength returns the length of the quoted token at the start of s,
// including both quotes. A doubled quote inside the token is an escape. An
// unterminated token runs to the end of s.
func quotedLength(s string) int {
	quote := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != quote {
			continue
		}
		if i+1 < len(s) && s[i+1] == quote {
			i++
			continue
		}
		return i + 1
	}
	return len(s)
}

// dollarQuotedLength returns the length of the dollar-quoted token at the
// start of s, including both tags. An unterminated token runs to the end of s.
func dollarQuotedLength(s, tag string) int {
	end := strings.Index(s[len(tag):], tag)
	if end < 0 {
		return len(s)
	}
	return len(tag) + end + len(tag)
}

// createIndexConcurrently captures the index name of a CREATE INDEX
// CONCURRENTLY statement. Only unquoted identifiers are supported, which is all
// the migrations use.
var createIndexConcurrently = regexp.MustCompile(`(?is)^CREATE\s+(?:UNIQUE\s+)?INDEX\s+CONCURRENTLY\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z_][A-Za-z0-9_]*)`)

// concurrentIndexNames returns the names of the indexes the statements build
// with CREATE INDEX CONCURRENTLY.
func concurrentIndexNames(statements []string) []string {
	var names []string
	for _, statement := range statements {
		if match := createIndexConcurrently.FindStringSubmatch(statement); match != nil {
			names = append(names, match[1])
		}
	}
	return names
}
