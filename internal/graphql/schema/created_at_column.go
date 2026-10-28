package schema

import (
	"fmt"

	"github.com/GainForest/hyperindex/internal/atproto"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// usesRecordCreatedAtColumn reports whether validated reads of collection can
// sort and filter createdAt through the indexed record_created_at column
// instead of json->>'createdAt'.
//
// This is only safe when the Lexicon declares top-level createdAt as a required
// datetime: validation uses the same strict datetime parser that fills
// record_created_at, so every valid row then has a non-NULL column value and
// the column's IS NOT NULL predicate excludes nothing. Unvalidated reads keep
// the JSON path because invalid rows may lack a parseable createdAt.
func usesRecordCreatedAtColumn(registry *lexicon.Registry, collection string, validOnly bool) bool {
	if !validOnly || registry == nil {
		return false
	}
	def, ok := registry.GetRecordDef(collection)
	if !ok || def == nil {
		return false
	}
	prop := def.GetProperty("createdAt")
	return prop != nil && prop.Type == lexicon.TypeString && prop.Format == lexicon.FormatDatetime && prop.Required
}

// mapCreatedAtToColumn rewrites a createdAt sort and createdAt filters to the
// record_created_at column. Filters whose operator or value the column cannot
// represent exactly (contains, startsWith, isNull, date-only strings) keep the
// JSON string comparison so existing results do not change.
func mapCreatedAtToColumn(sortOpt *repositories.SortOption, filters []repositories.FieldFilter) {
	if sortOpt != nil && sortOpt.Field == "createdAt" {
		sortOpt.Field = repositories.SortFieldRecordCreatedAt
	}
	for i := range filters {
		if column, ok := createdAtColumnFilter(filters[i]); ok {
			filters[i] = column
		}
	}
}

func createdAtColumnFilter(f repositories.FieldFilter) (repositories.FieldFilter, bool) {
	if f.Field != "createdAt" || len(f.Path) > 0 || len(f.ArrayPath) > 0 ||
		(f.Target != "" && f.Target != repositories.FieldFilterTargetJSON) {
		return f, false
	}

	var value interface{}
	switch f.Operator {
	case "eq", "neq", "gt", "lt", "gte", "lte":
		s, ok := f.Value.(string)
		if !ok {
			return f, false
		}
		normalized, ok := atproto.NormalizeRecordDatetime(s)
		if !ok {
			return f, false
		}
		value = normalized
	case "in":
		list, ok := f.Value.([]interface{})
		if !ok {
			return f, false
		}
		normalizedList := make([]interface{}, 0, len(list))
		for _, v := range list {
			s, ok := v.(string)
			if !ok {
				return f, false
			}
			normalized, ok := atproto.NormalizeRecordDatetime(s)
			if !ok {
				return f, false
			}
			normalizedList = append(normalizedList, normalized)
		}
		value = normalizedList
	default:
		return f, false
	}

	return repositories.FieldFilter{
		Field:     repositories.SortFieldRecordCreatedAt,
		Operator:  f.Operator,
		Value:     value,
		FieldType: "datetime",
		Target:    repositories.FieldFilterTargetColumn,
	}, true
}

// normalizeCreatedAtCursor rewrites the sort value of a createdAt cursor to the
// stored record_created_at form. Cursors issued before the column mapping hold
// the raw JSON string, which can carry extra precision or a non-UTC offset;
// comparing that raw value would repeat or skip rows at the page boundary.
func normalizeCreatedAtCursor(sortOpt *repositories.SortOption, cursorValues []string) error {
	if sortOpt == nil || sortOpt.Field != repositories.SortFieldRecordCreatedAt || len(cursorValues) != 2 {
		return nil
	}
	normalized, ok := atproto.NormalizeRecordDatetime(cursorValues[0])
	if !ok {
		return fmt.Errorf("invalid cursor: createdAt value %q is not a valid datetime; restart pagination without a cursor", cursorValues[0])
	}
	cursorValues[0] = normalized
	return nil
}
