package schema

import (
	"fmt"

	"github.com/GainForest/hyperindex/internal/atproto"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// createdAtDatetimeProperty returns the Lexicon's top-level createdAt property
// when it is declared as a datetime string, and nil otherwise.
func createdAtDatetimeProperty(registry *lexicon.Registry, collection string) *lexicon.Property {
	if registry == nil {
		return nil
	}
	def, ok := registry.GetRecordDef(collection)
	if !ok || def == nil {
		return nil
	}
	prop := def.GetProperty("createdAt")
	if prop == nil || prop.Type != lexicon.TypeString || prop.Format != lexicon.FormatDatetime {
		return nil
	}
	return prop
}

// sortsByRecordCreatedAtColumn reports whether sortBy: createdAt on collection
// can order by the indexed record_created_at column instead of
// json->>'createdAt'.
//
// That only needs createdAt to be declared as a datetime. The column is filled
// from the same strict datetime parser regardless of validation status, and is
// NULL exactly when createdAt is missing, not a string, or not a valid datetime.
// The query keeps those NULL rows and sorts them as the oldest, so neither a
// required createdAt nor a validated read is needed.
func sortsByRecordCreatedAtColumn(registry *lexicon.Registry, collection string) bool {
	return createdAtDatetimeProperty(registry, collection) != nil
}

// filtersByRecordCreatedAtColumn reports whether where: { createdAt: ... }
// filters on collection can read record_created_at instead of the JSON value.
//
// Unlike sorting, a filter drops rows whose column is NULL, so it is only safe
// when createdAt is a required datetime and the read is validated: validation
// uses the same strict datetime parser that fills the column, so every valid
// row then has a non-NULL value. Unvalidated reads and optional createdAt keep
// the JSON comparison.
func filtersByRecordCreatedAtColumn(registry *lexicon.Registry, collection string, validOnly bool) bool {
	prop := createdAtDatetimeProperty(registry, collection)
	return validOnly && prop != nil && prop.Required
}

// mapCreatedAtSortToColumn rewrites a createdAt sort to the record_created_at
// column.
func mapCreatedAtSortToColumn(sortOpt *repositories.SortOption) {
	if sortOpt != nil && sortOpt.Field == "createdAt" {
		sortOpt.Field = repositories.SortFieldRecordCreatedAt
	}
}

// mapCreatedAtFiltersToColumn rewrites createdAt filters to the
// record_created_at column. Filters whose operator or value the column cannot
// represent exactly (contains, startsWith, isNull, date-only strings) keep the
// JSON string comparison so existing results do not change.
func mapCreatedAtFiltersToColumn(filters []repositories.FieldFilter) {
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
//
// An empty value is the cursor of a record without a record_created_at and is
// kept as is; the repository reads it as "on the NULL rows".
func normalizeCreatedAtCursor(sortOpt *repositories.SortOption, cursorValues []string) error {
	if sortOpt == nil || sortOpt.Field != repositories.SortFieldRecordCreatedAt || len(cursorValues) != 2 || cursorValues[0] == "" {
		return nil
	}
	normalized, ok := atproto.NormalizeRecordDatetime(cursorValues[0])
	if !ok {
		return fmt.Errorf("invalid cursor: createdAt value %q is not a valid datetime; restart pagination without a cursor", cursorValues[0])
	}
	cursorValues[0] = normalized
	return nil
}
