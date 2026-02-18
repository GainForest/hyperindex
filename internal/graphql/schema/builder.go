// Package schema provides the GraphQL schema builder.
package schema

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/graphql/query"
	"github.com/GainForest/hypergoat/internal/graphql/resolver"
	"github.com/GainForest/hypergoat/internal/graphql/subscription"
	"github.com/GainForest/hypergoat/internal/graphql/types"
	"github.com/GainForest/hypergoat/internal/lexicon"
)

// Builder builds a GraphQL schema from lexicon definitions.
type Builder struct {
	registry      *lexicon.Registry
	mapper        *types.Mapper
	objectBuilder *types.ObjectBuilder

	// Built types
	recordTypes     map[string]*graphql.Object // lexiconID -> record type
	connectionTypes map[string]*graphql.Object // lexiconID -> connection type
	sortFieldEnums  map[string]*graphql.Enum   // lexiconID -> sort field enum
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
	}
}

// Build builds the complete GraphQL schema.
func (b *Builder) Build() (*graphql.Schema, error) {
	// Phase 1: Build all object types (non-record helper types)
	b.buildObjectTypes()

	// Phase 2: Build all record types
	b.buildRecordTypes()

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
				return event.Record, nil
			},
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   "Subscription",
		Fields: fields,
	})
}

// Generic record type for the records query
var genericRecordType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "GenericRecord",
	Description: "A generic AT Protocol record",
	Fields: graphql.Fields{
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
	},
})

// Generic record edge for pagination
var genericRecordEdgeType = graphql.NewObject(graphql.ObjectConfig{
	Name: "GenericRecordEdge",
	Fields: graphql.Fields{
		"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"node":   &graphql.Field{Type: genericRecordType},
	},
})

// Generic record connection for pagination
var genericRecordConnectionType = graphql.NewObject(graphql.ObjectConfig{
	Name: "GenericRecordConnection",
	Fields: graphql.Fields{
		"edges":    &graphql.Field{Type: graphql.NewList(genericRecordEdgeType)},
		"pageInfo": &graphql.Field{Type: query.PageInfoType},
	},
})

// buildQueryType builds the root Query type with fields for each collection.
func (b *Builder) buildQueryType() *graphql.Object {
	fields := graphql.Fields{}

	// Add generic records query that works for any collection
	fields["records"] = &graphql.Field{
		Type:        genericRecordConnectionType,
		Description: "Query records from any collection (useful for collections without lexicon schemas)",
		Args: graphql.FieldConfigArgument{
			"collection": &graphql.ArgumentConfig{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "Collection NSID (e.g., org.impactindexer.review.like)",
			},
			"first": &graphql.ArgumentConfig{
				Type:         graphql.Int,
				DefaultValue: 20,
				Description:  "Number of records to return",
			},
			"after": &graphql.ArgumentConfig{
				Type:        graphql.String,
				Description: "Cursor for pagination",
			},
		},
		Resolve: b.createGenericRecordsResolver(),
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

// resolveRecordConnection is the shared implementation for paginated record queries.
// It uses deterministic keyset pagination with a composite (indexed_at, uri) cursor.
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
	firstArg, _ := p.Args["first"].(int)
	first := query.ClampPageSize(firstArg)
	after, _ := p.Args["after"].(string)

	// Decode composite cursor if provided
	var afterTimestamp, afterURI string
	if after != "" {
		var err error
		afterTimestamp, afterURI, err = decodeCursor(after)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
	}

	// Fetch first+1 to determine hasNextPage
	records, err := repos.Records.GetByCollectionWithKeysetCursor(p.Context, collection, first+1, afterTimestamp, afterURI)
	if err != nil {
		return nil, fmt.Errorf("failed to query records: %w", err)
	}

	// Determine if there are more results
	hasNextPage := len(records) > first
	if hasNextPage {
		records = records[:first]
	}

	// Build edges
	edges := make([]interface{}, 0, len(records))
	var startCursor, endCursor string

	for _, rec := range records {
		var value map[string]interface{}
		if err := json.Unmarshal([]byte(rec.JSON), &value); err != nil {
			continue // Skip records with invalid JSON
		}

		node, ok := buildNode(rec, value)
		if !ok {
			continue
		}

		cursor := encodeCursor(rec.IndexedAt.Format("2006-01-02T15:04:05Z"), rec.URI)
		if startCursor == "" {
			startCursor = cursor
		}
		endCursor = cursor

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

// encodeCursor encodes a composite (indexed_at, uri) cursor as base64.
func encodeCursor(indexedAt, uri string) string {
	return base64.URLEncoding.EncodeToString([]byte(indexedAt + "|" + uri))
}

// decodeCursor decodes a base64 cursor into (indexed_at, uri) components.
func decodeCursor(cursor string) (string, string, error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(data), "|", 2)
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
