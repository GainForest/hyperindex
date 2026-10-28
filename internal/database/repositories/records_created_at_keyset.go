package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/GainForest/hyperindex/internal/database"
)

// recordCreatedAtTextPG renders record_created_at in the normalized UTC
// millisecond form used by cursors and stored by SQLite.
const recordCreatedAtTextPG = `to_char(record_created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`

// recordCreatedAtNullsClause makes the NULL placement of a record_created_at
// ordering explicit: NULL is the oldest value, so DESC ends with it and ASC
// starts with it. It matches idx_record_collection_created_at.
func recordCreatedAtNullsClause(direction string) string {
	if direction == "ASC" {
		return "NULLS FIRST"
	}
	return "NULLS LAST"
}

// recordCreatedAtOrderBy returns the ORDER BY list for a record_created_at
// sort over the given column names, with the uri tiebreaker in the same
// direction. The NULLS clause is explicit on PostgreSQL only: it defaults to
// NULLS FIRST for DESC, the opposite of the index, while SQLite already treats
// NULL as the smallest value.
func (r *RecordsRepository) recordCreatedAtOrderBy(createdAtColumn, uriColumn, direction string) string {
	if direction != "ASC" {
		direction = "DESC"
	}
	expr := createdAtColumn + " " + direction
	if r.db.Dialect() == database.PostgreSQL {
		expr += " " + recordCreatedAtNullsClause(direction)
	}
	return fmt.Sprintf("%s, %s %s", expr, uriColumn, direction)
}

// keysetSegment is one contiguous range of a keyset continuation: the WHERE
// condition that selects it and the bind arguments that condition uses.
type keysetSegment struct {
	condition string
	args      []any
}

// recordCreatedAtKeysetSegments returns the ranges that follow the cursor
// (cursorValue, cursorURI) in a record_created_at ordering, in the order they
// are read. Placeholders start at firstPlaceholder. direction is the direction
// of the query being built, so a backward page passes its reversed direction.
//
// An empty cursorValue is a cursor on a row whose record_created_at is NULL.
// NULL is the oldest value, so the rows after it are the NULL rows with a later
// uri in that direction and, only when ascending, every dated row. After a
// dated row, descending also continues into all NULL rows.
//
// Every segment is a single index range: a row-value comparison for the dated
// part, which both databases turn into an index bound at the cursor, and
// IS NULL / IS NOT NULL plus uri for the rest. They are kept apart because a
// condition such as "dated range OR created IS NULL" cannot be bounded, so the
// database would scan from the head of the index and discard every row before
// the cursor. See readSegments for how a page is filled from them.
func (r *RecordsRepository) recordCreatedAtKeysetSegments(direction, cursorValue, cursorURI string, firstPlaceholder int) []keysetSegment {
	ascending := direction == "ASC"
	cmp := "<"
	if ascending {
		cmp = ">"
	}

	if cursorValue == "" {
		undated := keysetSegment{
			condition: fmt.Sprintf("(record_created_at IS NULL AND uri %s %s)", cmp, r.db.Placeholder(firstPlaceholder)),
			args:      []any{cursorURI},
		}
		if !ascending {
			return []keysetSegment{undated}
		}
		return []keysetSegment{undated, {condition: "record_created_at IS NOT NULL"}}
	}

	createdAt := r.recordCreatedAtCursorValueExpr(r.db.Placeholder(firstPlaceholder))
	dated := keysetSegment{
		condition: fmt.Sprintf("(record_created_at, uri) %s (%s, %s)", cmp, createdAt, r.db.Placeholder(firstPlaceholder+1)),
		args:      []any{cursorValue, cursorURI},
	}
	if ascending {
		return []keysetSegment{dated}
	}
	return []keysetSegment{dated, {condition: "record_created_at IS NULL"}}
}

// sortedPage is a keyset page query. Most pages are one statement; a
// record_created_at page whose continuation spans the dated and undated rows is
// several segments that are read in order (see readSegments).
type sortedPage struct {
	sort     *SortOption
	columns  string
	orderBy  string
	segments []pageSegment
}

// pageSegment is the filtered WHERE clause and bind arguments of one segment.
type pageSegment struct {
	where string
	args  []any
}

// sql returns the statement that reads up to limit rows of one segment.
func (p sortedPage) sql(segment, limit int) string {
	return fmt.Sprintf("SELECT %s FROM record WHERE %s ORDER BY %s LIMIT %d",
		p.columns, p.segments[segment].where, p.orderBy, limit)
}

// newSortedPage joins the conditions shared by every segment with each segment's
// own condition and arguments. A nil keyset means a single segment with no
// extra condition.
func (r *RecordsRepository) newSortedPage(sort *SortOption, whereParts []string, args []any, keyset []keysetSegment) sortedPage {
	if len(keyset) == 0 {
		keyset = []keysetSegment{{}}
	}
	segments := make([]pageSegment, len(keyset))
	for i, k := range keyset {
		parts := append([]string(nil), whereParts...)
		if k.condition != "" {
			parts = append(parts, k.condition)
		}
		segments[i] = pageSegment{
			where: strings.Join(parts, " AND "),
			args:  append(append([]any(nil), args...), k.args...),
		}
	}
	return sortedPage{
		sort:     sort,
		columns:  r.sortedRecordColumns(sort),
		orderBy:  r.buildSortExpr(sort),
		segments: segments,
	}
}

// readSortedPage reads up to limit rows of page. label names the page kind in
// the error ("sorted" or "reversed").
//
// A page that spans several segments is filled sequentially, so it is not read
// from one snapshot. A write that lands between two segment statements and
// moves a row from one segment to the other (a record gaining its first
// parseable createdAt) can make that page repeat or miss that one row, the same
// effect a write between two pages already has.
func (r *RecordsRepository) readSortedPage(ctx context.Context, page sortedPage, limit int, label string) ([]*Record, error) {
	return readSegments(limit, len(page.segments), func(segment, remaining int) ([]*Record, error) {
		return r.querySegment(ctx, page, segment, remaining, label)
	})
}

func (r *RecordsRepository) querySegment(ctx context.Context, page sortedPage, segment, limit int, label string) ([]*Record, error) {
	args := page.segments[segment].args
	if err := r.validateSQLiteAggregateParameterCount(len(args)); err != nil {
		return nil, err
	}
	rows, err := r.db.DB().QueryContext(ctx, page.sql(segment, limit), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s records: %w", label, err)
	}
	defer rows.Close()
	return scanSortedRecords(rows, page.sort)
}

// readSegments fills a page of limit rows from segments read in order. The
// first segment always runs. Each later segment runs only if the rows so far
// fall short of limit, and then asks for just the shortfall, so a page the first
// segment can fill never touches the others. This is what keeps a selective
// filter on the undated rows from costing anything on a dated page.
func readSegments(limit, segments int, read func(segment, limit int) ([]*Record, error)) ([]*Record, error) {
	var records []*Record
	for segment := 0; segment < segments; segment++ {
		if segment > 0 && len(records) >= limit {
			break
		}
		page, err := read(segment, limit-len(records))
		if err != nil {
			return nil, err
		}
		records = append(records, page...)
	}
	return records, nil
}
