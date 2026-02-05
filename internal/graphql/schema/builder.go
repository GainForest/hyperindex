// Package schema provides the GraphQL schema builder.
package schema

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

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

	for lexiconID, connectionType := range b.connectionTypes {
		fieldName := lexicon.ToFieldName(lexiconID)

		fields[fieldName] = &graphql.Field{
			Type:        connectionType,
			Description: fmt.Sprintf("Query %s records", lexiconID),
			Args:        query.ConnectionArgs(),
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

// recordWithTimestamp holds a record and its effective timestamp for sorting
type recordWithTimestamp struct {
	record    *repositories.Record
	value     map[string]interface{}
	timestamp time.Time
}

// getRecordTimestamp extracts the best timestamp for sorting:
// prefers createdAt from JSON value, falls back to indexed_at
func getRecordTimestamp(rec *repositories.Record, value map[string]interface{}) time.Time {
	// Try to get createdAt from the record value
	if createdAt, ok := value["createdAt"].(string); ok && createdAt != "" {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			return t
		}
		// Try other formats
		if t, err := time.Parse("2006-01-02T15:04:05.999999Z", createdAt); err == nil {
			return t
		}
	}
	// Fall back to indexed_at
	return rec.IndexedAt
}

// createGenericRecordsResolver creates a resolver for the generic records query.
func (b *Builder) createGenericRecordsResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		collection, ok := p.Args["collection"].(string)
		if !ok || collection == "" {
			return nil, fmt.Errorf("collection is required")
		}

		// Get repositories from context
		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			return emptyConnection(), nil
		}

		// Extract pagination args
		first, _ := p.Args["first"].(int)
		if first == 0 {
			first = 20
		}
		after, _ := p.Args["after"].(string)

		// Decode cursor to timestamp if provided
		var afterTimestamp string
		if after != "" {
			var err error
			afterTimestamp, err = decodeCursor(after)
			if err != nil {
				return nil, fmt.Errorf("invalid cursor: %w", err)
			}
		}

		// Fetch extra records to allow for re-sorting by createdAt
		fetchLimit := first * 2
		if fetchLimit < 50 {
			fetchLimit = 50
		}

		// Query database with cursor (ordered by indexed_at DESC)
		records, err := repos.Records.GetByCollectionWithCursor(p.Context, collection, fetchLimit, afterTimestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to query records: %w", err)
		}

		// Parse JSON and extract timestamps for sorting
		recordsWithTs := make([]recordWithTimestamp, 0, len(records))
		for _, rec := range records {
			var value map[string]interface{}
			if err := json.Unmarshal([]byte(rec.JSON), &value); err != nil {
				value = map[string]interface{}{"raw": rec.JSON}
			}
			recordsWithTs = append(recordsWithTs, recordWithTimestamp{
				record:    rec,
				value:     value,
				timestamp: getRecordTimestamp(rec, value),
			})
		}

		// Sort by effective timestamp (createdAt or indexed_at) DESC
		sort.Slice(recordsWithTs, func(i, j int) bool {
			return recordsWithTs[i].timestamp.After(recordsWithTs[j].timestamp)
		})

		// Filter by cursor timestamp if provided (for proper pagination after re-sort)
		if afterTimestamp != "" {
			cursorTime, _ := time.Parse("2006-01-02 15:04:05", afterTimestamp)
			filtered := make([]recordWithTimestamp, 0, len(recordsWithTs))
			for _, r := range recordsWithTs {
				if r.timestamp.Before(cursorTime) {
					filtered = append(filtered, r)
				}
			}
			recordsWithTs = filtered
		}

		// Determine if there are more results
		hasNextPage := len(recordsWithTs) > first
		if hasNextPage {
			recordsWithTs = recordsWithTs[:first]
		}

		// Build edges
		edges := make([]interface{}, 0, len(recordsWithTs))
		var startCursor, endCursor string

		for _, r := range recordsWithTs {
			// Use effective timestamp as cursor
			cursor := encodeCursor(r.timestamp.Format("2006-01-02 15:04:05"))
			if startCursor == "" {
				startCursor = cursor
			}
			endCursor = cursor

			edges = append(edges, map[string]interface{}{
				"cursor": cursor,
				"node": map[string]interface{}{
					"uri":        r.record.URI,
					"cid":        r.record.CID,
					"did":        r.record.DID,
					"collection": r.record.Collection,
					"rkey":       r.record.RKey,
					"value":      r.value,
				},
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

// createCollectionResolver creates a resolver for querying a collection.
func (b *Builder) createCollectionResolver(lexiconID string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		// Get repositories from context
		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			// Return empty connection if no repos (for tests without DB)
			return emptyConnection(), nil
		}

		// Extract pagination args
		first, _ := p.Args["first"].(int)
		after, _ := p.Args["after"].(string)

		// Default limit
		if first == 0 {
			first = 20
		}

		// Decode cursor to timestamp if provided
		var afterTimestamp string
		if after != "" {
			var err error
			afterTimestamp, err = decodeCursor(after)
			if err != nil {
				return nil, fmt.Errorf("invalid cursor: %w", err)
			}
		}

		// Fetch extra records to allow for re-sorting by createdAt
		fetchLimit := first * 2
		if fetchLimit < 50 {
			fetchLimit = 50
		}

		// Query database with cursor (ordered by indexed_at DESC)
		records, err := repos.Records.GetByCollectionWithCursor(p.Context, lexiconID, fetchLimit, afterTimestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to query records: %w", err)
		}

		// Parse JSON and extract timestamps for sorting
		recordsWithTs := make([]recordWithTimestamp, 0, len(records))
		for _, rec := range records {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(rec.JSON), &data); err != nil {
				continue // Skip records with invalid JSON
			}
			// Add standard record fields
			data["uri"] = rec.URI
			data["cid"] = rec.CID

			recordsWithTs = append(recordsWithTs, recordWithTimestamp{
				record:    rec,
				value:     data,
				timestamp: getRecordTimestamp(rec, data),
			})
		}

		// Sort by effective timestamp (createdAt or indexed_at) DESC
		sort.Slice(recordsWithTs, func(i, j int) bool {
			return recordsWithTs[i].timestamp.After(recordsWithTs[j].timestamp)
		})

		// Filter by cursor timestamp if provided (for proper pagination after re-sort)
		if afterTimestamp != "" {
			cursorTime, _ := time.Parse("2006-01-02 15:04:05", afterTimestamp)
			filtered := make([]recordWithTimestamp, 0, len(recordsWithTs))
			for _, r := range recordsWithTs {
				if r.timestamp.Before(cursorTime) {
					filtered = append(filtered, r)
				}
			}
			recordsWithTs = filtered
		}

		// Determine if there are more results
		hasNextPage := len(recordsWithTs) > first
		if hasNextPage {
			recordsWithTs = recordsWithTs[:first]
		}

		// Build edges
		edges := make([]interface{}, 0, len(recordsWithTs))
		var startCursor, endCursor string

		for _, r := range recordsWithTs {
			// Use effective timestamp as cursor
			cursor := encodeCursor(r.timestamp.Format("2006-01-02 15:04:05"))
			if startCursor == "" {
				startCursor = cursor
			}
			endCursor = cursor

			edges = append(edges, map[string]interface{}{
				"cursor": cursor,
				"node":   r.value,
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
			"totalCount": nil, // Could add COUNT query if needed
		}, nil
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

		return data, nil
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

// encodeCursor encodes a URI as a base64 cursor.
func encodeCursor(uri string) string {
	return base64.URLEncoding.EncodeToString([]byte(uri))
}

// decodeCursor decodes a base64 cursor to a URI.
func decodeCursor(cursor string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetRecordType returns the GraphQL type for a record.
func (b *Builder) GetRecordType(lexiconID string) *graphql.Object {
	return b.recordTypes[lexiconID]
}

// GetConnectionType returns the connection type for a record.
func (b *Builder) GetConnectionType(lexiconID string) *graphql.Object {
	return b.connectionTypes[lexiconID]
}
