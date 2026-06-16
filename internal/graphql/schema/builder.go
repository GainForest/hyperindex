// Package schema provides the GraphQL schema builder.
package schema

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/certifiedprofiles"
	"github.com/GainForest/hyperindex/internal/graphql/externallabels"
	"github.com/GainForest/hyperindex/internal/graphql/query"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/subscription"
	"github.com/GainForest/hyperindex/internal/graphql/types"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// Builder builds a GraphQL schema from lexicon definitions.
type Builder struct {
	registry      *lexicon.Registry
	mapper        *types.Mapper
	objectBuilder *types.ObjectBuilder

	// Built types
	recordTypes     map[string]*graphql.Object      // lexiconID -> record type
	connectionTypes map[string]*graphql.Object      // lexiconID -> connection type
	sortFieldEnums  map[string]*graphql.Enum        // lexiconID -> sort field enum
	whereInputTypes map[string]*graphql.InputObject // lexiconID -> where input type

	genericRecordType       *graphql.Object
	genericRecordConnection *graphql.Object
}

// NewBuilder creates a new schema builder.
func NewBuilder(registry *lexicon.Registry) *Builder {
	mapper := types.NewMapper()
	return &Builder{
		registry:        registry,
		mapper:          mapper,
		objectBuilder:   types.NewObjectBuilder(mapper, registry),
		recordTypes:     make(map[string]*graphql.Object),
		connectionTypes: make(map[string]*graphql.Object),
		sortFieldEnums:  make(map[string]*graphql.Enum),
		whereInputTypes: make(map[string]*graphql.InputObject),
	}
}

// Build builds the complete GraphQL schema.
func (b *Builder) Build() (*graphql.Schema, error) {
	// Phase 1: Build all object types (non-record helper types)
	b.buildObjectTypes()

	// Phase 2: Build all record types
	b.buildRecordTypes()

	// Phase 2b: Build GenericRecord types now that generated record types are available.
	b.buildGenericRecordTypes()

	// Phase 2c: Build per-collection WhereInput types
	b.buildWhereInputTypes()

	// Phase 3: Build connection types
	b.buildConnectionTypes()

	// Phase 3b: Build sort field enums for each collection
	b.buildSortFieldEnums()

	// Phase 4: Build Query type
	queryType := b.buildQueryType()

	// Phase 5: Build Subscription type
	subscriptionType := b.buildSubscriptionType()

	// Create schema
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:        queryType,
		Subscription: subscriptionType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &schema, nil
}

// buildObjectTypes builds GraphQL types for all non-record definitions.
func (b *Builder) buildObjectTypes() {
	// Get all lexicons that only have defs (no main record)
	for _, lex := range b.registry.GetDefsLexicons() {
		for defName, def := range lex.Defs.Others {
			if def.IsObject() {
				ref := lexicon.MakeRef(lex.ID, defName)
				b.objectBuilder.BuildObjectType(ref, def.Object)
			}
		}
	}

	// Also build object defs from collection lexicons
	for _, lex := range b.registry.GetCollectionLexicons() {
		for defName, def := range lex.Defs.Others {
			if def.IsObject() {
				ref := lexicon.MakeRef(lex.ID, defName)
				b.objectBuilder.BuildObjectType(ref, def.Object)
			}
		}
	}
}

// buildRecordTypes builds GraphQL types for all record definitions.
func (b *Builder) buildRecordTypes() {
	for _, lex := range b.registry.GetCollectionLexicons() {
		if lex.Defs.Main != nil {
			recordType := b.objectBuilder.BuildRecordType(lex.ID, lex.Defs.Main)
			b.recordTypes[lex.ID] = recordType
		}
	}
}

// buildConnectionTypes builds Relay connection types for all record types.
func (b *Builder) buildConnectionTypes() {
	for lexiconID, recordType := range b.recordTypes {
		connectionType := query.BuildConnectionType(recordType)
		b.connectionTypes[lexiconID] = connectionType
	}
}

// buildSortFieldEnums builds per-collection sort field enums from lexicon properties.
func (b *Builder) buildSortFieldEnums() {
	for _, lex := range b.registry.GetCollectionLexicons() {
		if lex.Defs.Main == nil {
			continue
		}

		recordType, ok := b.recordTypes[lex.ID]
		if !ok {
			continue
		}

		// Collect sortable properties from the lexicon's main record definition
		var sortableProps []query.SortableProperty
		for _, entry := range lex.Defs.Main.Properties {
			sortableProps = append(sortableProps, query.SortableProperty{
				Name:   entry.Name,
				Type:   entry.Property.Type,
				Format: entry.Property.Format,
			})
		}

		sortEnum := query.BuildSortFieldEnum(recordType.Name(), sortableProps)
		b.sortFieldEnums[lex.ID] = sortEnum
	}
}

var externalLabelPredicateInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "ExternalLabelPredicateInput",
	Description: "Filter conditions for matching external labels on records.",
	Fields: graphql.InputObjectConfigFieldMap{
		"src": &graphql.InputObjectFieldConfig{
			Type:        types.StringFilterInput,
			Description: "Filter by label source DID.",
		},
		"val": &graphql.InputObjectFieldConfig{
			Type:        types.StringFilterInput,
			Description: "Filter by label value.",
		},
		"activeOnly": &graphql.InputObjectFieldConfig{
			Type:         graphql.Boolean,
			DefaultValue: true,
			Description:  "When true, only active external labels can match records.",
		},
	},
})

var externalLabelWhereInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "ExternalLabelWhereInput",
	Description: "Record-level external label predicates.",
	Fields: graphql.InputObjectConfigFieldMap{
		"has": &graphql.InputObjectFieldConfig{
			Type:        externalLabelPredicateInput,
			Description: "Keep records that have a matching external label.",
		},
		"none": &graphql.InputObjectFieldConfig{
			Type:        externalLabelPredicateInput,
			Description: "Keep records that do not have a matching external label.",
		},
	},
})

// buildWhereInputTypes builds per-collection WhereInput GraphQL InputObject types.
// For each collection lexicon, it creates a WhereInput type with scalar
// operators for scalar properties, presence-only filters for complex top-level
// properties, and explicit metadata filters.
func (b *Builder) buildWhereInputTypes() {
	for _, lex := range b.registry.GetCollectionLexicons() {
		if lex.Defs.Main == nil {
			continue
		}

		typeName := lexicon.ToTypeName(lex.ID) + "WhereInput"
		fields := graphql.InputObjectConfigFieldMap{}

		// Always include URI and DID as filterable metadata fields.
		// Both are column-level filters, so only exact and batched lookup operators
		// are exposed; substring operators are intentionally not meaningful here.
		fields["uri"] = &graphql.InputObjectFieldConfig{
			Type:        types.URIFilterInput,
			Description: "Filter by AT-URI",
		}
		fields["did"] = &graphql.InputObjectFieldConfig{
			Type:        types.DIDFilterInput,
			Description: "Filter by DID (record author)",
		}
		fields["externalLabels"] = &graphql.InputObjectFieldConfig{
			Type:        externalLabelWhereInput,
			Description: "Filter records by locally ingested external labels before pagination.",
		}

		// Add a field for each filterable property.
		for _, entry := range lex.Defs.Main.Properties {
			if entry.Name == "did" {
				continue // did is handled separately as a metadata filter
			}
			if types.ReservedRecordFields[entry.Name] {
				continue // Skip properties that collide with reserved metadata fields
			}
			filterInput := types.FilterInputForLexiconProperty(entry.Property.Type, entry.Property.Format)
			if filterInput == nil {
				continue // Non-filterable type, such as record.
			}

			description := fmt.Sprintf("Filter by %s", entry.Name)
			if filterInput == types.PresenceFilterInput {
				description = fmt.Sprintf("Filter by whether %s is missing/null or present; nested values are not filterable", entry.Name)
			}
			fields[entry.Name] = &graphql.InputObjectFieldConfig{
				Type:        filterInput,
				Description: description,
			}
		}

		whereInput := graphql.NewInputObject(graphql.InputObjectConfig{
			Name:        typeName,
			Description: fmt.Sprintf("Filter conditions for %s queries", lex.ID),
			Fields:      fields,
		})

		b.whereInputTypes[lex.ID] = whereInput
	}
}

// RecordEvent GraphQL type for subscriptions
var recordEventType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "RecordEvent",
	Description: "A real-time record change event",
	Fields: graphql.Fields{
		"type": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Event type: create, update, or delete",
		},
		"uri": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "AT-URI of the record",
		},
		"cid": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "CID of the record",
		},
		"did": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID of the actor who made the change",
		},
		"collection": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Collection NSID",
		},
		"record": &graphql.Field{
			Type:        types.JSONScalar,
			Description: "The record data (null for delete events)",
		},
	},
})

// CollectionStat GraphQL type for collection statistics
var collectionStatType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "CollectionStat",
	Description: "Statistics for a collection",
	Fields: graphql.Fields{
		"collection": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Collection NSID",
		},
		"count": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Number of records in the collection",
		},
	},
})

// TimeSeriesPoint GraphQL type for time series data points
var timeSeriesPointType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "TimeSeriesPoint",
	Description: "A single data point in a time series",
	Fields: graphql.Fields{
		"date": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Date in YYYY-MM-DD format",
		},
		"count": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Number of records on this date",
		},
		"cumulative": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Cumulative count up to and including this date",
		},
	},
})

// CollectionTimeSeries GraphQL type for collection time series data
var collectionTimeSeriesType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "CollectionTimeSeries",
	Description: "Time series data for a collection",
	Fields: graphql.Fields{
		"collection": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Collection NSID",
		},
		"totalRecords": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Total number of records in the collection",
		},
		"uniqueUsers": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Number of unique users (DIDs) in the collection",
		},
		"data": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(timeSeriesPointType))),
			Description: "Time series data points",
		},
	},
})

// buildSubscriptionType builds the root Subscription type.
func (b *Builder) buildSubscriptionType() *graphql.Object {
	fields := graphql.Fields{
		// Subscribe to all record events
		"recordEvents": &graphql.Field{
			Type:        recordEventType,
			Description: "Subscribe to all record change events",
			Args: graphql.FieldConfigArgument{
				"collection": &graphql.ArgumentConfig{
					Type:        graphql.String,
					Description: "Filter by collection NSID (optional)",
				},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				// Extract recordEvents from the root object passed by subscription handler
				if m, ok := p.Source.(map[string]interface{}); ok {
					return m["recordEvents"], nil
				}
				return p.Source, nil
			},
		},
	}

	// Add per-collection subscriptions
	for lexiconID, recordType := range b.recordTypes {
		fieldName := lexicon.ToFieldName(lexiconID) + "Events"
		collection := lexiconID // Capture for closure

		fields[fieldName] = &graphql.Field{
			Type:        recordType,
			Description: fmt.Sprintf("Subscribe to %s record changes", lexiconID),
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				event, ok := p.Source.(*subscription.RecordEvent)
				if !ok || event == nil {
					return nil, nil
				}
				// Only return if collection matches
				if event.Collection != collection {
					return nil, nil
				}
				if event.Record != nil {
					b.coerceRequiredFields(event.Record, collection)
				}
				return event.Record, nil
			},
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   "Subscription",
		Fields: fields,
	})
}

// buildGenericRecordTypes builds the GenericRecord connection types. This runs
// after generated record types so GenericRecord can reference virtual fields that
// point at generated types, such as certifiedProfileData.
func (b *Builder) buildGenericRecordTypes() {
	fields := graphql.Fields{
		"uri": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "AT-URI of the record",
		},
		"cid": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "CID of the record",
		},
		"did": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID of the actor",
		},
		"collection": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Collection NSID",
		},
		"rkey": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Record key",
		},
		"value": &graphql.Field{
			Type:        types.JSONScalar,
			Description: "The record data as JSON",
		},
		"externalLabels": externallabels.Field(),
	}
	if profileType, ok := b.recordTypes[certifiedprofiles.CollectionID]; ok {
		fields["certifiedProfileData"] = certifiedprofiles.Field(profileType)
	}

	b.genericRecordType = graphql.NewObject(graphql.ObjectConfig{
		Name:        "GenericRecord",
		Description: "A generic AT Protocol record",
		Fields:      fields,
	})

	genericRecordEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "GenericRecordEdge",
		Fields: graphql.Fields{
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"node":   &graphql.Field{Type: b.genericRecordType},
		},
	})

	b.genericRecordConnection = graphql.NewObject(graphql.ObjectConfig{
		Name: "GenericRecordConnection",
		Fields: graphql.Fields{
			"edges":      &graphql.Field{Type: graphql.NewList(genericRecordEdgeType)},
			"pageInfo":   &graphql.Field{Type: query.PageInfoType},
			"totalCount": &graphql.Field{Type: graphql.Int, Description: "Total number of items (if known)"},
		},
	})
}

// buildQueryType builds the root Query type with fields for each collection.
func (b *Builder) buildQueryType() *graphql.Object {
	fields := graphql.Fields{}

	// Add generic records query that works for any collection
	fields["records"] = &graphql.Field{
		Type:        b.genericRecordConnection,
		Description: "Query records from any collection (useful for collections without lexicon schemas)",
		Args: graphql.FieldConfigArgument{
			"collection": &graphql.ArgumentConfig{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "Collection NSID (e.g., org.impactindexer.review.like)",
			},
			"first": &graphql.ArgumentConfig{
				Type:        graphql.Int,
				Description: "Number of records to return (default 20, max 1000)",
			},
			"after": &graphql.ArgumentConfig{
				Type:        graphql.String,
				Description: "Cursor for forward pagination",
			},
			"last": &graphql.ArgumentConfig{
				Type:        graphql.Int,
				Description: "Number of items to return from the end",
			},
			"before": &graphql.ArgumentConfig{
				Type:        graphql.String,
				Description: "Cursor to paginate before (backward pagination)",
			},
		},
		Resolve: b.createGenericRecordsResolver(),
	}

	// Add external label lookup by subject DID or AT-URI.
	fields["externalLabels"] = &graphql.Field{
		Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(externallabels.Type))),
		Description: "Query locally ingested external ATProto labels by DID or AT-URI subject.",
		Args:        externallabels.Args(true),
		Resolve:     b.createExternalLabelsResolver(),
	}

	// Add search query for cross-collection text search
	fields["search"] = &graphql.Field{
		Type:        b.genericRecordConnection,
		Description: "Search records by text content",
		Args: graphql.FieldConfigArgument{
			"query": &graphql.ArgumentConfig{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "Search text (matched against record JSON content)",
			},
			"collection": &graphql.ArgumentConfig{
				Type:        graphql.String,
				Description: "Optional collection NSID to restrict search",
			},
			"first": &graphql.ArgumentConfig{
				Type:         graphql.Int,
				DefaultValue: 20,
			},
			"after": &graphql.ArgumentConfig{
				Type: graphql.String,
			},
		},
		Resolve: b.createSearchResolver(),
	}

	// Add collectionStats query for efficient aggregate counts
	fields["collectionStats"] = &graphql.Field{
		Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(collectionStatType))),
		Description: "Get record counts for collections (efficient aggregate query)",
		Args: graphql.FieldConfigArgument{
			"collections": &graphql.ArgumentConfig{
				Type:        graphql.NewList(graphql.NewNonNull(graphql.String)),
				Description: "Filter by collection NSIDs (optional, returns all if not specified)",
			},
		},
		Resolve: b.createCollectionStatsResolver(),
	}

	// Add collectionTimeSeries query for time series data
	fields["collectionTimeSeries"] = &graphql.Field{
		Type:        collectionTimeSeriesType,
		Description: "Get time series data for a collection (records grouped by date)",
		Args: graphql.FieldConfigArgument{
			"collection": &graphql.ArgumentConfig{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "Collection NSID",
			},
		},
		Resolve: b.createCollectionTimeSeriesResolver(),
	}

	for lexiconID, connectionType := range b.connectionTypes {
		fieldName := lexicon.ToFieldName(lexiconID)

		// Build args: start with standard connection args, then add sort args if available
		args := query.ConnectionArgs()
		if sortEnum, ok := b.sortFieldEnums[lexiconID]; ok {
			args["sortBy"] = &graphql.ArgumentConfig{
				Type:        sortEnum,
				Description: "Field to sort by (default: indexed_at)",
			}
			args["sortDirection"] = &graphql.ArgumentConfig{
				Type:        query.SortDirectionEnum,
				Description: "Sort direction (default: DESC)",
			}
		}
		if whereInput, ok := b.whereInputTypes[lexiconID]; ok {
			args["where"] = &graphql.ArgumentConfig{
				Type:        whereInput,
				Description: "Filter conditions",
			}
		}

		fields[fieldName] = &graphql.Field{
			Type:        connectionType,
			Description: fmt.Sprintf("Query %s records", lexiconID),
			Args:        args,
			Resolve:     b.createCollectionResolver(lexiconID),
		}

		// Also add a singular lookup by URI
		recordType := b.recordTypes[lexiconID]
		fields[fieldName+"ByUri"] = &graphql.Field{
			Type:        recordType,
			Description: fmt.Sprintf("Get a single %s by AT-URI", lexiconID),
			Args: graphql.FieldConfigArgument{
				"uri": &graphql.ArgumentConfig{
					Type:        graphql.NewNonNull(graphql.String),
					Description: "AT-URI of the record",
				},
			},
			Resolve: b.createSingleRecordResolver(lexiconID),
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   "Query",
		Fields: fields,
	})
}

// nodeBuilder transforms a Record and its parsed JSON into a GraphQL node.
type nodeBuilder func(rec *repositories.Record, value map[string]interface{}) (interface{}, bool)

// extractFilters extracts FieldFilter conditions and an optional DIDFilter from
// the GraphQL `where` argument. The whereArg is expected to be a
// map[string]interface{} where each key is a field name and each value is a
// map[string]interface{} of operator→value pairs (e.g. {"eq": "hello"}).
//
// The special key "did" is extracted separately as a DID column filter rather
// than a JSON field filter. DIDFilterInput only exposes "eq" and "in", so only
// those operators are handled. All other keys are looked up in the lexicon registry
// to determine the correct FieldType for SQL casting.
func extractFilters(whereArg interface{}, lexiconID string, registry *lexicon.Registry) ([]repositories.FieldFilter, repositories.DIDFilter, error) {
	filters, didFilter, _, err := extractFiltersWithExternalLabels(whereArg, lexiconID, registry)
	return filters, didFilter, err
}

func extractFiltersWithExternalLabels(whereArg interface{}, lexiconID string, registry *lexicon.Registry) ([]repositories.FieldFilter, repositories.DIDFilter, repositories.ExternalLabelRecordFilter, error) {
	whereMap, ok := whereArg.(map[string]interface{})
	if !ok || len(whereMap) == 0 {
		return nil, repositories.DIDFilter{}, repositories.ExternalLabelRecordFilter{}, nil
	}

	var filters []repositories.FieldFilter
	var didFilter repositories.DIDFilter
	var externalLabelFilter repositories.ExternalLabelRecordFilter

	// Look up the record definition once for property type resolution
	recordDef, _ := registry.GetRecordDef(lexiconID)

	for fieldName, filterVal := range whereMap {
		filterMap, ok := filterVal.(map[string]interface{})
		if !ok || filterMap == nil {
			continue
		}

		if fieldName == "did" {
			// DID is a column filter, not a JSON field filter.
			// DIDFilterInput only exposes "eq" and "in".
			// The DID filter does not count toward MaxFilterConditions.
			if eqVal, ok := filterMap["eq"].(string); ok && eqVal != "" {
				didFilter.EQ = eqVal
			}
			if inVals, ok := filterMap["in"].([]interface{}); ok {
				for _, v := range inVals {
					if s, ok := v.(string); ok && s != "" {
						didFilter.IN = append(didFilter.IN, s)
					}
				}
			}
			continue
		}

		if fieldName == "externalLabels" {
			parsedExternalLabelFilter, err := extractExternalLabelRecordFilter(filterMap)
			if err != nil {
				return nil, repositories.DIDFilter{}, repositories.ExternalLabelRecordFilter{}, err
			}
			externalLabelFilter = parsedExternalLabelFilter
			continue
		}

		// Determine the filter target and lexicon type for this field so the repository
		// can read from the correct storage location and CAST correctly. URI is
		// generated metadata, not a JSON property, so it targets the record column and
		// must stay string-typed even if a lexicon defines a colliding numeric property
		// named "uri".
		fieldType := "string" // default
		fieldTarget := repositories.FieldFilterTargetJSON
		if fieldName == "uri" {
			fieldTarget = repositories.FieldFilterTargetColumn
		} else if recordDef != nil {
			if prop := recordDef.GetProperty(fieldName); prop != nil {
				if prop.Format == "datetime" {
					fieldType = "datetime"
				} else {
					fieldType = prop.Type
				}
			}
		}

		// Each key in filterMap is an operator (eq, neq, gt, lt, gte, lte, in, contains, startsWith, isNull).
		for op, val := range filterMap {
			if val == nil {
				continue
			}
			filters = append(filters, repositories.FieldFilter{
				Field:     fieldName,
				Operator:  op,
				Value:     val,
				FieldType: fieldType,
				Target:    fieldTarget,
			})
		}
	}

	if len(filters) > repositories.MaxFilterConditions {
		return nil, repositories.DIDFilter{}, repositories.ExternalLabelRecordFilter{}, fmt.Errorf("too many filter conditions: %d (maximum %d)", len(filters), repositories.MaxFilterConditions)
	}

	return filters, didFilter, externalLabelFilter, nil
}

func extractExternalLabelRecordFilter(filterMap map[string]interface{}) (repositories.ExternalLabelRecordFilter, error) {
	var filter repositories.ExternalLabelRecordFilter
	if hasVal, ok := filterMap["has"].(map[string]interface{}); ok && hasVal != nil {
		predicate, err := extractExternalLabelPredicate(hasVal)
		if err != nil {
			return repositories.ExternalLabelRecordFilter{}, fmt.Errorf("invalid externalLabels.has filter: %w", err)
		}
		filter.Has = &predicate
	}
	if noneVal, ok := filterMap["none"].(map[string]interface{}); ok && noneVal != nil {
		predicate, err := extractExternalLabelPredicate(noneVal)
		if err != nil {
			return repositories.ExternalLabelRecordFilter{}, fmt.Errorf("invalid externalLabels.none filter: %w", err)
		}
		filter.None = &predicate
	}
	return filter, nil
}

func extractExternalLabelPredicate(predicateMap map[string]interface{}) (repositories.ExternalLabelPredicate, error) {
	predicate := repositories.ExternalLabelPredicate{ActiveOnly: true}
	if activeOnly, ok := predicateMap["activeOnly"].(bool); ok {
		predicate.ActiveOnly = activeOnly
	}

	if srcFilter, ok := predicateMap["src"].(map[string]interface{}); ok && srcFilter != nil {
		filters, err := extractExternalLabelStringFilters(srcFilter)
		if err != nil {
			return repositories.ExternalLabelPredicate{}, fmt.Errorf("invalid src filter: %w", err)
		}
		predicate.Sources = filters
	}
	if valFilter, ok := predicateMap["val"].(map[string]interface{}); ok && valFilter != nil {
		filters, err := extractExternalLabelStringFilters(valFilter)
		if err != nil {
			return repositories.ExternalLabelPredicate{}, fmt.Errorf("invalid val filter: %w", err)
		}
		predicate.Values = filters
	}

	return predicate, nil
}

func extractExternalLabelStringFilters(filterMap map[string]interface{}) ([]repositories.ExternalLabelStringFilter, error) {
	filters := make([]repositories.ExternalLabelStringFilter, 0, len(filterMap))
	for op, val := range filterMap {
		if val == nil {
			continue
		}
		switch op {
		case "eq", "neq", "in", "contains", "startsWith", "isNull":
			filters = append(filters, repositories.ExternalLabelStringFilter{Operator: op, Value: val})
		default:
			return nil, fmt.Errorf("unsupported operator %q", op)
		}
	}
	return filters, nil
}

// isTotalCountRequested checks whether the GraphQL query selected the totalCount field.
// This is used to avoid executing an expensive COUNT query when totalCount is not needed.
func isTotalCountRequested(p graphql.ResolveParams) bool {
	for _, field := range p.Info.FieldASTs {
		if field.SelectionSet == nil {
			continue
		}
		for _, sel := range field.SelectionSet.Selections {
			if f, ok := sel.(*ast.Field); ok && f.Name.Value == "totalCount" {
				return true
			}
		}
	}
	return false
}

func isFieldPathSelected(p graphql.ResolveParams, fieldPath ...string) bool {
	for _, field := range p.Info.FieldASTs {
		if selectionSetHasFieldPath(p.Info, field.SelectionSet, fieldPath, map[string]bool{}) {
			return true
		}
	}
	return false
}

func selectionSetHasFieldPath(info graphql.ResolveInfo, selectionSet *ast.SelectionSet, fieldPath []string, visitedFragments map[string]bool) bool {
	if selectionSet == nil || len(fieldPath) == 0 {
		return false
	}

	for _, selection := range selectionSet.Selections {
		switch selected := selection.(type) {
		case *ast.Field:
			if selected.Name == nil || selected.Name.Value != fieldPath[0] {
				continue
			}
			if len(fieldPath) == 1 {
				return true
			}
			if selectionSetHasFieldPath(info, selected.SelectionSet, fieldPath[1:], visitedFragments) {
				return true
			}
		case *ast.InlineFragment:
			if selectionSetHasFieldPath(info, selected.SelectionSet, fieldPath, visitedFragments) {
				return true
			}
		case *ast.FragmentSpread:
			if selected.Name == nil || visitedFragments[selected.Name.Value] {
				continue
			}
			fragment, ok := info.Fragments[selected.Name.Value].(*ast.FragmentDefinition)
			if !ok {
				continue
			}
			visitedFragments[selected.Name.Value] = true
			if selectionSetHasFieldPath(info, fragment.SelectionSet, fieldPath, visitedFragments) {
				return true
			}
		}
	}

	return false
}

type externalLabelHydrationRequirements struct {
	active  bool
	history bool
}

func (r externalLabelHydrationRequirements) any() bool {
	return r.active || r.history
}

func (r *externalLabelHydrationRequirements) merge(other externalLabelHydrationRequirements) {
	r.active = r.active || other.active
	r.history = r.history || other.history
}

type externalLabelHydration struct {
	active  map[string][]repositories.ExternalLabel
	history map[string][]repositories.ExternalLabel
}

func (b *Builder) hydrateExternalLabelsForConnection(p graphql.ResolveParams, repos *resolver.Repositories, records []*repositories.Record) (*externalLabelHydration, error) {
	requirements := externalLabelHydrationRequirementsForPath(p, "edges", "node", "externalLabels")
	if !requirements.any() {
		return nil, nil
	}
	return b.hydrateExternalLabelsForRecords(p, repos, records, requirements)
}

func (b *Builder) hydrateExternalLabelsForSingleRecord(p graphql.ResolveParams, repos *resolver.Repositories, rec *repositories.Record) (*externalLabelHydration, error) {
	requirements := externalLabelHydrationRequirementsForPath(p, "externalLabels")
	if !requirements.any() {
		return nil, nil
	}
	return b.hydrateExternalLabelsForRecords(p, repos, []*repositories.Record{rec}, requirements)
}

func externalLabelHydrationRequirementsForPath(p graphql.ResolveParams, fieldPath ...string) externalLabelHydrationRequirements {
	var requirements externalLabelHydrationRequirements
	for _, field := range p.Info.FieldASTs {
		requirements.merge(selectionSetExternalLabelHydrationRequirements(p.Info, field.SelectionSet, fieldPath, map[string]bool{}))
	}
	return requirements
}

func selectionSetExternalLabelHydrationRequirements(info graphql.ResolveInfo, selectionSet *ast.SelectionSet, fieldPath []string, visitedFragments map[string]bool) externalLabelHydrationRequirements {
	if selectionSet == nil || len(fieldPath) == 0 {
		return externalLabelHydrationRequirements{}
	}

	var requirements externalLabelHydrationRequirements
	for _, selection := range selectionSet.Selections {
		switch selected := selection.(type) {
		case *ast.Field:
			if selected.Name == nil || selected.Name.Value != fieldPath[0] {
				continue
			}
			if len(fieldPath) == 1 {
				requirements.merge(externalLabelHydrationRequirementsForField(info, selected))
				continue
			}
			requirements.merge(selectionSetExternalLabelHydrationRequirements(info, selected.SelectionSet, fieldPath[1:], visitedFragments))
		case *ast.InlineFragment:
			requirements.merge(selectionSetExternalLabelHydrationRequirements(info, selected.SelectionSet, fieldPath, visitedFragments))
		case *ast.FragmentSpread:
			if selected.Name == nil || visitedFragments[selected.Name.Value] {
				continue
			}
			fragment, ok := info.Fragments[selected.Name.Value].(*ast.FragmentDefinition)
			if !ok {
				continue
			}
			visitedFragments[selected.Name.Value] = true
			requirements.merge(selectionSetExternalLabelHydrationRequirements(info, fragment.SelectionSet, fieldPath, visitedFragments))
		}
	}
	return requirements
}

func externalLabelHydrationRequirementsForField(info graphql.ResolveInfo, field *ast.Field) externalLabelHydrationRequirements {
	activeOnly, known := externalLabelActiveOnlyArgument(info, field)
	if !known {
		return externalLabelHydrationRequirements{active: true, history: true}
	}
	if activeOnly {
		return externalLabelHydrationRequirements{active: true}
	}
	return externalLabelHydrationRequirements{history: true}
}

func externalLabelActiveOnlyArgument(info graphql.ResolveInfo, field *ast.Field) (bool, bool) {
	for _, arg := range field.Arguments {
		if arg.Name == nil || arg.Name.Value != "activeOnly" {
			continue
		}
		switch value := arg.Value.(type) {
		case *ast.BooleanValue:
			return value.Value, true
		case *ast.Variable:
			if value.Name == nil {
				return false, false
			}
			variableValue, ok := info.VariableValues[value.Name.Value]
			if !ok {
				return false, false
			}
			if variableValue == nil {
				return true, true
			}
			activeOnly, ok := variableValue.(bool)
			return activeOnly, ok
		default:
			return false, false
		}
	}
	return true, true
}

func (b *Builder) hydrateExternalLabelsForRecords(p graphql.ResolveParams, repos *resolver.Repositories, records []*repositories.Record, requirements externalLabelHydrationRequirements) (*externalLabelHydration, error) {
	if repos == nil || repos.ExternalLabels == nil || len(records) == 0 || !requirements.any() {
		return nil, nil
	}

	subjects := make([]repositories.LabelSubject, 0, len(records))
	for _, rec := range records {
		subjects = append(subjects, repositories.LabelSubject{URI: rec.URI, CID: rec.CID})
	}

	var activeLabelsBySubject map[string][]repositories.ExternalLabel
	if requirements.active {
		var err error
		activeLabelsBySubject, err = repos.ExternalLabels.GetBySubjects(p.Context, subjects, repositories.ExternalLabelFilter{ActiveOnly: true})
		if err != nil {
			return nil, fmt.Errorf("failed to hydrate active external labels: %w", err)
		}
	}

	var historyLabelsBySubject map[string][]repositories.ExternalLabel
	if requirements.history {
		var err error
		historyLabelsBySubject, err = repos.ExternalLabels.GetBySubjects(p.Context, subjects, repositories.ExternalLabelFilter{ActiveOnly: false})
		if err != nil {
			return nil, fmt.Errorf("failed to hydrate historical external labels: %w", err)
		}
	}

	return &externalLabelHydration{
		active:  activeLabelsBySubject,
		history: historyLabelsBySubject,
	}, nil
}

func attachExternalLabels(node interface{}, rec *repositories.Record, hydration *externalLabelHydration) {
	if hydration == nil {
		return
	}
	nodeMap, ok := node.(map[string]interface{})
	if !ok {
		return
	}
	subject := repositories.LabelSubject{URI: rec.URI, CID: rec.CID}
	nodeMap[externallabels.ActiveSourceKey] = hydration.active[subject.Key()]
	nodeMap[externallabels.HistorySourceKey] = hydration.history[subject.Key()]
}

type certifiedProfileHydration struct {
	byDID map[string]map[string]interface{}
}

func (b *Builder) hydrateCertifiedProfilesForConnection(p graphql.ResolveParams, repos *resolver.Repositories, records []*repositories.Record) (*certifiedProfileHydration, error) {
	if !isFieldPathSelected(p, "edges", "node", "certifiedProfileData") {
		return nil, nil
	}
	externalLabelRequirements := externalLabelHydrationRequirementsForPath(p, "edges", "node", "certifiedProfileData", "externalLabels")
	return b.hydrateCertifiedProfilesForRecords(p, repos, records, externalLabelRequirements)
}

func (b *Builder) hydrateCertifiedProfileForSingleRecord(p graphql.ResolveParams, repos *resolver.Repositories, rec *repositories.Record) (*certifiedProfileHydration, error) {
	if !isFieldPathSelected(p, "certifiedProfileData") {
		return nil, nil
	}
	externalLabelRequirements := externalLabelHydrationRequirementsForPath(p, "certifiedProfileData", "externalLabels")
	return b.hydrateCertifiedProfilesForRecords(p, repos, []*repositories.Record{rec}, externalLabelRequirements)
}

func (b *Builder) hydrateCertifiedProfilesForRecords(p graphql.ResolveParams, repos *resolver.Repositories, records []*repositories.Record, externalLabelRequirements externalLabelHydrationRequirements) (*certifiedProfileHydration, error) {
	if repos == nil || repos.Records == nil || len(records) == 0 {
		return nil, nil
	}
	if _, ok := b.recordTypes[certifiedprofiles.CollectionID]; !ok {
		return nil, nil
	}

	urisByDID := make(map[string]string, len(records))
	uris := make([]string, 0, len(records))
	for _, rec := range records {
		if rec == nil || rec.DID == "" {
			continue
		}
		if _, exists := urisByDID[rec.DID]; exists {
			continue
		}
		uri := certifiedProfileURI(rec.DID)
		urisByDID[rec.DID] = uri
		uris = append(uris, uri)
	}
	if len(uris) == 0 {
		return &certifiedProfileHydration{byDID: map[string]map[string]interface{}{}}, nil
	}

	profileRecords, err := repos.Records.GetByURIs(p.Context, uris)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch certified profiles: %w", err)
	}

	var profileLabelHydration *externalLabelHydration
	if externalLabelRequirements.any() {
		profileLabelHydration, err = b.hydrateExternalLabelsForRecords(p, repos, profileRecords, externalLabelRequirements)
		if err != nil {
			return nil, fmt.Errorf("failed to hydrate certified profile external labels: %w", err)
		}
	}

	profilesByDID := make(map[string]map[string]interface{}, len(profileRecords))
	for _, profileRecord := range profileRecords {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(profileRecord.JSON), &data); err != nil {
			slog.Warn("Skipping certified profile with invalid JSON", "uri", profileRecord.URI, "error", err)
			continue
		}
		data["uri"] = profileRecord.URI
		data["cid"] = profileRecord.CID
		data["did"] = profileRecord.DID
		data["rkey"] = profileRecord.RKey
		b.coerceRequiredFields(data, certifiedprofiles.CollectionID)
		attachExternalLabels(data, profileRecord, profileLabelHydration)
		profilesByDID[profileRecord.DID] = data
	}

	return &certifiedProfileHydration{byDID: profilesByDID}, nil
}

func attachCertifiedProfileData(node interface{}, rec *repositories.Record, hydration *certifiedProfileHydration) {
	if hydration == nil || rec == nil {
		return
	}
	nodeMap, ok := node.(map[string]interface{})
	if !ok {
		return
	}
	profile, ok := hydration.byDID[rec.DID]
	if !ok || profile == nil {
		nodeMap[certifiedprofiles.SourceKey] = nil
		return
	}
	nodeMap[certifiedprofiles.SourceKey] = profile
}

func certifiedProfileURI(did string) string {
	return "at://" + did + "/" + certifiedprofiles.CollectionID + "/" + certifiedprofiles.RKey
}

// buildSortAwareCursor builds a sort-aware cursor string for a record.
// directSortCols mirrors the repository's directSortColumns set.
var directSortCols = map[string]bool{
	"indexed_at": true,
	"uri":        true,
	"did":        true,
	"collection": true,
	"cid":        true,
	"rkey":       true,
}

// sortFieldValueForRecord extracts the sort field value from a record for cursor building.
func sortFieldValueForRecord(rec *repositories.Record, value map[string]interface{}, sortOpt *repositories.SortOption) string {
	if sortOpt == nil {
		return rec.IndexedAt.UTC().Format(time.RFC3339Nano)
	}
	if directSortCols[sortOpt.Field] {
		switch sortOpt.Field {
		case "indexed_at":
			return rec.IndexedAt.UTC().Format(time.RFC3339Nano)
		case "uri":
			return rec.URI
		case "did":
			return rec.DID
		case "collection":
			return rec.Collection
		case "cid":
			return rec.CID
		case "rkey":
			return rec.RKey
		default:
			return rec.IndexedAt.Format("2006-01-02T15:04:05Z")
		}
	}
	// JSON field
	if v, exists := value[sortOpt.Field]; exists && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// resolveRecordConnection is the shared implementation for paginated record queries.
// It uses deterministic keyset pagination with a composite (sortField, uri) cursor.
// Supports both forward pagination (first/after) and backward pagination (last/before).
func (b *Builder) resolveRecordConnection(
	p graphql.ResolveParams,
	collection string,
	buildNode nodeBuilder,
) (interface{}, error) {
	repos := resolver.GetRepositories(p.Context)
	if repos == nil || repos.Records == nil {
		return emptyConnection(), nil
	}

	// Extract pagination args
	firstArg, hasFirst := p.Args["first"].(int)
	after, _ := p.Args["after"].(string)
	lastArg, hasLast := p.Args["last"].(int)
	before, _ := p.Args["before"].(string)

	// Validate: cannot use both first/after and last/before
	if (hasFirst || after != "") && (hasLast || before != "") {
		return nil, fmt.Errorf("cannot use both first/after and last/before")
	}

	// Extract where filters if present (typed collection queries only)
	var filters []repositories.FieldFilter
	var didFilter repositories.DIDFilter
	var externalLabelFilter repositories.ExternalLabelRecordFilter
	if whereArg, ok := p.Args["where"]; ok && whereArg != nil {
		var err error
		filters, didFilter, externalLabelFilter, err = extractFiltersWithExternalLabels(whereArg, collection, b.registry)
		if err != nil {
			return nil, err
		}
	}

	// Extract sort args if present (typed collection queries only)
	var sortOpt *repositories.SortOption
	if sortByArg, ok := p.Args["sortBy"].(string); ok && sortByArg != "" {
		direction := "DESC" // default
		if dirArg, ok := p.Args["sortDirection"].(string); ok && dirArg != "" {
			direction = dirArg
		}
		sortOpt = &repositories.SortOption{Field: sortByArg, Direction: direction}
	}

	// Backward pagination path
	if hasLast || before != "" {
		last := query.ClampPageSize(lastArg)

		// Decode before cursor if provided
		var beforeCursorValues []string
		if before != "" {
			parts, err := decodeCursorValues(before)
			if err != nil {
				return nil, fmt.Errorf("invalid cursor: %w", err)
			}
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid cursor: expected 2 components")
			}
			beforeCursorValues = parts
		}

		// Fetch last+1 to detect hasPreviousPage
		records, err := repos.Records.GetByCollectionReversedWithKeysetCursorAndExternalLabels(p.Context, collection, filters, didFilter, externalLabelFilter, sortOpt, last+1, beforeCursorValues)
		if err != nil {
			return nil, fmt.Errorf("failed to query records: %w", err)
		}

		// Determine if there are more results before the returned page.
		// After reversal, the extra record is at the front (oldest end).
		hasPreviousPage := len(records) > last
		if hasPreviousPage {
			records = records[1:]
		}

		labelsBySubject, err := b.hydrateExternalLabelsForConnection(p, repos, records)
		if err != nil {
			return nil, err
		}
		certifiedProfiles, err := b.hydrateCertifiedProfilesForConnection(p, repos, records)
		if err != nil {
			return nil, err
		}

		// Build edges
		edges := make([]interface{}, 0, len(records))
		var startCursor, endCursor string

		for _, rec := range records {
			var value map[string]interface{}
			if err := json.Unmarshal([]byte(rec.JSON), &value); err != nil {
				slog.Warn("Skipping record with invalid JSON", "uri", rec.URI, "error", err)
				continue
			}

			node, ok := buildNode(rec, value)
			if !ok {
				continue
			}
			attachExternalLabels(node, rec, labelsBySubject)
			attachCertifiedProfileData(node, rec, certifiedProfiles)

			cursor := encodeCursorValues(sortFieldValueForRecord(rec, value, sortOpt), rec.URI)
			if startCursor == "" {
				startCursor = cursor
			}
			endCursor = cursor

			edges = append(edges, map[string]interface{}{
				"cursor": cursor,
				"node":   node,
			})
		}

		result := map[string]interface{}{
			"edges": edges,
			"pageInfo": map[string]interface{}{
				"hasNextPage":     before != "",
				"hasPreviousPage": hasPreviousPage,
				"startCursor":     startCursor,
				"endCursor":       endCursor,
			},
		}

		if isTotalCountRequested(p) {
			count, err := repos.Records.GetCollectionCountFilteredWithExternalLabels(p.Context, collection, filters, didFilter, externalLabelFilter)
			if err == nil {
				result["totalCount"] = int(count)
			}
		}

		return result, nil
	}

	// Forward pagination path (default)
	first := query.ClampPageSize(firstArg)

	// Decode composite cursor if provided
	var afterCursorValues []string
	if after != "" {
		var err error
		afterCursorValues, err = decodeCursorValues(after)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		// Ensure we have exactly 2 values for keyset pagination
		if len(afterCursorValues) != 2 {
			return nil, fmt.Errorf("invalid cursor: expected 2 components")
		}
	}

	// Fetch first+1 to determine hasNextPage using the sorted method
	records, err := repos.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(p.Context, collection, filters, didFilter, externalLabelFilter, sortOpt, first+1, afterCursorValues)
	if err != nil {
		return nil, fmt.Errorf("failed to query records: %w", err)
	}

	// Determine if there are more results
	hasNextPage := len(records) > first
	if hasNextPage {
		records = records[:first]
	}

	labelsBySubject, err := b.hydrateExternalLabelsForConnection(p, repos, records)
	if err != nil {
		return nil, err
	}
	certifiedProfiles, err := b.hydrateCertifiedProfilesForConnection(p, repos, records)
	if err != nil {
		return nil, err
	}

	// Build edges with sort-aware cursors
	edges := make([]interface{}, 0, len(records))
	var startCursor, endCursor string

	for _, rec := range records {
		var value map[string]interface{}
		if err := json.Unmarshal([]byte(rec.JSON), &value); err != nil {
			slog.Warn("Skipping record with invalid JSON", "uri", rec.URI, "error", err)
			continue
		}

		node, ok := buildNode(rec, value)
		if !ok {
			continue
		}
		attachExternalLabels(node, rec, labelsBySubject)
		attachCertifiedProfileData(node, rec, certifiedProfiles)

		cursor := encodeCursorValues(sortFieldValueForRecord(rec, value, sortOpt), rec.URI)
		if startCursor == "" {
			startCursor = cursor
		}
		endCursor = cursor

		edges = append(edges, map[string]interface{}{
			"cursor": cursor,
			"node":   node,
		})
	}

	result := map[string]interface{}{
		"edges": edges,
		"pageInfo": map[string]interface{}{
			"hasNextPage":     hasNextPage,
			"hasPreviousPage": after != "",
			"startCursor":     startCursor,
			"endCursor":       endCursor,
		},
	}

	if isTotalCountRequested(p) {
		count, err := repos.Records.GetCollectionCountFilteredWithExternalLabels(p.Context, collection, filters, didFilter, externalLabelFilter)
		if err == nil {
			result["totalCount"] = int(count)
		}
	}

	return result, nil
}

// createExternalLabelsResolver creates a resolver for generic external label subject lookups.
func (b *Builder) createExternalLabelsResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.ExternalLabels == nil {
			return []map[string]interface{}{}, nil
		}

		subjectValues := stringListArg(p.Args["subjects"])
		if len(subjectValues) == 0 {
			return []map[string]interface{}{}, nil
		}

		subjects := make([]repositories.LabelSubject, 0, len(subjectValues))
		for _, subject := range subjectValues {
			subjects = append(subjects, repositories.LabelSubject{URI: subject})
		}

		filter := externallabels.FilterFromArgs(p.Args)
		labelsBySubject, err := repos.ExternalLabels.GetBySubjects(p.Context, subjects, repositories.ExternalLabelFilter{
			Sources:    filter.Sources,
			Values:     filter.Values,
			ActiveOnly: filter.ActiveOnly,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query external labels: %w", err)
		}

		labels := make([]repositories.ExternalLabel, 0)
		for _, subject := range subjects {
			labels = append(labels, labelsBySubject[subject.Key()]...)
		}

		return externallabels.ToGraphQL(labels), nil
	}
}

func stringListArg(value interface{}) []string {
	switch v := value.(type) {
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				items = append(items, s)
			}
		}
		return items
	case []string:
		return v
	default:
		return nil
	}
}

// createSearchResolver creates a resolver for the search query.
// It validates the query string (minimum 3 runes) and calls the Search repository method.
func (b *Builder) createSearchResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		searchQuery, _ := p.Args["query"].(string)
		if utf8.RuneCountInString(searchQuery) < 3 {
			return nil, fmt.Errorf("search query must be at least 3 characters")
		}

		collection, _ := p.Args["collection"].(string)

		firstArg, _ := p.Args["first"].(int)
		first := query.ClampPageSize(firstArg)

		after, _ := p.Args["after"].(string)

		var afterTimestamp, afterURI string
		if after != "" {
			var err error
			afterTimestamp, afterURI, err = decodeCursor(after)
			if err != nil {
				return nil, fmt.Errorf("invalid cursor: %w", err)
			}
		}

		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			return emptyConnection(), nil
		}

		records, err := repos.Records.Search(p.Context, searchQuery, collection, first+1, afterTimestamp, afterURI)
		if err != nil {
			return nil, fmt.Errorf("failed to search records: %w", err)
		}

		hasNextPage := len(records) > first
		if hasNextPage {
			records = records[:first]
		}

		labelsBySubject, err := b.hydrateExternalLabelsForConnection(p, repos, records)
		if err != nil {
			return nil, err
		}
		certifiedProfiles, err := b.hydrateCertifiedProfilesForConnection(p, repos, records)
		if err != nil {
			return nil, err
		}

		edges := make([]interface{}, 0, len(records))
		var startCursor, endCursor string

		for _, rec := range records {
			var value map[string]interface{}
			if err := json.Unmarshal([]byte(rec.JSON), &value); err != nil {
				slog.Warn("Skipping record with invalid JSON", "uri", rec.URI, "error", err)
				continue
			}

			cursor := encodeCursor(rec.IndexedAt.Format("2006-01-02T15:04:05Z"), rec.URI)
			if startCursor == "" {
				startCursor = cursor
			}
			endCursor = cursor

			node := map[string]interface{}{
				"uri":        rec.URI,
				"cid":        rec.CID,
				"did":        rec.DID,
				"collection": rec.Collection,
				"rkey":       rec.RKey,
				"value":      value,
			}
			attachExternalLabels(node, rec, labelsBySubject)
			attachCertifiedProfileData(node, rec, certifiedProfiles)

			edges = append(edges, map[string]interface{}{
				"cursor": cursor,
				"node":   node,
			})
		}

		return map[string]interface{}{
			"edges": edges,
			"pageInfo": map[string]interface{}{
				"hasNextPage":     hasNextPage,
				"hasPreviousPage": after != "",
				"startCursor":     startCursor,
				"endCursor":       endCursor,
			},
		}, nil
	}
}

// createGenericRecordsResolver creates a resolver for the generic records query.
func (b *Builder) createGenericRecordsResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		collection, ok := p.Args["collection"].(string)
		if !ok || collection == "" {
			return nil, fmt.Errorf("collection is required")
		}

		return b.resolveRecordConnection(p, collection,
			func(rec *repositories.Record, value map[string]interface{}) (interface{}, bool) {
				return map[string]interface{}{
					"uri":        rec.URI,
					"cid":        rec.CID,
					"did":        rec.DID,
					"collection": rec.Collection,
					"rkey":       rec.RKey,
					"value":      value,
				}, true
			})
	}
}

// coerceRequiredFields fills in zero values for required fields that are missing or null.
// This prevents NonNull violations when historical records lack fields that became required.
func (b *Builder) coerceRequiredFields(data map[string]interface{}, collection string) {
	recordDef, ok := b.registry.GetRecordDef(collection)
	if !ok {
		return
	}
	for _, entry := range recordDef.RequiredProperties() {
		val, exists := data[entry.Name]
		if exists && val != nil {
			continue
		}
		zero := lexicon.ZeroValueForType(entry.Property.Type)
		if zero == nil {
			// Complex type (ref, union, blob, etc.) — skip, keep nil
			continue
		}
		slog.Debug("Coercing missing required field to zero value",
			"collection", collection,
			"field", entry.Name,
			"type", entry.Property.Type,
		)
		data[entry.Name] = zero
	}
}

// createCollectionResolver creates a resolver for querying a typed collection.
func (b *Builder) createCollectionResolver(lexiconID string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		return b.resolveRecordConnection(p, lexiconID,
			func(rec *repositories.Record, data map[string]interface{}) (interface{}, bool) {
				// Inject standard record fields into the flat data
				data["uri"] = rec.URI
				data["cid"] = rec.CID
				data["did"] = rec.DID
				data["rkey"] = rec.RKey
				b.coerceRequiredFields(data, lexiconID)
				return data, true
			})
	}
}

// createSingleRecordResolver creates a resolver for fetching a single record.
func (b *Builder) createSingleRecordResolver(lexiconID string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		uri, ok := p.Args["uri"].(string)
		if !ok {
			return nil, fmt.Errorf("uri is required")
		}

		// Get repositories from context
		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			return nil, nil
		}

		// Query database
		rec, err := repos.Records.GetByURI(p.Context, uri)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil // Not found
			}
			return nil, fmt.Errorf("failed to fetch record: %w", err)
		}

		// Parse JSON to map
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(rec.JSON), &data); err != nil {
			return nil, fmt.Errorf("failed to parse record JSON: %w", err)
		}

		// Add standard record fields
		data["uri"] = rec.URI
		data["cid"] = rec.CID
		data["did"] = rec.DID
		data["rkey"] = rec.RKey
		b.coerceRequiredFields(data, lexiconID)

		labelsBySubject, err := b.hydrateExternalLabelsForSingleRecord(p, repos, rec)
		if err != nil {
			return nil, err
		}
		certifiedProfiles, err := b.hydrateCertifiedProfileForSingleRecord(p, repos, rec)
		if err != nil {
			return nil, err
		}
		attachExternalLabels(data, rec, labelsBySubject)
		attachCertifiedProfileData(data, rec, certifiedProfiles)

		return data, nil
	}
}

// createCollectionStatsResolver creates a resolver for collection statistics.
func (b *Builder) createCollectionStatsResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		// Get repositories from context
		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			return []interface{}{}, nil
		}

		// Extract optional collections filter
		var collections []string
		if collectionsArg, ok := p.Args["collections"].([]interface{}); ok {
			for _, c := range collectionsArg {
				if s, ok := c.(string); ok {
					collections = append(collections, s)
				}
			}
		}

		// Query database
		stats, err := repos.Records.GetCollectionStatsFiltered(p.Context, collections)
		if err != nil {
			return nil, fmt.Errorf("failed to get collection stats: %w", err)
		}

		// Convert to interface slice for GraphQL
		result := make([]interface{}, len(stats))
		for i, stat := range stats {
			result[i] = map[string]interface{}{
				"collection": stat.Collection,
				"count":      stat.Count,
			}
		}

		return result, nil
	}
}

// createCollectionTimeSeriesResolver creates a resolver for collection time series data.
func (b *Builder) createCollectionTimeSeriesResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		collection, ok := p.Args["collection"].(string)
		if !ok || collection == "" {
			return nil, fmt.Errorf("collection is required")
		}

		// Get repositories from context
		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			return nil, nil
		}

		// Query database
		timeSeries, err := repos.Records.GetCollectionTimeSeries(p.Context, collection)
		if err != nil {
			return nil, fmt.Errorf("failed to get collection time series: %w", err)
		}

		// Convert data points to interface slice
		dataPoints := make([]interface{}, len(timeSeries.Data))
		for i, point := range timeSeries.Data {
			dataPoints[i] = map[string]interface{}{
				"date":       point.Date,
				"count":      point.Count,
				"cumulative": point.Cumulative,
			}
		}

		return map[string]interface{}{
			"collection":   timeSeries.Collection,
			"totalRecords": timeSeries.TotalRecords,
			"uniqueUsers":  timeSeries.UniqueUsers,
			"data":         dataPoints,
		}, nil
	}
}

// emptyConnection returns an empty Relay connection.
func emptyConnection() map[string]interface{} {
	return map[string]interface{}{
		"edges": []interface{}{},
		"pageInfo": map[string]interface{}{
			"hasNextPage":     false,
			"hasPreviousPage": false,
			"startCursor":     nil,
			"endCursor":       nil,
		},
		"totalCount": 0,
	}
}

// encodeCursorValues encodes multiple cursor component values into a base64 string.
// Uses JSON array encoding to safely handle values that contain the pipe character.
func encodeCursorValues(values ...string) string {
	jsonBytes, _ := json.Marshal(values)
	return base64.URLEncoding.EncodeToString(jsonBytes)
}

// decodeCursorValues decodes a base64 cursor into its component values.
// Supports both the current JSON array format and the legacy pipe-delimited format
// for backward compatibility with cursors issued before the JSON encoding change.
func decodeCursorValues(cursor string) ([]string, error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	var parts []string
	if err := json.Unmarshal(data, &parts); err != nil {
		// Backward compatibility: try legacy pipe-delimited format.
		parts = strings.Split(string(data), "|")
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid cursor format")
		}
	}
	return parts, nil
}

// encodeCursor encodes a composite (indexed_at, uri) cursor as base64.
// Kept for backward compatibility; delegates to encodeCursorValues.
func encodeCursor(indexedAt, uri string) string {
	return encodeCursorValues(indexedAt, uri)
}

// decodeCursor decodes a base64 cursor into (indexed_at, uri) components.
// Kept for backward compatibility; delegates to decodeCursorValues.
func decodeCursor(cursor string) (string, string, error) {
	parts, err := decodeCursorValues(cursor)
	if err != nil {
		return "", "", err
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed cursor: expected 'timestamp|uri'")
	}
	return parts[0], parts[1], nil
}

// GetRecordType returns the GraphQL type for a record.
func (b *Builder) GetRecordType(lexiconID string) *graphql.Object {
	return b.recordTypes[lexiconID]
}

// GetConnectionType returns the connection type for a record.
func (b *Builder) GetConnectionType(lexiconID string) *graphql.Object {
	return b.connectionTypes[lexiconID]
}
