package schema

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/testutil"
)

// loadLexiconsFromDir loads all lexicon JSON files from a directory tree.
func loadLexiconsFromDir(dir string) ([]*lexicon.Lexicon, error) {
	var lexicons []*lexicon.Lexicon

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lex, parseErr := lexicon.ParseBytes(data)
		if parseErr != nil {
			// Skip non-lexicon JSON files
			return nil //nolint:nilerr // intentionally skip parse errors
		}

		lexicons = append(lexicons, lex)
		return nil
	})

	return lexicons, err
}

// TestEncodeDecode verifies that encodeCursorValues and decodeCursorValues
// correctly round-trip values, handle pipe characters in values, and maintain
// backward compatibility with the legacy pipe-delimited format.
func TestEncodeDecode(t *testing.T) {
	t.Run("round-trip normal values", func(t *testing.T) {
		input := []string{"hello", "at://did:plc:abc/col/rkey"}
		cursor := encodeCursorValues(input...)
		got, err := decodeCursorValues(cursor)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Fatalf("expected %d parts, got %d", len(input), len(got))
		}
		for i, v := range input {
			if got[i] != v {
				t.Errorf("part[%d]: want %q, got %q", i, v, got[i])
			}
		}
	})

	t.Run("values containing pipe characters", func(t *testing.T) {
		input := []string{"hello|world", "at://did:plc:abc/col/rkey"}
		cursor := encodeCursorValues(input...)
		got, err := decodeCursorValues(cursor)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Fatalf("expected %d parts, got %d", len(input), len(got))
		}
		for i, v := range input {
			if got[i] != v {
				t.Errorf("part[%d]: want %q, got %q", i, v, got[i])
			}
		}
	})

	t.Run("empty strings", func(t *testing.T) {
		input := []string{"", ""}
		cursor := encodeCursorValues(input...)
		got, err := decodeCursorValues(cursor)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(input) {
			t.Fatalf("expected %d parts, got %d", len(input), len(got))
		}
		for i, v := range input {
			if got[i] != v {
				t.Errorf("part[%d]: want %q, got %q", i, v, got[i])
			}
		}
	})

	t.Run("single value", func(t *testing.T) {
		input := []string{"only-one"}
		cursor := encodeCursorValues(input...)
		got, err := decodeCursorValues(cursor)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != input[0] {
			t.Errorf("want %v, got %v", input, got)
		}
	})

	t.Run("legacy pipe-delimited format (backward compatibility)", func(t *testing.T) {
		// Simulate a cursor produced by the old pipe-delimited implementation.
		legacyCursor := base64.URLEncoding.EncodeToString([]byte("2024-01-01T00:00:00Z|at://did:plc:abc/col/rkey"))
		got, err := decodeCursorValues(legacyCursor)
		if err != nil {
			t.Fatalf("unexpected error decoding legacy cursor: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(got))
		}
		if got[0] != "2024-01-01T00:00:00Z" {
			t.Errorf("part[0]: want %q, got %q", "2024-01-01T00:00:00Z", got[0])
		}
		if got[1] != "at://did:plc:abc/col/rkey" {
			t.Errorf("part[1]: want %q, got %q", "at://did:plc:abc/col/rkey", got[1])
		}
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		_, err := decodeCursorValues("!!!invalid!!!")
		if err == nil {
			t.Error("expected error for invalid base64, got nil")
		}
	})
}

func TestBuildSchemaFromHypercertsLexicons(t *testing.T) {
	// Load all hypercerts lexicons
	lexicons, err := loadLexiconsFromDir("../../../testdata/lexicons")
	if err != nil {
		t.Fatalf("Failed to load lexicons: %v", err)
	}

	if len(lexicons) == 0 {
		t.Fatal("No lexicons loaded")
	}

	t.Logf("Loaded %d lexicons", len(lexicons))
	for _, lex := range lexicons {
		t.Logf("  - %s", lex.ID)
	}

	// Create registry and register all lexicons
	registry := lexicon.NewRegistry()
	for _, lex := range lexicons {
		registry.Register(lex)
	}

	// Build schema
	builder := NewBuilder(registry)
	schema, err := builder.Build()
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	// Verify schema has Query type
	queryType := schema.QueryType()
	if queryType == nil {
		t.Fatal("Schema has no Query type")
	}

	// Log all query fields
	t.Log("Query fields:")
	for name := range queryType.Fields() {
		t.Logf("  - %s", name)
	}

	// Verify we have the activity claim field
	activityField := queryType.Fields()["orgHypercertsClaimActivity"]
	if activityField == nil {
		t.Error("Missing orgHypercertsClaimActivity query field")
	} else {
		t.Logf("Activity field type: %s", activityField.Type.Name())
	}

	// Verify single record lookup
	activityByURI := queryType.Fields()["orgHypercertsClaimActivityByUri"]
	if activityByURI == nil {
		t.Error("Missing orgHypercertsClaimActivityByUri query field")
	}
}

func TestActivityClaimType(t *testing.T) {
	// Load activity claim lexicon specifically
	data, err := os.ReadFile("../../../testdata/lexicons/org/hypercerts/claim/activity.json")
	if err != nil {
		t.Fatalf("Failed to read activity.json: %v", err)
	}

	lex, err := lexicon.ParseBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse activity.json: %v", err)
	}

	// Load supporting lexicons
	defsData, _ := os.ReadFile("../../../testdata/lexicons/org/hypercerts/defs.json")
	defsLex, _ := lexicon.ParseBytes(defsData)

	strongRefData, _ := os.ReadFile("../../../testdata/lexicons/com/atproto/repo/strongRef.json")
	strongRefLex, _ := lexicon.ParseBytes(strongRefData)

	// Create registry
	registry := lexicon.NewRegistry()
	registry.Register(lex)
	if defsLex != nil {
		registry.Register(defsLex)
	}
	if strongRefLex != nil {
		registry.Register(strongRefLex)
	}

	// Build schema
	builder := NewBuilder(registry)
	schema, err := builder.Build()
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	// Get the activity type
	activityType := builder.GetRecordType("org.hypercerts.claim.activity")
	if activityType == nil {
		t.Fatal("Activity record type not built")
	}

	t.Logf("Activity type: %s", activityType.Name())

	// Verify fields
	fields := activityType.Fields()
	expectedFields := []string{
		"uri", "cid", // Standard record fields
		"title", "shortDescription", "createdAt", // Required fields
		"description", "image", "workScope", "startDate", "endDate",
		"contributors", "rights", "locations",
	}

	for _, fieldName := range expectedFields {
		field, ok := fields[fieldName]
		if !ok {
			t.Errorf("Missing field: %s", fieldName)
		} else {
			t.Logf("  Field %s: %s", fieldName, field.Type.String())
		}
	}

	// Test query execution
	query := `{
		orgHypercertsClaimActivity(first: 10) {
			edges {
				cursor
				node {
					uri
					title
					shortDescription
				}
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
			}
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       context.Background(),
	})

	if len(result.Errors) > 0 {
		t.Errorf("GraphQL query errors: %v", result.Errors)
	} else {
		jsonResult, _ := json.MarshalIndent(result.Data, "", "  ")
		t.Logf("Query result:\n%s", jsonResult)
	}
}

func TestUnionTypes(t *testing.T) {
	// Load lexicons
	activityData, _ := os.ReadFile("../../../testdata/lexicons/org/hypercerts/claim/activity.json")
	activityLex, _ := lexicon.ParseBytes(activityData)

	defsData, _ := os.ReadFile("../../../testdata/lexicons/org/hypercerts/defs.json")
	defsLex, _ := lexicon.ParseBytes(defsData)

	strongRefData, _ := os.ReadFile("../../../testdata/lexicons/com/atproto/repo/strongRef.json")
	strongRefLex, _ := lexicon.ParseBytes(strongRefData)

	registry := lexicon.NewRegistry()
	if activityLex != nil {
		registry.Register(activityLex)
	}
	if defsLex != nil {
		registry.Register(defsLex)
	}
	if strongRefLex != nil {
		registry.Register(strongRefLex)
	}

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	// Get activity type and check union fields
	activityType := builder.GetRecordType("org.hypercerts.claim.activity")
	if activityType == nil {
		t.Fatal("Activity type not found")
	}

	fields := activityType.Fields()

	// image is a union of org.hypercerts.defs#uri | org.hypercerts.defs#smallImage
	imageField := fields["image"]
	if imageField == nil {
		t.Error("Missing image field")
	} else {
		t.Logf("image field type: %s", imageField.Type.String())
	}

	// workScope is a union of com.atproto.repo.strongRef | #workScopeString
	workScopeField := fields["workScope"]
	if workScopeField == nil {
		t.Error("Missing workScope field")
	} else {
		t.Logf("workScope field type: %s", workScopeField.Type.String())
	}
}

func TestSchemaIntrospection(t *testing.T) {
	// Load all lexicons
	lexicons, err := loadLexiconsFromDir("../../../testdata/lexicons")
	if err != nil {
		t.Fatalf("Failed to load lexicons: %v", err)
	}

	registry := lexicon.NewRegistry()
	for _, lex := range lexicons {
		registry.Register(lex)
	}

	builder := NewBuilder(registry)
	schema, err := builder.Build()
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	// Test introspection query
	query := `{
		__schema {
			queryType {
				name
				fields {
					name
					type {
						name
						kind
					}
				}
			}
			types {
				name
				kind
			}
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
	})

	if len(result.Errors) > 0 {
		t.Errorf("Introspection errors: %v", result.Errors)
	}

	jsonResult, _ := json.MarshalIndent(result.Data, "", "  ")
	t.Logf("Introspection result:\n%s", jsonResult)
}

// buildReservedCollisionLexicon creates a Lexicon whose main record definition
// contains properties that collide with reserved metadata field names.
func buildReservedCollisionLexicon(id string, collidingProps []string) *lexicon.Lexicon {
	props := []lexicon.PropertyEntry{
		// A normal, non-colliding property that must always appear.
		{
			Name: "title",
			Property: lexicon.Property{
				Type: "string",
			},
		},
	}
	for _, name := range collidingProps {
		props = append(props, lexicon.PropertyEntry{
			Name: name,
			Property: lexicon.Property{
				// Use integer so we can detect if the metadata field (String!) was replaced.
				Type:        "integer",
				Description: "Colliding property — must be skipped",
			},
		})
	}
	return &lexicon.Lexicon{
		ID: id,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type:       "record",
				Key:        "tid",
				Properties: props,
			},
		},
	}
}

func TestBuildRecordType_ReservedFieldCollision(t *testing.T) {
	tests := []struct {
		name      string
		colliding string // reserved property name the lexicon tries to define
		wantType  string
	}{
		{name: "uri collision", colliding: "uri", wantType: "String!"},
		{name: "did collision", colliding: "did", wantType: "String!"},
		{name: "cid collision", colliding: "cid", wantType: "String!"},
		{name: "rkey collision", colliding: "rkey", wantType: "String!"},
		{name: "externalLabels collision", colliding: "externalLabels", wantType: "[ExternalLabel!]!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexiconID := "com.example.reserved." + tt.colliding

			lex := buildReservedCollisionLexicon(lexiconID, []string{tt.colliding})
			registry := lexicon.NewRegistry()
			registry.Register(lex)

			builder := NewBuilder(registry)
			_, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() failed: %v", err)
			}

			recordType := builder.GetRecordType(lexiconID)
			if recordType == nil {
				t.Fatal("record type not found after Build()")
			}

			fields := recordType.Fields()

			// The reserved metadata field must still be present with its generated type.
			metaField, ok := fields[tt.colliding]
			if !ok {
				t.Fatalf("metadata field %q is missing from the type", tt.colliding)
			}
			if metaField.Type.String() != tt.wantType {
				t.Errorf("metadata field %q type = %q, want %q (lexicon property must not overwrite it)",
					tt.colliding, metaField.Type.String(), tt.wantType)
			}

			// The normal non-colliding property must still be present.
			if _, ok := fields["title"]; !ok {
				t.Error("non-colliding property 'title' is missing from the type")
			}
		})
	}
}

func TestBuildRecordType_DoesNotExposeAuthorLabelsVirtualField(t *testing.T) {
	lexiconID := "com.example.reserved.authorLabels"
	lex := buildReservedCollisionLexicon(lexiconID, []string{"authorLabels"})
	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	recordType := builder.GetRecordType(lexiconID)
	if recordType == nil {
		t.Fatal("record type not found after Build()")
	}
	if _, ok := recordType.Fields()["authorLabels"]; ok {
		t.Fatal("record type exposed authorLabels; this release only supports where.authorLabels filtering")
	}
	if _, ok := recordType.Fields()["title"]; !ok {
		t.Fatal("non-colliding property 'title' is missing from the type")
	}
}

func TestBuildWhereInput_ReservedFieldCollision(t *testing.T) {
	tests := []struct {
		name      string
		colliding string // reserved property name the lexicon tries to define
	}{
		{name: "uri collision in WhereInput", colliding: "uri"},
		{name: "cid collision in WhereInput", colliding: "cid"},
		{name: "rkey collision in WhereInput", colliding: "rkey"},
		{name: "externalLabels collision in WhereInput", colliding: "externalLabels"},
		{name: "authorLabels collision in WhereInput", colliding: "authorLabels"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexiconID := "com.example.whereinput." + tt.colliding

			lex := buildReservedCollisionLexicon(lexiconID, []string{tt.colliding})
			registry := lexicon.NewRegistry()
			registry.Register(lex)

			builder := NewBuilder(registry)
			_, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() failed: %v", err)
			}

			whereInput, ok := builder.whereInputTypes[lexiconID]
			if !ok {
				t.Fatal("WhereInput type not found after Build()")
			}

			inputFields := whereInput.Fields()

			// The colliding lexicon property must not overwrite generated metadata filters.
			switch tt.colliding {
			case "externalLabels", "authorLabels":
				field, exists := inputFields[tt.colliding]
				if !exists {
					t.Fatalf("WhereInput missing generated %s metadata filter", tt.colliding)
				}
				if field.Type.String() != "ExternalLabelWhereInput" {
					t.Errorf("%s filter type = %q, want ExternalLabelWhereInput", tt.colliding, field.Type.String())
				}
			case "uri":
				field, exists := inputFields[tt.colliding]
				if !exists {
					t.Fatalf("WhereInput missing generated uri metadata filter")
				}
				if field.Type.String() != "URIFilterInput" {
					t.Errorf("uri filter type = %q, want URIFilterInput", field.Type.String())
				}
			default:
				if _, exists := inputFields[tt.colliding]; exists {
					t.Errorf("WhereInput has field %q which should have been skipped (reserved name collision)", tt.colliding)
				}
			}

			// The normal non-colliding property must still appear as a filter.
			if _, exists := inputFields["title"]; !exists {
				t.Error("non-colliding property 'title' is missing from WhereInput")
			}
		})
	}
}

func TestBuildWhereInput_CollectionFilterExtensions(t *testing.T) {
	lexicons, err := loadLexiconsFromDir("../../../testdata/lexicons")
	if err != nil {
		t.Fatalf("load lexicons: %v", err)
	}
	registry := lexicon.NewRegistry()
	for _, lex := range lexicons {
		registry.Register(lex)
	}

	builder := NewBuilder(registry)
	_, err = builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	activityWhereInput := builder.whereInputTypes["org.hypercerts.claim.activity"]
	if activityWhereInput == nil {
		t.Fatal("activity WhereInput type not found after Build()")
	}
	contributorDidField, ok := activityWhereInput.Fields()["contributorDid"]
	if !ok {
		t.Fatal("activity WhereInput missing contributorDid collection filter extension")
	}
	if got := contributorDidField.Type.String(); got != "DIDFilterInput" {
		t.Fatalf("contributorDid filter type = %q, want DIDFilterInput", got)
	}

	awardWhereInput := builder.whereInputTypes["app.certified.badge.award"]
	if awardWhereInput == nil {
		t.Fatal("badge award WhereInput type not found after Build()")
	}
	badgeTypeField, ok := awardWhereInput.Fields()["badgeType"]
	if !ok {
		t.Fatal("badge award WhereInput missing badgeType collection filter extension")
	}
	if got := badgeTypeField.Type.String(); got != "StringFilterInput" {
		t.Fatalf("badgeType filter type = %q, want StringFilterInput", got)
	}

	definitionWhereInput := builder.whereInputTypes["app.certified.badge.definition"]
	if definitionWhereInput == nil {
		t.Fatal("badge definition WhereInput type not found after Build()")
	}
	definitionBadgeTypeField, ok := definitionWhereInput.Fields()["badgeType"]
	if !ok {
		t.Fatal("badge definition WhereInput missing lexicon-defined badgeType filter")
	}
	if got := definitionBadgeTypeField.Type.String(); got != "StringFilterInput" {
		t.Fatalf("definition badgeType filter type = %q, want StringFilterInput", got)
	}
}

func TestBuildWhereInput_CollectionFilterExtensionCollisionFailsBuild(t *testing.T) {
	lex := &lexicon.Lexicon{
		ID: "app.certified.badge.award",
		Defs: lexicon.Defs{Main: &lexicon.RecordDef{
			Type: "record",
			Key:  "tid",
			Properties: []lexicon.PropertyEntry{
				{Name: "badgeType", Property: lexicon.Property{Type: lexicon.TypeString}},
			},
		}},
	}
	registry := lexicon.NewRegistry()
	registry.Register(lex)

	_, err := NewBuilder(registry).Build()
	if err == nil {
		t.Fatal("Build() succeeded, want collection filter extension collision error")
	}
	if !strings.Contains(err.Error(), "collection filter extension app.certified.badge.award.badgeType conflicts") {
		t.Fatalf("Build() error = %q, want badgeType collision message", err.Error())
	}
}

// TestExtractFilters_DIDFilter verifies that extractFilters correctly populates
// DIDFilter for both eq and in operators, and does not treat DID as a JSON field filter.
func TestExtractFilters_DIDFilter(t *testing.T) {
	registry := lexicon.NewRegistry()

	tests := []struct {
		name        string
		whereArg    interface{}
		wantDIDEQ   string
		wantDIDIN   []string
		wantFilters int // number of FieldFilters (non-DID)
	}{
		{
			name:     "nil whereArg returns empty",
			whereArg: nil,
		},
		{
			name:     "empty map returns empty",
			whereArg: map[string]interface{}{},
		},
		{
			name: "did eq filter",
			whereArg: map[string]interface{}{
				"did": map[string]interface{}{
					"eq": "did:plc:abc",
				},
			},
			wantDIDEQ: "did:plc:abc",
		},
		{
			name: "did in filter",
			whereArg: map[string]interface{}{
				"did": map[string]interface{}{
					"in": []interface{}{"did:plc:abc", "did:plc:def"},
				},
			},
			wantDIDIN: []string{"did:plc:abc", "did:plc:def"},
		},
		{
			name: "did eq takes precedence when both set",
			whereArg: map[string]interface{}{
				"did": map[string]interface{}{
					"eq": "did:plc:abc",
					"in": []interface{}{"did:plc:xyz"},
				},
			},
			wantDIDEQ: "did:plc:abc",
			wantDIDIN: []string{"did:plc:xyz"},
		},
		{
			name: "non-did field filter is not treated as DID",
			whereArg: map[string]interface{}{
				"title": map[string]interface{}{
					"eq": "hello",
				},
			},
			wantFilters: 1,
		},
		{
			name: "did and non-did field filters together",
			whereArg: map[string]interface{}{
				"did": map[string]interface{}{
					"eq": "did:plc:abc",
				},
				"title": map[string]interface{}{
					"eq": "hello",
				},
			},
			wantDIDEQ:   "did:plc:abc",
			wantFilters: 1,
		},
		{
			name: "empty did eq is ignored",
			whereArg: map[string]interface{}{
				"did": map[string]interface{}{
					"eq": "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, didFilter, err := extractFilters(tt.whereArg, "com.example.test", registry)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if didFilter.EQ != tt.wantDIDEQ {
				t.Errorf("DIDFilter.EQ = %q, want %q", didFilter.EQ, tt.wantDIDEQ)
			}

			if len(didFilter.IN) != len(tt.wantDIDIN) {
				t.Errorf("DIDFilter.IN = %v, want %v", didFilter.IN, tt.wantDIDIN)
			} else {
				for i, v := range tt.wantDIDIN {
					if didFilter.IN[i] != v {
						t.Errorf("DIDFilter.IN[%d] = %q, want %q", i, didFilter.IN[i], v)
					}
				}
			}

			if len(filters) != tt.wantFilters {
				t.Errorf("len(filters) = %d, want %d (filters: %v)", len(filters), tt.wantFilters, filters)
			}
		})
	}
}

// TestBuildWhereInput_UsesDIDFilterInput verifies that the WhereInput for a collection
// uses DIDFilterInput (not StringFilterInput) for the did field, and that DIDFilterInput
// only exposes eq and in operators.
func TestBuildWhereInput_UsesDIDFilterInput(t *testing.T) {
	lexiconID := "com.example.didfilter.post"
	lex := &lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "title", Property: lexicon.Property{Type: "string"}},
				},
			},
		},
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	whereInput, ok := builder.whereInputTypes[lexiconID]
	if !ok {
		t.Fatal("WhereInput type not found after Build()")
	}

	inputFields := whereInput.Fields()

	// did field must be present
	didField, ok := inputFields["did"]
	if !ok {
		t.Fatal("WhereInput is missing the 'did' field")
	}

	// The type must be DIDFilterInput (named "DIDFilterInput")
	inputObj, ok := didField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("WhereInput 'did' field type = %T, want *graphql.InputObject", didField.Type)
	}
	if inputObj.Name() != "DIDFilterInput" {
		t.Errorf("WhereInput 'did' field type name = %q, want %q", inputObj.Name(), "DIDFilterInput")
	}

	// DIDFilterInput must only have eq and in
	didFilterFields := inputObj.Fields()
	if _, ok := didFilterFields["eq"]; !ok {
		t.Error("DIDFilterInput: missing 'eq' field")
	}
	if _, ok := didFilterFields["in"]; !ok {
		t.Error("DIDFilterInput: missing 'in' field")
	}
	// Must NOT have contains, startsWith, neq, etc.
	for _, absent := range []string{"contains", "startsWith", "neq", "isNull", "gt", "lt"} {
		if _, ok := didFilterFields[absent]; ok {
			t.Errorf("DIDFilterInput: field %q should be absent", absent)
		}
	}
}

func TestBuildWhereInput_UsesURIFilterInput(t *testing.T) {
	lexiconID := "com.example.urifilter.post"
	lex := &lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "title", Property: lexicon.Property{Type: "string"}},
				},
			},
		},
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	whereInput, ok := builder.whereInputTypes[lexiconID]
	if !ok {
		t.Fatal("WhereInput type not found after Build()")
	}

	uriField, ok := whereInput.Fields()["uri"]
	if !ok {
		t.Fatal("WhereInput is missing the 'uri' field")
	}

	inputObj, ok := uriField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("WhereInput 'uri' field type = %T, want *graphql.InputObject", uriField.Type)
	}
	if inputObj.Name() != "URIFilterInput" {
		t.Errorf("WhereInput 'uri' field type name = %q, want %q", inputObj.Name(), "URIFilterInput")
	}

	uriFilterFields := inputObj.Fields()
	for _, present := range []string{"eq", "in"} {
		if _, ok := uriFilterFields[present]; !ok {
			t.Errorf("URIFilterInput: missing %q field", present)
		}
	}
	for _, absent := range []string{"contains", "startsWith", "neq", "isNull", "gt", "lt"} {
		if _, ok := uriFilterFields[absent]; ok {
			t.Errorf("URIFilterInput: field %q should be absent", absent)
		}
	}
}

func TestBuildWhereInput_DidHandledSeparately(t *testing.T) {
	// A lexicon with a "did" property must not result in a duplicate "did" filter.
	// The "did" metadata filter is always added; the lexicon property "did" must be skipped.
	lexiconID := "com.example.whereinput.did"

	lex := buildReservedCollisionLexicon(lexiconID, []string{"did"})
	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	whereInput, ok := builder.whereInputTypes[lexiconID]
	if !ok {
		t.Fatal("WhereInput type not found after Build()")
	}

	inputFields := whereInput.Fields()

	// "did" must appear exactly once (as the metadata filter).
	if _, exists := inputFields["did"]; !exists {
		t.Error("WhereInput is missing the 'did' metadata filter field")
	}

	// "title" must still appear.
	if _, exists := inputFields["title"]; !exists {
		t.Error("non-colliding property 'title' is missing from WhereInput")
	}
}

func TestExtractFilters_URIFilter(t *testing.T) {
	registry := lexicon.NewRegistry()

	tests := []struct {
		name      string
		whereArg  interface{}
		wantOps   []string
		wantValue []interface{}
	}{
		{
			name: "uri eq filter",
			whereArg: map[string]interface{}{
				"uri": map[string]interface{}{
					"eq": "at://did:plc:alice/com.example.post/1",
				},
			},
			wantOps:   []string{"eq"},
			wantValue: []interface{}{"at://did:plc:alice/com.example.post/1"},
		},
		{
			name: "uri in filter",
			whereArg: map[string]interface{}{
				"uri": map[string]interface{}{
					"in": []interface{}{
						"at://did:plc:alice/com.example.post/1",
						"at://did:plc:bob/com.example.post/2",
					},
				},
			},
			wantOps: []string{"in"},
			wantValue: []interface{}{[]interface{}{
				"at://did:plc:alice/com.example.post/1",
				"at://did:plc:bob/com.example.post/2",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, didFilter, err := extractFilters(tt.whereArg, "com.example.test", registry)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !didFilter.IsEmpty() {
				t.Fatalf("didFilter = %#v, want empty", didFilter)
			}
			if len(filters) != len(tt.wantOps) {
				t.Fatalf("len(filters) = %d, want %d (filters: %#v)", len(filters), len(tt.wantOps), filters)
			}
			for i, filter := range filters {
				if filter.Field != "uri" {
					t.Errorf("filters[%d].Field = %q, want uri", i, filter.Field)
				}
				if filter.Operator != tt.wantOps[i] {
					t.Errorf("filters[%d].Operator = %q, want %q", i, filter.Operator, tt.wantOps[i])
				}
				if !reflect.DeepEqual(filter.Value, tt.wantValue[i]) {
					t.Errorf("filters[%d].Value = %#v, want %#v", i, filter.Value, tt.wantValue[i])
				}
				if filter.Target != repositories.FieldFilterTargetColumn {
					t.Errorf("filters[%d].Target = %q, want %q", i, filter.Target, repositories.FieldFilterTargetColumn)
				}
			}
		})
	}
}

func TestExtractFilters_URIFilterKeepsMetadataStringType(t *testing.T) {
	lexiconID := "com.example.urifilter.collision"
	registry := lexicon.NewRegistry()
	registry.Register(buildReservedCollisionLexicon(lexiconID, []string{"uri"}))

	filters, didFilter, err := extractFilters(map[string]interface{}{
		"uri": map[string]interface{}{
			"eq": "at://did:plc:alice/com.example.urifilter.collision/1",
		},
	}, lexiconID, registry)
	if err != nil {
		t.Fatalf("extractFilters() error = %v", err)
	}
	if !didFilter.IsEmpty() {
		t.Fatalf("didFilter = %#v, want empty", didFilter)
	}
	if len(filters) != 1 {
		t.Fatalf("len(filters) = %d, want 1 (filters: %#v)", len(filters), filters)
	}
	if filters[0].FieldType != "string" {
		t.Fatalf("uri FieldType = %q, want string; metadata uri must not inherit colliding lexicon property type", filters[0].FieldType)
	}
	if filters[0].Target != repositories.FieldFilterTargetColumn {
		t.Fatalf("uri Target = %q, want %q", filters[0].Target, repositories.FieldFilterTargetColumn)
	}
}

func TestBuildWhereInput_UnresolvedRefsUsePresenceOnlyFilter(t *testing.T) {
	lexiconID := "com.example.unresolved.ref"
	lex := &lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{Main: &lexicon.RecordDef{
			Type: "record",
			Key:  "tid",
			Properties: []lexicon.PropertyEntry{
				{Name: "target", Property: lexicon.Property{Type: lexicon.TypeRef, Ref: "com.example.missing#main"}},
			},
		}},
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	whereInput := builder.whereInputTypes[lexiconID]
	targetField := whereInput.Fields()["target"]
	if targetField == nil {
		t.Fatal("WhereInput missing target field")
		return
	}
	inputObj, ok := targetField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("target type = %T, want *graphql.InputObject", targetField.Type)
	}
	fields := inputObj.Fields()
	if _, ok := fields["isNull"]; !ok {
		t.Fatalf("unresolved ref filter missing isNull")
	}
	if _, ok := fields["eq"]; ok {
		t.Fatalf("unresolved ref filter exposes eq but extraction cannot apply it")
	}
}

func TestBuildWhereInput_UnionConflictOmitAmbiguousNestedField(t *testing.T) {
	lexiconID := "com.example.union.conflict"
	lex := &lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "subject", Property: lexicon.Property{Type: lexicon.TypeUnion, Refs: []string{"#stringSubject", "#intSubject"}}},
				},
			},
			Others: map[string]lexicon.Def{
				"stringSubject": {Type: "object", Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{{Name: "value", Property: lexicon.Property{Type: lexicon.TypeString}}}}},
				"intSubject":    {Type: "object", Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{{Name: "value", Property: lexicon.Property{Type: lexicon.TypeInteger}}}}},
			},
		},
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	subjectField := builder.whereInputTypes[lexiconID].Fields()["subject"]
	inputObj, ok := subjectField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("subject type = %T, want *graphql.InputObject", subjectField.Type)
	}
	if _, ok := inputObj.Fields()["value"]; ok {
		t.Fatalf("ambiguous union field value should be omitted from generated filter input")
	}
}

func TestBuildWhereInput_ThreeLevelNestedFilters(t *testing.T) {
	lexiconID := "com.example.nested.depth"
	lex := &lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "one", Property: lexicon.Property{Type: lexicon.TypeRef, Ref: "#one"}},
				},
			},
			Others: map[string]lexicon.Def{
				"one": {
					Type: "object",
					Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{
						{Name: "two", Property: lexicon.Property{Type: lexicon.TypeRef, Ref: "#two"}},
					}},
				},
				"two": {
					Type: "object",
					Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{
						{Name: "three", Property: lexicon.Property{Type: lexicon.TypeString}},
						{Name: "threeObject", Property: lexicon.Property{Type: lexicon.TypeRef, Ref: "#threeObject"}},
					}},
				},
				"threeObject": {
					Type: "object",
					Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{
						{Name: "four", Property: lexicon.Property{Type: lexicon.TypeString}},
					}},
				},
			},
		},
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	whereInput := builder.whereInputTypes[lexiconID]
	oneField := whereInput.Fields()["one"]
	oneInput, ok := oneField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("one type = %T, want *graphql.InputObject", oneField.Type)
	}
	twoField := oneInput.Fields()["two"]
	if twoField == nil {
		t.Fatal("one filter missing second-level field two")
		return
	}
	twoInput, ok := twoField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("two type = %T, want *graphql.InputObject", twoField.Type)
	}
	threeField := twoInput.Fields()["three"]
	if threeField == nil {
		t.Fatal("two filter missing third-level scalar field three")
		return
	}
	if got := threeField.Type.String(); got != "ExactStringFilterInput" {
		t.Fatalf("three filter type = %q, want ExactStringFilterInput", got)
	}
	threeObjectField := twoInput.Fields()["threeObject"]
	if threeObjectField == nil {
		t.Fatal("two filter missing third-level object field threeObject")
		return
	}
	threeObjectInput, ok := threeObjectField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("threeObject type = %T, want *graphql.InputObject", threeObjectField.Type)
	}
	if _, ok := threeObjectInput.Fields()["four"]; ok {
		t.Fatal("fourth-level scalar field four should not be generated")
	}
	if _, ok := threeObjectInput.Fields()["isNull"]; !ok {
		t.Fatal("third-level object filter should still expose isNull")
	}
}

func TestExtractFilters_ThreeLevelNestedArrayFilterPath(t *testing.T) {
	const lexiconID = "org.hypercerts.collection"
	const targetURI = "at://did:plc:maker/org.hypercerts.claim.activity/activity-1"

	registry := lexicon.NewRegistry()
	registry.Register(&lexicon.Lexicon{
		ID: "com.atproto.repo.strongRef",
		Defs: lexicon.Defs{Main: &lexicon.RecordDef{
			Type: lexicon.TypeObject,
			Properties: []lexicon.PropertyEntry{
				{Name: "uri", Property: lexicon.Property{Type: lexicon.TypeString, Format: lexicon.FormatATURI}},
				{Name: "cid", Property: lexicon.Property{Type: lexicon.TypeString, Format: lexicon.FormatCID}},
			},
		}},
	})
	registry.Register(&lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "items", Property: lexicon.Property{Type: lexicon.TypeArray, Items: &lexicon.ArrayItems{Type: lexicon.TypeRef, Ref: "#item"}}},
				},
			},
			Others: map[string]lexicon.Def{
				"item": {
					Type: "object",
					Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{
						{Name: "itemIdentifier", Property: lexicon.Property{Type: lexicon.TypeRef, Ref: "com.atproto.repo.strongRef"}},
					}},
				},
			},
		},
	})

	filters, didFilter, err := extractFilters(map[string]interface{}{
		"items": map[string]interface{}{
			"any": map[string]interface{}{
				"itemIdentifier": map[string]interface{}{
					"uri": map[string]interface{}{
						"eq": targetURI,
					},
				},
			},
		},
	}, lexiconID, registry)
	if err != nil {
		t.Fatalf("extractFilters() error = %v", err)
	}
	if !didFilter.IsEmpty() {
		t.Fatalf("didFilter = %#v, want empty", didFilter)
	}
	if len(filters) != 1 {
		t.Fatalf("len(filters) = %d, want 1: %#v", len(filters), filters)
	}

	filter := filters[0]
	if filter.Field != "items" {
		t.Fatalf("filter.Field = %q, want items", filter.Field)
	}
	if !reflect.DeepEqual(filter.ArrayPath, []string{"items"}) {
		t.Fatalf("filter.ArrayPath = %#v, want [items]", filter.ArrayPath)
	}
	if !reflect.DeepEqual(filter.Path, []string{"itemIdentifier", "uri"}) {
		t.Fatalf("filter.Path = %#v, want [itemIdentifier uri]", filter.Path)
	}
	if filter.Operator != "eq" {
		t.Fatalf("filter.Operator = %q, want eq", filter.Operator)
	}
	if filter.Value != targetURI {
		t.Fatalf("filter.Value = %#v, want %q", filter.Value, targetURI)
	}
	if filter.FieldType != lexicon.TypeString {
		t.Fatalf("filter.FieldType = %q, want string", filter.FieldType)
	}
	if filter.Target != "" {
		t.Fatalf("filter.Target = %q, want zero-value JSON target", filter.Target)
	}
}

func TestBuildWhereInput_HidesNestedArrayAnyInsideArrayAny(t *testing.T) {
	lexiconID := "com.example.nested.arrays"
	lex := &lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "facets", Property: lexicon.Property{Type: lexicon.TypeArray, Items: &lexicon.ArrayItems{Type: lexicon.TypeRef, Ref: "#facet"}}},
					{Name: "topFeatures", Property: lexicon.Property{Type: lexicon.TypeArray, Items: &lexicon.ArrayItems{Type: lexicon.TypeRef, Ref: "#feature"}}},
				},
			},
			Others: map[string]lexicon.Def{
				"facet": {
					Type: "object",
					Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{
						{Name: "features", Property: lexicon.Property{Type: lexicon.TypeArray, Items: &lexicon.ArrayItems{Type: lexicon.TypeRef, Ref: "#feature"}}},
					}},
				},
				"feature": {
					Type: "object",
					Object: &lexicon.ObjectDef{Type: "object", Properties: []lexicon.PropertyEntry{
						{Name: "tag", Property: lexicon.Property{Type: lexicon.TypeString}},
					}},
				},
			},
		},
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	whereInput := builder.whereInputTypes[lexiconID]
	whereFields := whereInput.Fields()

	facetsInput, ok := whereFields["facets"].Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("facets filter type = %T, want *graphql.InputObject", whereFields["facets"].Type)
	}
	facetsAny := facetsInput.Fields()["any"]
	if facetsAny == nil {
		t.Fatal("top-level facets array should expose any")
		return
	}
	facetInput, ok := facetsAny.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("facets.any type = %T, want *graphql.InputObject", facetsAny.Type)
	}

	featuresField := facetInput.Fields()["features"]
	if featuresField == nil {
		t.Fatal("facets.any should expose nested features presence filter")
		return
	}
	featuresInput, ok := featuresField.Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("facets.any.features type = %T, want *graphql.InputObject", featuresField.Type)
	}
	if _, ok := featuresInput.Fields()["isNull"]; !ok {
		t.Fatal("facets.any.features should keep isNull presence filtering")
	}
	if _, ok := featuresInput.Fields()["any"]; ok {
		t.Fatal("facets.any.features should not expose nested any because nested array any filters cannot execute")
	}

	topFeaturesInput, ok := whereFields["topFeatures"].Type.(*graphql.InputObject)
	if !ok {
		t.Fatalf("topFeatures filter type = %T, want *graphql.InputObject", whereFields["topFeatures"].Type)
	}
	if _, ok := topFeaturesInput.Fields()["any"]; !ok {
		t.Fatal("top-level topFeatures array should still expose any")
	}
}

func TestBuildWhereInput_ComplexPropertiesUseNestedOrPresenceFilters(t *testing.T) {
	lexiconID := "com.example.whereinput.presence"
	lex := &lexicon.Lexicon{
		ID: lexiconID,
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "title", Property: lexicon.Property{Type: lexicon.TypeString}},
					{Name: "createdAt", Property: lexicon.Property{Type: lexicon.TypeString, Format: lexicon.FormatDatetime}},
					{Name: "contributors", Property: lexicon.Property{Type: lexicon.TypeArray, Items: &lexicon.ArrayItems{Type: lexicon.TypeString}}},
					{Name: "image", Property: lexicon.Property{Type: lexicon.TypeBlob}},
					{Name: "root", Property: lexicon.Property{Type: lexicon.TypeCIDLink}},
					{Name: "raw", Property: lexicon.Property{Type: lexicon.TypeUnknown}},
					{Name: "nestedRecord", Property: lexicon.Property{Type: lexicon.TypeRecord}},
				},
			},
		},
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	builder := NewBuilder(registry)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	whereInput, ok := builder.whereInputTypes[lexiconID]
	if !ok {
		t.Fatal("WhereInput type not found after Build()")
	}

	inputFields := whereInput.Fields()
	wantTypes := map[string]string{
		"uri":            "URIFilterInput",
		"did":            "DIDFilterInput",
		"externalLabels": "ExternalLabelWhereInput",
		"authorLabels":   "ExternalLabelWhereInput",
		"title":          "StringFilterInput",
		"createdAt":      "DateTimeFilterInput",
		"contributors":   "ComExampleWhereinputPresenceContributorsArrayFilterInput",
		"image":          "PresenceFilterInput",
		"root":           "PresenceFilterInput",
		"raw":            "PresenceFilterInput",
	}
	for fieldName, wantType := range wantTypes {
		field, exists := inputFields[fieldName]
		if !exists {
			t.Errorf("WhereInput missing field %q", fieldName)
			continue
		}
		if gotType := field.Type.String(); gotType != wantType {
			t.Errorf("WhereInput field %q type = %q, want %q", fieldName, gotType, wantType)
		}
	}

	if _, exists := inputFields["nestedRecord"]; exists {
		t.Error("record-typed property should not be filterable")
	}
	if imageField, exists := inputFields["image"]; exists {
		if got := imageField.Description(); !strings.Contains(got, "nested values are not filterable") {
			t.Errorf("presence field description = %q, want nested-filtering limitation", got)
		}
	}
}

// TestSortFieldValueForRecord verifies that sortFieldValueForRecord extracts the
// correct sort field value from a record for cursor building.
func TestSortFieldValueForRecord(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	rec := &repositories.Record{
		URI:        "at://did:plc:abc/com.example.post/rkey123",
		CID:        "bafyreiabcdef",
		DID:        "did:plc:abc",
		Collection: "com.example.post",
		RKey:       "rkey123",
		IndexedAt:  now,
	}

	tests := []struct {
		name    string
		sortOpt *repositories.SortOption
		value   map[string]interface{}
		want    string
	}{
		{
			name:    "nil sortOpt returns indexed_at",
			sortOpt: nil,
			value:   map[string]interface{}{},
			want:    "2024-06-15T12:00:00Z",
		},
		{
			name:    "indexed_at field returns formatted time",
			sortOpt: &repositories.SortOption{Field: "indexed_at", Direction: "DESC"},
			value:   map[string]interface{}{},
			want:    "2024-06-15T12:00:00Z",
		},
		{
			name:    "uri field returns record URI",
			sortOpt: &repositories.SortOption{Field: "uri", Direction: "DESC"},
			value:   map[string]interface{}{},
			want:    "at://did:plc:abc/com.example.post/rkey123",
		},
		{
			name:    "did field returns record DID",
			sortOpt: &repositories.SortOption{Field: "did", Direction: "ASC"},
			value:   map[string]interface{}{},
			want:    "did:plc:abc",
		},
		{
			name:    "cid field returns record CID",
			sortOpt: &repositories.SortOption{Field: "cid", Direction: "DESC"},
			value:   map[string]interface{}{},
			want:    "bafyreiabcdef",
		},
		{
			name:    "rkey field returns record RKey",
			sortOpt: &repositories.SortOption{Field: "rkey", Direction: "DESC"},
			value:   map[string]interface{}{},
			want:    "rkey123",
		},
		{
			name:    "collection field returns record Collection",
			sortOpt: &repositories.SortOption{Field: "collection", Direction: "DESC"},
			value:   map[string]interface{}{},
			want:    "com.example.post",
		},
		{
			name:    "JSON field present returns its value",
			sortOpt: &repositories.SortOption{Field: "title", Direction: "DESC"},
			value:   map[string]interface{}{"title": "Hello World"},
			want:    "Hello World",
		},
		{
			name:    "JSON field missing returns empty string",
			sortOpt: &repositories.SortOption{Field: "title", Direction: "DESC"},
			value:   map[string]interface{}{},
			want:    "",
		},
		{
			name:    "JSON field with nil value returns empty string",
			sortOpt: &repositories.SortOption{Field: "title", Direction: "DESC"},
			value:   map[string]interface{}{"title": nil},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortFieldValueForRecord(rec, tt.value, tt.sortOpt)
			if got != tt.want {
				t.Errorf("sortFieldValueForRecord() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEmptyConnection verifies that emptyConnection returns a well-formed
// Relay connection with empty edges, all-false pageInfo, and totalCount of 0.
func TestEmptyConnection(t *testing.T) {
	result := emptyConnection()

	// Verify edges is an empty (non-nil) slice
	edges, ok := result["edges"]
	if !ok {
		t.Fatal("emptyConnection: missing 'edges' key")
	}
	edgeSlice, ok := edges.([]interface{})
	if !ok {
		t.Fatalf("emptyConnection: edges is %T, want []interface{}", edges)
	}
	if len(edgeSlice) != 0 {
		t.Errorf("emptyConnection: edges length = %d, want 0", len(edgeSlice))
	}

	// Verify pageInfo structure
	pageInfoRaw, ok := result["pageInfo"]
	if !ok {
		t.Fatal("emptyConnection: missing 'pageInfo' key")
	}
	pageInfo, ok := pageInfoRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("emptyConnection: pageInfo is %T, want map[string]interface{}", pageInfoRaw)
	}

	if v, ok := pageInfo["hasNextPage"].(bool); !ok || v {
		t.Errorf("emptyConnection: hasNextPage = %v, want false", pageInfo["hasNextPage"])
	}
	if v, ok := pageInfo["hasPreviousPage"].(bool); !ok || v {
		t.Errorf("emptyConnection: hasPreviousPage = %v, want false", pageInfo["hasPreviousPage"])
	}
	if pageInfo["startCursor"] != nil {
		t.Errorf("emptyConnection: startCursor = %v, want nil", pageInfo["startCursor"])
	}
	if pageInfo["endCursor"] != nil {
		t.Errorf("emptyConnection: endCursor = %v, want nil", pageInfo["endCursor"])
	}

	// Verify totalCount is 0
	totalCount, ok := result["totalCount"]
	if !ok {
		t.Fatal("emptyConnection: missing 'totalCount' key")
	}
	if totalCount != 0 {
		t.Errorf("emptyConnection: totalCount = %v, want 0", totalCount)
	}
}

// setupCoercionTestDB creates an in-memory SQLite database with migrations applied,
// inserts a single org.hypercerts.claim.activity record with the given JSON payload,
// and returns a context that carries the repositories.
func setupCoercionTestDB(t *testing.T, recordJSON string) context.Context {
	t.Helper()

	return setupSchemaRecordTestDB(t, &repositories.Record{
		URI:        "at://did:plc:test/org.hypercerts.claim.activity/rkey1",
		CID:        "bafyreiabc123",
		DID:        "did:plc:test",
		Collection: "org.hypercerts.claim.activity",
		JSON:       recordJSON,
		RKey:       "rkey1",
	})
}

// setupSchemaRecordTestDB creates an in-memory SQLite database with migrations applied,
// inserts a single record, and returns a context that carries the repositories.
func setupSchemaRecordTestDB(t *testing.T, rec *repositories.Record) context.Context {
	t.Helper()
	return setupSchemaRecordsTestDB(t, []*repositories.Record{rec})
}

func setupSchemaRecordsTestDB(t *testing.T, recordsToInsert []*repositories.Record) context.Context {
	t.Helper()

	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	if err := db.Records.BatchInsert(ctx, recordsToInsert); err != nil {
		t.Fatalf("setupSchemaRecordsTestDB: failed to insert records: %v", err)
	}

	repos := &resolver.Repositories{
		Records:        db.Records,
		ExternalLabels: db.ExternalLabels,
	}
	return resolver.WithRepositories(ctx, repos)
}

func buildCIDLinkRegressionSchema(t *testing.T) *graphql.Schema {
	t.Helper()

	registry := lexicon.NewRegistry()
	registry.Register(&lexicon.Lexicon{
		ID: "com.example.cidlink.record",
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "image", Property: lexicon.Property{Type: lexicon.TypeBlob}},
					{Name: "root", Property: lexicon.Property{Type: lexicon.TypeCIDLink}},
				},
			},
		},
	})

	schema, err := NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("buildCIDLinkRegressionSchema: failed to build schema: %v", err)
	}
	return schema
}

func TestCIDLinkSerializationInBuiltSchema(t *testing.T) {
	const cid = "bafkreidlp6sdj6jkroakvmbntpy2clsj77foijheae5byt3iwz7d2k542a"
	const recordURI = "at://did:plc:test/com.example.cidlink.record/rkey1"

	ctx := setupSchemaRecordTestDB(t, &repositories.Record{
		URI:        recordURI,
		CID:        "bafyreirecordcid",
		DID:        "did:plc:test",
		Collection: "com.example.cidlink.record",
		JSON:       `{"image":{"ref":{"$link":"` + cid + `"},"mimeType":"image/png"},"root":{"$link":"` + cid + `"}}`,
		RKey:       "rkey1",
	})
	schema := buildCIDLinkRegressionSchema(t)

	query := `{
		comExampleCidlinkRecord(first: 10) {
			edges {
				node {
					image { ref mimeType }
					root
				}
			}
		}
		comExampleCidlinkRecordByUri(uri: "at://did:plc:test/com.example.cidlink.record/rkey1") {
			image { ref }
			root
		}
		records(collection: "com.example.cidlink.record", first: 10) {
			edges {
				node { value }
			}
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("TestCIDLinkSerializationInBuiltSchema: unexpected GraphQL errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}

	collectionNode := firstConnectionNode(t, data["comExampleCidlinkRecord"], "comExampleCidlinkRecord")
	assertCIDLinkFields(t, collectionNode, cid, "collection query")

	byURIRecord, ok := data["comExampleCidlinkRecordByUri"].(map[string]interface{})
	if !ok {
		t.Fatalf("comExampleCidlinkRecordByUri is %T, want map[string]interface{}", data["comExampleCidlinkRecordByUri"])
	}
	assertCIDLinkFields(t, byURIRecord, cid, "ByUri query")

	rawNode := firstConnectionNode(t, data["records"], "records")
	value, ok := rawNode["value"].(map[string]interface{})
	if !ok {
		t.Fatalf("records node value is %T, want map[string]interface{}", rawNode["value"])
	}
	assertRawCIDLinkShape(t, value, cid)
}

func executeConnectionQuery(ctx context.Context, t *testing.T, schema *graphql.Schema, query, fieldName string) map[string]interface{} {
	t.Helper()

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", result.Errors)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}
	conn, ok := data[fieldName].(map[string]interface{})
	if !ok {
		t.Fatalf("%s is %T, want map[string]interface{}", fieldName, data[fieldName])
	}
	return conn
}

func assertConnectionURIs(t *testing.T, conn map[string]interface{}, wantURIs []string) {
	t.Helper()

	edges, ok := conn["edges"].([]interface{})
	if !ok {
		t.Fatalf("edges is %T, want []interface{}", conn["edges"])
	}
	if len(edges) != len(wantURIs) {
		t.Fatalf("len(edges) = %d, want %d: %v", len(edges), len(wantURIs), edges)
	}

	got := map[string]bool{}
	for _, edgeValue := range edges {
		edge, ok := edgeValue.(map[string]interface{})
		if !ok {
			t.Fatalf("edge is %T, want map[string]interface{}", edgeValue)
		}
		node, ok := edge["node"].(map[string]interface{})
		if !ok {
			t.Fatalf("node is %T, want map[string]interface{}", edge["node"])
		}
		uri, ok := node["uri"].(string)
		if !ok {
			t.Fatalf("node.uri is %T, want string", node["uri"])
		}
		got[uri] = true
	}

	for _, wantURI := range wantURIs {
		if !got[wantURI] {
			t.Fatalf("missing URI %q in result set %v", wantURI, got)
		}
	}
}

func firstConnectionNode(t *testing.T, connectionValue interface{}, fieldName string) map[string]interface{} {
	t.Helper()

	conn, ok := connectionValue.(map[string]interface{})
	if !ok {
		t.Fatalf("%s is %T, want map[string]interface{}", fieldName, connectionValue)
	}
	edges, ok := conn["edges"].([]interface{})
	if !ok || len(edges) == 0 {
		t.Fatalf("%s edges = %v, want at least one edge", fieldName, conn["edges"])
	}
	edge, ok := edges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("%s edge[0] is %T, want map[string]interface{}", fieldName, edges[0])
	}
	node, ok := edge["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s node is %T, want map[string]interface{}", fieldName, edge["node"])
	}
	return node
}

func assertCIDLinkFields(t *testing.T, record map[string]interface{}, cid, path string) {
	t.Helper()

	image, ok := record["image"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s image is %T, want map[string]interface{}", path, record["image"])
	}
	if got := image["ref"]; got != cid {
		t.Fatalf("%s image.ref = %v, want %q", path, got, cid)
	}
	if got := record["root"]; got != cid {
		t.Fatalf("%s root = %v, want %q", path, got, cid)
	}
}

func assertRawCIDLinkShape(t *testing.T, value map[string]interface{}, cid string) {
	t.Helper()

	root, ok := value["root"].(map[string]interface{})
	if !ok {
		t.Fatalf("raw root is %T, want map[string]interface{}", value["root"])
	}
	if got := root["$link"]; got != cid {
		t.Fatalf("raw root $link = %v, want %q", got, cid)
	}

	image, ok := value["image"].(map[string]interface{})
	if !ok {
		t.Fatalf("raw image is %T, want map[string]interface{}", value["image"])
	}
	ref, ok := image["ref"].(map[string]interface{})
	if !ok {
		t.Fatalf("raw image.ref is %T, want map[string]interface{}", image["ref"])
	}
	if got := ref["$link"]; got != cid {
		t.Fatalf("raw image.ref $link = %v, want %q", got, cid)
	}
}

// buildActivitySchema builds a GraphQL schema from the org.hypercerts.claim.activity lexicon.
func buildAllTestdataSchema(t *testing.T) *graphql.Schema {
	t.Helper()

	lexicons, err := loadLexiconsFromDir("../../../testdata/lexicons")
	if err != nil {
		t.Fatalf("buildAllTestdataSchema: failed to load lexicons: %v", err)
	}
	registry := lexicon.NewRegistry()
	for _, lex := range lexicons {
		registry.Register(lex)
	}

	schema, err := NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("buildAllTestdataSchema: failed to build schema: %v", err)
	}
	return schema
}

func buildActivitySchema(t *testing.T) *graphql.Schema {
	t.Helper()

	data, err := os.ReadFile("../../../testdata/lexicons/org/hypercerts/claim/activity.json")
	if err != nil {
		t.Fatalf("buildActivitySchema: failed to read activity.json: %v", err)
	}
	lex, err := lexicon.ParseBytes(data)
	if err != nil {
		t.Fatalf("buildActivitySchema: failed to parse activity.json: %v", err)
	}

	registry := lexicon.NewRegistry()
	registry.Register(lex)

	schema, err := NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("buildActivitySchema: failed to build schema: %v", err)
	}
	return schema
}

func TestCollectionResolver_URIWhereFilterUsesRecordMetadata(t *testing.T) {
	ctx := setupCoercionTestDB(t, `{"title":"My Title","shortDescription":"My Desc","createdAt":"2025-01-01T00:00:00Z","uri":"json-shadow"}`)
	schema := buildActivitySchema(t)

	query := `{
		orgHypercertsClaimActivity(
			first: 10
			where: { uri: { eq: "at://did:plc:test/org.hypercerts.claim.activity/rkey1" } }
		) {
			edges {
				node { uri title }
			}
			totalCount
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}
	conn, ok := data["orgHypercertsClaimActivity"].(map[string]interface{})
	if !ok {
		t.Fatalf("orgHypercertsClaimActivity is %T", data["orgHypercertsClaimActivity"])
	}
	edges, ok := conn["edges"].([]interface{})
	if !ok {
		t.Fatalf("edges is %T, want []interface{}", conn["edges"])
	}
	if len(edges) != 1 {
		t.Fatalf("len(edges) = %d, want 1", len(edges))
	}
	edge := edges[0].(map[string]interface{})
	node := edge["node"].(map[string]interface{})
	if got := node["uri"]; got != "at://did:plc:test/org.hypercerts.claim.activity/rkey1" {
		t.Errorf("node.uri = %v, want record metadata URI", got)
	}
	if got := conn["totalCount"]; got != 1 {
		t.Errorf("totalCount = %v, want 1", got)
	}
}

func TestCollectionResolver_URIWhereFilterRejectsSubstringOperators(t *testing.T) {
	ctx := setupCoercionTestDB(t, `{"title":"My Title","shortDescription":"My Desc","createdAt":"2025-01-01T00:00:00Z"}`)
	schema := buildActivitySchema(t)

	query := `{
		orgHypercertsClaimActivity(
			first: 10
			where: { uri: { contains: "org.hypercerts.claim.activity" } }
		) {
			edges { node { uri } }
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})

	if len(result.Errors) == 0 {
		t.Fatal("expected GraphQL validation error for unsupported uri.contains filter")
	}
	if !strings.Contains(result.Errors[0].Message, "Unknown field") {
		t.Fatalf("error = %q, want unknown field validation", result.Errors[0].Message)
	}
}

func TestCollectionResolver_NestedUnionFilterFindsBadgeAwardRecipientDID(t *testing.T) {
	schema := buildAllTestdataSchema(t)
	ctx := setupSchemaRecordsTestDB(t, []*repositories.Record{
		{
			URI:        "at://did:plc:issuer/app.certified.badge.award/award-alice",
			CID:        "bafyawardalice",
			DID:        "did:plc:issuer",
			Collection: "app.certified.badge.award",
			JSON:       `{"badge":{"uri":"at://did:plc:issuer/app.certified.badge.definition/1","cid":"bafybadge"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:alice"},"createdAt":"2026-01-01T00:00:00Z"}`,
		},
		{
			URI:        "at://did:plc:issuer/app.certified.badge.award/award-bob",
			CID:        "bafyawardbob",
			DID:        "did:plc:issuer",
			Collection: "app.certified.badge.award",
			JSON:       `{"badge":{"uri":"at://did:plc:issuer/app.certified.badge.definition/1","cid":"bafybadge"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:bob"},"createdAt":"2026-01-01T00:00:00Z"}`,
		},
	})

	query := `{
		appCertifiedBadgeAward(
			first: 10
			where: { subject: { did: { eq: "did:plc:alice" } } }
		) {
			totalCount
			edges { node { uri } }
		}
	}`

	conn := executeConnectionQuery(ctx, t, schema, query, "appCertifiedBadgeAward")
	assertConnectionURIs(t, conn, []string{"at://did:plc:issuer/app.certified.badge.award/award-alice"})
	if got := conn["totalCount"]; got != 1 {
		t.Fatalf("totalCount = %v, want 1", got)
	}
}

func TestCollectionResolver_BadgeAwardBadgeTypeFilterFindsReferencedDefinitionType(t *testing.T) {
	schema := buildAllTestdataSchema(t)
	const endorsementBadgeURI = "at://did:plc:issuer/app.certified.badge.definition/endorsement"
	const otherBadgeURI = "at://did:plc:issuer/app.certified.badge.definition/other"
	ctx := setupSchemaRecordsTestDB(t, []*repositories.Record{
		{
			URI:        endorsementBadgeURI,
			CID:        "bafybadgeendorsement",
			DID:        "did:plc:issuer",
			Collection: "app.certified.badge.definition",
			JSON:       `{"title":"Endorsement","badgeType":"endorsement","createdAt":"2026-01-01T00:00:00Z"}`,
		},
		{
			URI:        otherBadgeURI,
			CID:        "bafybadgeother",
			DID:        "did:plc:issuer",
			Collection: "app.certified.badge.definition",
			JSON:       `{"title":"Other","badgeType":"credential","createdAt":"2026-01-01T00:00:00Z"}`,
		},
		{
			URI:        "at://did:plc:issuer/app.certified.badge.award/award-endorsement",
			CID:        "bafyawardendorsement",
			DID:        "did:plc:issuer",
			Collection: "app.certified.badge.award",
			JSON:       `{"badge":{"uri":"` + endorsementBadgeURI + `","cid":"bafybadgeendorsement"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:alice"},"createdAt":"2026-01-01T00:00:00Z"}`,
		},
		{
			URI:        "at://did:plc:issuer/app.certified.badge.award/award-other",
			CID:        "bafyawardother",
			DID:        "did:plc:issuer",
			Collection: "app.certified.badge.award",
			JSON:       `{"badge":{"uri":"` + otherBadgeURI + `","cid":"bafybadgeother"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:bob"},"createdAt":"2026-01-01T00:00:00Z"}`,
		},
		{
			URI:        "at://did:plc:issuer/app.certified.badge.award/award-missing-definition",
			CID:        "bafyawardmissing",
			DID:        "did:plc:issuer",
			Collection: "app.certified.badge.award",
			JSON:       `{"badge":{"uri":"at://did:plc:issuer/app.certified.badge.definition/missing","cid":"bafymissing"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:carol"},"createdAt":"2026-01-01T00:00:00Z"}`,
		},
	})

	query := `{
		appCertifiedBadgeAward(
			first: 10
			where: { badgeType: { eq: "endorsement" } }
		) {
			totalCount
			edges { node { uri } }
		}
	}`

	conn := executeConnectionQuery(ctx, t, schema, query, "appCertifiedBadgeAward")
	assertConnectionURIs(t, conn, []string{"at://did:plc:issuer/app.certified.badge.award/award-endorsement"})
	if got := conn["totalCount"]; got != 1 {
		t.Fatalf("totalCount = %v, want 1", got)
	}
}

func TestCollectionResolver_NestedArrayFilterFindsCollectionContainingItemURI(t *testing.T) {
	schema := buildAllTestdataSchema(t)
	const activityURI = "at://did:plc:maker/org.hypercerts.claim.activity/activity-1"
	ctx := setupSchemaRecordsTestDB(t, []*repositories.Record{
		{
			URI:        "at://did:plc:alice/org.hypercerts.collection/contains-activity",
			CID:        "bafycollection1",
			DID:        "did:plc:alice",
			Collection: "org.hypercerts.collection",
			JSON:       `{"title":"Project","createdAt":"2026-01-01T00:00:00Z","items":[{"itemIdentifier":{"uri":"` + activityURI + `","cid":"bafyactivity"},"itemWeight":"1"}]}`,
		},
		{
			URI:        "at://did:plc:alice/org.hypercerts.collection/other",
			CID:        "bafycollection2",
			DID:        "did:plc:alice",
			Collection: "org.hypercerts.collection",
			JSON:       `{"title":"Other","createdAt":"2026-01-01T00:00:00Z","items":[{"itemIdentifier":{"uri":"at://did:plc:maker/org.hypercerts.claim.activity/other","cid":"bafyother"}}]}`,
		},
	})

	query := `{
		orgHypercertsCollection(
			first: 10
			where: { items: { any: { itemIdentifier: { uri: { eq: "` + activityURI + `" } } } } }
		) {
			totalCount
			edges { node { uri } }
		}
	}`

	conn := executeConnectionQuery(ctx, t, schema, query, "orgHypercertsCollection")
	assertConnectionURIs(t, conn, []string{"at://did:plc:alice/org.hypercerts.collection/contains-activity"})
	if got := conn["totalCount"]; got != 1 {
		t.Fatalf("totalCount = %v, want 1", got)
	}
}

func TestCollectionResolver_NestedArrayAnyFilterKeepsPredicatesOnSameElement(t *testing.T) {
	schema := buildAllTestdataSchema(t)
	const targetURI = "at://did:plc:maker/org.hypercerts.claim.activity/activity-1"
	const targetCID = "bafyactivity"
	ctx := setupSchemaRecordsTestDB(t, []*repositories.Record{
		{
			URI:        "at://did:plc:alice/org.hypercerts.collection/exact-activity",
			CID:        "bafycollection1",
			DID:        "did:plc:alice",
			Collection: "org.hypercerts.collection",
			JSON:       `{"title":"Exact","createdAt":"2026-01-01T00:00:00Z","items":[{"itemIdentifier":{"uri":"` + targetURI + `","cid":"` + targetCID + `"},"itemWeight":"1"}]}`,
		},
		{
			URI:        "at://did:plc:alice/org.hypercerts.collection/split-activity",
			CID:        "bafycollection2",
			DID:        "did:plc:alice",
			Collection: "org.hypercerts.collection",
			JSON:       `{"title":"Split","createdAt":"2026-01-01T00:00:00Z","items":[{"itemIdentifier":{"uri":"` + targetURI + `","cid":"wrong-cid"}},{"itemIdentifier":{"uri":"at://did:plc:maker/org.hypercerts.claim.activity/other","cid":"` + targetCID + `"}}]}`,
		},
	})

	query := `{
		orgHypercertsCollection(
			first: 10
			where: { items: { any: { itemIdentifier: { uri: { eq: "` + targetURI + `" }, cid: { eq: "` + targetCID + `" } } } } }
		) {
			totalCount
			edges { node { uri } }
		}
	}`

	conn := executeConnectionQuery(ctx, t, schema, query, "orgHypercertsCollection")
	assertConnectionURIs(t, conn, []string{"at://did:plc:alice/org.hypercerts.collection/exact-activity"})
	if got := conn["totalCount"]; got != 1 {
		t.Fatalf("totalCount = %v, want 1", got)
	}
}

func TestCollectionResolver_ContributorDidCompatibilityFilter(t *testing.T) {
	schema := buildAllTestdataSchema(t)
	const contributorURI = "at://did:plc:contributor/org.hypercerts.claim.contributorInformation/info-1"
	ctx := setupSchemaRecordsTestDB(t, []*repositories.Record{
		{
			URI:        contributorURI,
			CID:        "bafycontributorinfo",
			DID:        "did:plc:contributor",
			Collection: "org.hypercerts.claim.contributorInformation",
			JSON:       `{"identifier":"did:plc:alice","displayName":"Alice","createdAt":"2026-01-01T00:00:00Z"}`,
		},
		{
			URI:        "at://did:plc:author/org.hypercerts.claim.activity/inline",
			CID:        "bafyactivityinline",
			DID:        "did:plc:author",
			Collection: "org.hypercerts.claim.activity",
			JSON:       `{"title":"Inline","shortDescription":"Inline contributor","createdAt":"2026-01-01T00:00:00Z","contributors":[{"contributorIdentity":{"identity":"did:plc:alice"}}]}`,
		},
		{
			URI:        "at://did:plc:author/org.hypercerts.claim.activity/strongref",
			CID:        "bafyactivityref",
			DID:        "did:plc:author",
			Collection: "org.hypercerts.claim.activity",
			JSON:       `{"title":"StrongRef","shortDescription":"Strong ref contributor","createdAt":"2026-01-01T00:00:00Z","contributors":[{"contributorIdentity":{"uri":"` + contributorURI + `","cid":"bafycontributorinfo"}}]}`,
		},
		{
			URI:        "at://did:plc:author/org.hypercerts.claim.activity/direct",
			CID:        "bafyactivitydirect",
			DID:        "did:plc:author",
			Collection: "org.hypercerts.claim.activity",
			JSON:       `{"title":"Direct","shortDescription":"Direct contributor identity","createdAt":"2026-01-01T00:00:00Z","contributors":[{"identity":"did:plc:alice"}]}`,
		},
		{
			URI:        "at://did:plc:author/org.hypercerts.claim.activity/bare",
			CID:        "bafyactivitybare",
			DID:        "did:plc:author",
			Collection: "org.hypercerts.claim.activity",
			JSON:       `{"title":"Bare","shortDescription":"Bare contributor","createdAt":"2026-01-01T00:00:00Z","contributors":["did:plc:alice"]}`,
		},
		{
			URI:        "at://did:plc:author/org.hypercerts.claim.activity/bob",
			CID:        "bafyactivitybob",
			DID:        "did:plc:author",
			Collection: "org.hypercerts.claim.activity",
			JSON:       `{"title":"Bob","shortDescription":"Other contributor","createdAt":"2026-01-01T00:00:00Z","contributors":[{"contributorIdentity":{"identity":"did:plc:bob"}}]}`,
		},
		{
			URI:        "at://did:plc:author/org.hypercerts.claim.activity/bare-bob",
			CID:        "bafyactivitybarebob",
			DID:        "did:plc:author",
			Collection: "org.hypercerts.claim.activity",
			JSON:       `{"title":"Bare Bob","shortDescription":"Other bare contributor","createdAt":"2026-01-01T00:00:00Z","contributors":["did:plc:bob"]}`,
		},
	})

	query := `{
		orgHypercertsClaimActivity(
			first: 10
			where: { contributorDid: { eq: "did:plc:alice" } }
		) {
			totalCount
			edges { node { uri } }
		}
	}`

	conn := executeConnectionQuery(ctx, t, schema, query, "orgHypercertsClaimActivity")
	assertConnectionURIs(t, conn, []string{
		"at://did:plc:author/org.hypercerts.claim.activity/inline",
		"at://did:plc:author/org.hypercerts.claim.activity/strongref",
		"at://did:plc:author/org.hypercerts.claim.activity/direct",
		"at://did:plc:author/org.hypercerts.claim.activity/bare",
	})
	if got := conn["totalCount"]; got != 4 {
		t.Fatalf("totalCount = %v, want 4", got)
	}
}

func TestCollectionResolver_NestedFiltersRejectSubstringOperators(t *testing.T) {
	schema := buildAllTestdataSchema(t)
	ctx := setupSchemaRecordsTestDB(t, []*repositories.Record{
		{
			URI:        "at://did:plc:author/org.hypercerts.claim.activity/inline",
			CID:        "bafyactivityinline",
			DID:        "did:plc:author",
			Collection: "org.hypercerts.claim.activity",
			JSON:       `{"title":"Inline","shortDescription":"Inline contributor","createdAt":"2026-01-01T00:00:00Z","contributors":[{"contributorIdentity":{"identity":"did:plc:alice"}}]}`,
		},
	})

	query := `{
		orgHypercertsClaimActivity(
			first: 10
			where: { contributors: { any: { contributorIdentity: { identity: { contains: "did:plc" } } } } }
		) {
			edges { node { uri } }
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})
	if len(result.Errors) == 0 {
		t.Fatal("expected GraphQL validation error for unsupported nested contains filter")
	}
	if !strings.Contains(result.Errors[0].Message, "Unknown field") {
		t.Fatalf("error = %q, want unknown field validation", result.Errors[0].Message)
	}
}

// TestCoerceRequiredFields_MissingFields verifies that required string fields that are
// absent from the stored JSON are coerced to their zero value ("") when resolved.
func TestCoerceRequiredFields_MissingFields(t *testing.T) {
	// Record is missing "title" and "shortDescription" — only "createdAt" is present.
	ctx := setupCoercionTestDB(t, `{"createdAt":"2025-01-01T00:00:00Z"}`)
	schema := buildActivitySchema(t)

	query := `{
		orgHypercertsClaimActivity(first: 10) {
			edges {
				node {
					title
					shortDescription
				}
			}
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("TestCoerceRequiredFields_MissingFields: unexpected GraphQL errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}

	conn, ok := data["orgHypercertsClaimActivity"].(map[string]interface{})
	if !ok {
		t.Fatalf("orgHypercertsClaimActivity is %T", data["orgHypercertsClaimActivity"])
	}

	edges, ok := conn["edges"].([]interface{})
	if !ok || len(edges) == 0 {
		t.Fatalf("expected at least one edge, got %v", conn["edges"])
	}

	edge, ok := edges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("edge[0] is %T", edges[0])
	}
	node, ok := edge["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("node is %T", edge["node"])
	}

	if title, ok := node["title"]; !ok || title != "" {
		t.Errorf("title = %v (%T), want \"\" (coerced zero value)", title, title)
	}
	if sd, ok := node["shortDescription"]; !ok || sd != "" {
		t.Errorf("shortDescription = %v (%T), want \"\" (coerced zero value)", sd, sd)
	}
}

// TestCoerceRequiredFields_PresentFields verifies that required fields that are already
// present in the stored JSON are returned unchanged.
func TestCoerceRequiredFields_PresentFields(t *testing.T) {
	ctx := setupCoercionTestDB(t, `{"title":"My Title","shortDescription":"My Desc","createdAt":"2025-01-01T00:00:00Z"}`)
	schema := buildActivitySchema(t)

	query := `{
		orgHypercertsClaimActivity(first: 10) {
			edges {
				node {
					title
					shortDescription
				}
			}
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("TestCoerceRequiredFields_PresentFields: unexpected GraphQL errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}

	conn, ok := data["orgHypercertsClaimActivity"].(map[string]interface{})
	if !ok {
		t.Fatalf("orgHypercertsClaimActivity is %T", data["orgHypercertsClaimActivity"])
	}

	edges, ok := conn["edges"].([]interface{})
	if !ok || len(edges) == 0 {
		t.Fatalf("expected at least one edge, got %v", conn["edges"])
	}

	edge, ok := edges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("edge[0] is %T", edges[0])
	}
	node, ok := edge["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("node is %T", edge["node"])
	}

	if title, ok := node["title"]; !ok || title != "My Title" {
		t.Errorf("title = %v, want %q (original value preserved)", title, "My Title")
	}
	if sd, ok := node["shortDescription"]; !ok || sd != "My Desc" {
		t.Errorf("shortDescription = %v, want %q (original value preserved)", sd, "My Desc")
	}
}

// TestCoerceRequiredFields_NullFields verifies that required fields that are explicitly
// set to null in the stored JSON are coerced to their zero value ("") when resolved.
func TestCoerceRequiredFields_NullFields(t *testing.T) {
	ctx := setupCoercionTestDB(t, `{"title":null,"shortDescription":null,"createdAt":"2025-01-01T00:00:00Z"}`)
	schema := buildActivitySchema(t)

	query := `{
		orgHypercertsClaimActivity(first: 10) {
			edges {
				node {
					title
					shortDescription
				}
			}
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("TestCoerceRequiredFields_NullFields: unexpected GraphQL errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}

	conn, ok := data["orgHypercertsClaimActivity"].(map[string]interface{})
	if !ok {
		t.Fatalf("orgHypercertsClaimActivity is %T", data["orgHypercertsClaimActivity"])
	}

	edges, ok := conn["edges"].([]interface{})
	if !ok || len(edges) == 0 {
		t.Fatalf("expected at least one edge, got %v", conn["edges"])
	}

	edge, ok := edges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("edge[0] is %T", edges[0])
	}
	node, ok := edge["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("node is %T", edge["node"])
	}

	if title, ok := node["title"]; !ok || title != "" {
		t.Errorf("title = %v (%T), want \"\" (coerced from null)", title, title)
	}
	if sd, ok := node["shortDescription"]; !ok || sd != "" {
		t.Errorf("shortDescription = %v (%T), want \"\" (coerced from null)", sd, sd)
	}
}

// TestCoerceRequiredFields_SingleRecordResolver verifies that the ByUri (single record)
// resolver path also coerces missing required fields to their zero values.
func TestCoerceRequiredFields_SingleRecordResolver(t *testing.T) {
	// Record is missing "title" and "shortDescription" — only "createdAt" is present.
	ctx := setupCoercionTestDB(t, `{"createdAt":"2025-01-01T00:00:00Z"}`)
	schema := buildActivitySchema(t)

	query := `{
		orgHypercertsClaimActivityByUri(uri: "at://did:plc:test/org.hypercerts.claim.activity/rkey1") {
			title
			shortDescription
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("TestCoerceRequiredFields_SingleRecordResolver: unexpected GraphQL errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}

	record, ok := data["orgHypercertsClaimActivityByUri"].(map[string]interface{})
	if !ok {
		t.Fatalf("orgHypercertsClaimActivityByUri is %T, want map[string]interface{}", data["orgHypercertsClaimActivityByUri"])
	}

	if title, ok := record["title"]; !ok || title != "" {
		t.Errorf("title = %v (%T), want \"\" (coerced zero value)", title, title)
	}
	if sd, ok := record["shortDescription"]; !ok || sd != "" {
		t.Errorf("shortDescription = %v (%T), want \"\" (coerced zero value)", sd, sd)
	}
}

// TestExtractFilters_MaxFilterConditions verifies that extractFilters enforces the
// MaxFilterConditions cap and that the DID filter does not count toward the cap.
func TestExtractFilters_MaxFilterConditions(t *testing.T) {
	registry := lexicon.NewRegistry()

	// Helper to build a whereArg with n distinct field filters (each with one operator).
	buildFieldFilters := func(n int) map[string]interface{} {
		m := map[string]interface{}{}
		for i := 0; i < n; i++ {
			m[fmt.Sprintf("field%d", i)] = map[string]interface{}{"eq": "value"}
		}
		return m
	}

	tests := []struct {
		name        string
		whereArg    interface{}
		wantErr     bool
		wantErrMsg  string
		wantFilters int
	}{
		{
			name:        "zero filter conditions succeeds",
			whereArg:    map[string]interface{}{},
			wantFilters: 0,
		},
		{
			name:        "exactly MaxFilterConditions succeeds",
			whereArg:    buildFieldFilters(repositories.MaxFilterConditions),
			wantFilters: repositories.MaxFilterConditions,
		},
		{
			name:       "one over MaxFilterConditions returns error",
			whereArg:   buildFieldFilters(repositories.MaxFilterConditions + 1),
			wantErr:    true,
			wantErrMsg: "too many filter conditions",
		},
		{
			name: "DID filter does not count toward cap",
			whereArg: func() map[string]interface{} {
				// MaxFilterConditions field filters + a DID filter: should still succeed.
				m := buildFieldFilters(repositories.MaxFilterConditions)
				m["did"] = map[string]interface{}{"eq": "did:plc:abc"}
				return m
			}(),
			wantFilters: repositories.MaxFilterConditions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, _, err := extractFilters(tt.whereArg, "com.example.test", registry)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrMsg)
				}
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(filters) != tt.wantFilters {
				t.Errorf("len(filters) = %d, want %d", len(filters), tt.wantFilters)
			}
		})
	}
}

func TestExternalLabelsGraphQLRootAndRecordHydration(t *testing.T) {
	const recordURI = "at://did:plc:test/com.example.label.record/rkey1"

	schema := buildExternalLabelsTestSchema(t)
	ctx := setupExternalLabelsGraphQLTestDB(t)

	query := `{
		externalLabels(subjects: ["at://did:plc:test/com.example.label.record/rkey1"], values: ["high-quality"]) {
			src
			uri
			cid
			val
			neg
			cts
		}
		comExampleLabelRecord(first: 10) {
			edges {
				node {
					uri
					cid
					externalLabels(values: ["high-quality"]) { src val neg }
					history: externalLabels(activeOnly: false, values: ["draft"]) { val neg }
				}
			}
		}
		records(collection: "com.example.label.record", first: 10) {
			edges {
				node {
					uri
					externalLabels(values: ["high-quality"]) { val neg }
				}
			}
		}
		search(query: "hello", collection: "com.example.label.record", first: 10) {
			edges {
				node {
					uri
					externalLabels(values: ["high-quality"]) { val neg }
				}
			}
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}

	rootLabels := data["externalLabels"].([]interface{})
	if len(rootLabels) != 1 {
		t.Fatalf("root externalLabels length = %d, want 1: %v", len(rootLabels), rootLabels)
	}
	rootLabel := rootLabels[0].(map[string]interface{})
	if rootLabel["uri"] != recordURI || rootLabel["val"] != "high-quality" || rootLabel["neg"] != false {
		t.Fatalf("root label = %+v, want active high-quality label for %s", rootLabel, recordURI)
	}

	typedNode := firstConnectionNode(t, data["comExampleLabelRecord"], "comExampleLabelRecord")
	typedLabels := typedNode["externalLabels"].([]interface{})
	if len(typedLabels) != 1 {
		t.Fatalf("typed externalLabels length = %d, want 1: %v", len(typedLabels), typedLabels)
	}
	if got := typedLabels[0].(map[string]interface{})["val"]; got != "high-quality" {
		t.Fatalf("typed externalLabels[0].val = %v, want high-quality", got)
	}

	historyLabels := typedNode["history"].([]interface{})
	if len(historyLabels) != 2 {
		t.Fatalf("history length = %d, want 2: %v", len(historyLabels), historyLabels)
	}
	if got := historyLabels[0].(map[string]interface{})["neg"]; got != true {
		t.Fatalf("history[0].neg = %v, want latest negation first", got)
	}
	if got := historyLabels[1].(map[string]interface{})["neg"]; got != false {
		t.Fatalf("history[1].neg = %v, want older positive second", got)
	}

	genericNode := firstConnectionNode(t, data["records"], "records")
	genericLabels := genericNode["externalLabels"].([]interface{})
	if len(genericLabels) != 1 {
		t.Fatalf("generic externalLabels length = %d, want 1: %v", len(genericLabels), genericLabels)
	}
	if got := genericLabels[0].(map[string]interface{})["val"]; got != "high-quality" {
		t.Fatalf("generic externalLabels[0].val = %v, want high-quality", got)
	}

	searchNode := firstConnectionNode(t, data["search"], "search")
	searchLabels := searchNode["externalLabels"].([]interface{})
	if len(searchLabels) != 1 {
		t.Fatalf("search externalLabels length = %d, want 1: %v", len(searchLabels), searchLabels)
	}
	if got := searchLabels[0].(map[string]interface{})["val"]; got != "high-quality" {
		t.Fatalf("search externalLabels[0].val = %v, want high-quality", got)
	}
}

func TestExternalLabelsGraphQLHydratesLargePages(t *testing.T) {
	schema := buildExternalLabelsTestSchema(t)
	ctx, wantLabelsByURI := setupExternalLabelsLargePageGraphQLTestDB(t)

	query := `{
		comExampleLabelRecord(first: 1000) {
			edges {
				node {
					uri
					externalLabels { val }
				}
			}
			pageInfo { hasNextPage }
		}
	}`

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["comExampleLabelRecord"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	if len(edges) != 1000 {
		t.Fatalf("edges length = %d, want 1000", len(edges))
	}
	pageInfo := conn["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != false {
		t.Fatalf("hasNextPage = %v, want false", pageInfo["hasNextPage"])
	}

	gotLabelsByURI := make(map[string]string)
	for _, edge := range edges {
		node := edge.(map[string]interface{})["node"].(map[string]interface{})
		labels := node["externalLabels"].([]interface{})
		if len(labels) == 0 {
			continue
		}
		if len(labels) > 1 {
			t.Fatalf("externalLabels for %s length = %d, want at most 1", node["uri"], len(labels))
		}
		gotLabelsByURI[node["uri"].(string)] = labels[0].(map[string]interface{})["val"].(string)
	}

	if len(gotLabelsByURI) != len(wantLabelsByURI) {
		t.Fatalf("labeled URI count = %d, want %d; got labels %v", len(gotLabelsByURI), len(wantLabelsByURI), gotLabelsByURI)
	}
	for uri, wantVal := range wantLabelsByURI {
		if gotLabelsByURI[uri] != wantVal {
			t.Fatalf("label for %s = %q, want %q; got labels %v", uri, gotLabelsByURI[uri], wantVal, gotLabelsByURI)
		}
	}
}

func TestExternalLabelsGraphQLHydratesHistoryOnly(t *testing.T) {
	schema := buildExternalLabelsTestSchema(t)
	ctx := setupExternalLabelsGraphQLTestDB(t)

	query := `{
		comExampleLabelRecord(first: 1) {
			edges {
				node {
					history: externalLabels(activeOnly: false, values: ["draft"]) { val neg }
				}
			}
		}
	}`

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	node := firstConnectionNode(t, data["comExampleLabelRecord"], "comExampleLabelRecord")
	historyLabels := node["history"].([]interface{})
	if len(historyLabels) != 2 {
		t.Fatalf("history length = %d, want 2: %v", len(historyLabels), historyLabels)
	}
	if got := historyLabels[0].(map[string]interface{})["neg"]; got != true {
		t.Fatalf("history[0].neg = %v, want latest negation first", got)
	}
	if got := historyLabels[1].(map[string]interface{})["neg"]; got != false {
		t.Fatalf("history[1].neg = %v, want older positive second", got)
	}
}

func TestExternalLabelHydrationRequirementsForSelection(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		variables map[string]interface{}
		want      externalLabelHydrationRequirements
	}{
		{
			name:  "default active labels",
			query: `{ records(collection: "x", first: 1) { edges { node { externalLabels { val } } } } }`,
			want:  externalLabelHydrationRequirements{active: true},
		},
		{
			name:  "history labels",
			query: `{ records(collection: "x", first: 1) { edges { node { externalLabels(activeOnly: false) { val } } } } }`,
			want:  externalLabelHydrationRequirements{history: true},
		},
		{
			name:  "active and history aliases",
			query: `{ records(collection: "x", first: 1) { edges { node { active: externalLabels { val } history: externalLabels(activeOnly: false) { val } } } } }`,
			want:  externalLabelHydrationRequirements{active: true, history: true},
		},
		{
			name:      "activeOnly variable false",
			query:     `query Labels($activeOnly: Boolean) { records(collection: "x", first: 1) { edges { node { externalLabels(activeOnly: $activeOnly) { val } } } } }`,
			variables: map[string]interface{}{"activeOnly": false},
			want:      externalLabelHydrationRequirements{history: true},
		},
		{
			name:      "unknown variable hydrates both to preserve correctness",
			query:     `query Labels($activeOnly: Boolean) { records(collection: "x", first: 1) { edges { node { externalLabels(activeOnly: $activeOnly) { val } } } } }`,
			variables: map[string]interface{}{},
			want:      externalLabelHydrationRequirements{active: true, history: true},
		},
		{
			name: "fragment selection",
			query: `{
				records(collection: "x", first: 1) {
					edges { node { ...RecordLabels } }
				}
			}
			fragment RecordLabels on GenericRecord {
				externalLabels(activeOnly: false) { val }
			}`,
			want: externalLabelHydrationRequirements{history: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := resolveParamsForQueryField(t, tt.query, tt.variables)
			got := externalLabelHydrationRequirementsForPath(params, "edges", "node", "externalLabels")
			if got != tt.want {
				t.Fatalf("requirements = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func resolveParamsForQueryField(t testing.TB, query string, variables map[string]interface{}) graphql.ResolveParams {
	t.Helper()

	document, err := parser.Parse(parser.ParseParams{Source: query})
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}

	fragments := make(map[string]ast.Definition)
	var fieldASTs []*ast.Field
	for _, definition := range document.Definitions {
		switch typedDefinition := definition.(type) {
		case *ast.OperationDefinition:
			for _, selection := range typedDefinition.SelectionSet.Selections {
				field, ok := selection.(*ast.Field)
				if ok {
					fieldASTs = append(fieldASTs, field)
				}
			}
		case *ast.FragmentDefinition:
			fragments[typedDefinition.Name.Value] = typedDefinition
		}
	}
	if len(fieldASTs) == 0 {
		t.Fatal("parsed query has no root field selections")
	}

	return graphql.ResolveParams{Info: graphql.ResolveInfo{
		FieldASTs:      fieldASTs,
		Fragments:      fragments,
		VariableValues: variables,
	}}
}

func TestExternalLabelsGraphQLByURIHydration(t *testing.T) {
	schema := buildExternalLabelsTestSchema(t)
	ctx := setupExternalLabelsGraphQLTestDB(t)

	query := `{
		comExampleLabelRecordByUri(uri: "at://did:plc:test/com.example.label.record/rkey1") {
			uri
			externalLabels(values: ["high-quality"]) { val }
		}
	}`

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	record := data["comExampleLabelRecordByUri"].(map[string]interface{})
	labels := record["externalLabels"].([]interface{})
	if len(labels) != 1 {
		t.Fatalf("ByUri externalLabels length = %d, want 1: %v", len(labels), labels)
	}
}

func TestCertifiedProfileDataGraphQLHydration(t *testing.T) {
	schema := buildCertifiedProfileTestSchema(t)
	ctx := setupCertifiedProfileGraphQLTestDB(t)

	query := `{
		comExampleCertifiedConsumer(first: 10) {
			edges {
				node {
					did
					certifiedProfileData {
						uri
						cid
						did
						rkey
						displayName
						description
						externalLabels(values: ["test-account"]) { src val neg }
					}
				}
			}
		}
		records(collection: "com.example.certified.consumer", first: 10) {
			edges {
				node {
					did
					certifiedProfileData {
						displayName
						externalLabels(values: ["test-account"]) { val }
					}
				}
			}
		}
		comExampleCertifiedConsumerByUri(uri: "at://did:plc:author/com.example.certified.consumer/rkey1") {
			did
			certifiedProfileData { displayName externalLabels(values: ["test-account"]) { val } }
		}
		search(query: "hello", collection: "com.example.certified.consumer", first: 10) {
			edges {
				node {
					did
					certifiedProfileData {
						displayName
						externalLabels(values: ["test-account"]) { val }
					}
				}
			}
		}
	}`

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Data is %T, want map[string]interface{}", result.Data)
	}

	typedNode := firstConnectionNode(t, data["comExampleCertifiedConsumer"], "comExampleCertifiedConsumer")
	assertCertifiedProfileData(t, typedNode, "typed collection")

	genericNode := firstConnectionNode(t, data["records"], "records")
	assertCertifiedProfileData(t, genericNode, "generic records")

	byURI, ok := data["comExampleCertifiedConsumerByUri"].(map[string]interface{})
	if !ok {
		t.Fatalf("ByUri result is %T, want map[string]interface{}", data["comExampleCertifiedConsumerByUri"])
	}
	assertCertifiedProfileData(t, byURI, "ByUri")

	searchNode := firstConnectionNode(t, data["search"], "search")
	assertCertifiedProfileData(t, searchNode, "search")
}

func TestCertifiedProfileDataMissingProfileReturnsNull(t *testing.T) {
	schema := buildCertifiedProfileTestSchema(t)
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	_, err := db.Records.Insert(ctx,
		"at://did:plc:missing/com.example.certified.consumer/rkey1",
		"cid-consumer-missing",
		"did:plc:missing",
		"com.example.certified.consumer",
		`{"text":"hello missing"}`,
	)
	if err != nil {
		t.Fatalf("insert record: %v", err)
	}
	repos := &resolver.Repositories{Records: db.Records, ExternalLabels: db.ExternalLabels}
	ctx = resolver.WithRepositories(ctx, repos)

	query := `{
		comExampleCertifiedConsumer(first: 10) {
			edges { node { certifiedProfileData { displayName } } }
		}
	}`
	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})
	node := firstConnectionNode(t, data["comExampleCertifiedConsumer"], "comExampleCertifiedConsumer")
	if got := node["certifiedProfileData"]; got != nil {
		t.Fatalf("certifiedProfileData = %#v, want nil", got)
	}
}

func buildCertifiedProfileTestSchema(t *testing.T) *graphql.Schema {
	t.Helper()

	registry := lexicon.NewRegistry()
	registerCertifiedProfileTestLexicon(registry)
	registry.Register(&lexicon.Lexicon{
		ID: "com.example.certified.consumer",
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "text", Property: lexicon.Property{Type: lexicon.TypeString}},
				},
			},
		},
	})

	schema, err := NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("buildCertifiedProfileTestSchema: failed to build schema: %v", err)
	}
	return schema
}

func registerCertifiedProfileTestLexicon(registry *lexicon.Registry) {
	registry.Register(&lexicon.Lexicon{
		ID: "app.certified.actor.profile",
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "literal:self",
				Properties: []lexicon.PropertyEntry{
					{Name: "displayName", Property: lexicon.Property{Type: lexicon.TypeString}},
					{Name: "description", Property: lexicon.Property{Type: lexicon.TypeString}},
					{Name: "createdAt", Property: lexicon.Property{Type: lexicon.TypeString, Format: "datetime", Required: true}},
				},
			},
		},
	})
}

func setupCertifiedProfileGraphQLTestDB(t *testing.T) context.Context {
	t.Helper()

	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	consumerURI := "at://did:plc:author/com.example.certified.consumer/rkey1"
	profileURI := "at://did:plc:author/app.certified.actor.profile/self"
	profileCID := "cid-profile"
	if err := db.Records.BatchInsert(ctx, []*repositories.Record{
		{
			URI:        consumerURI,
			CID:        "cid-consumer",
			DID:        "did:plc:author",
			Collection: "com.example.certified.consumer",
			JSON:       `{"text":"hello from author"}`,
			RKey:       "rkey1",
		},
		{
			URI:        profileURI,
			CID:        profileCID,
			DID:        "did:plc:author",
			Collection: "app.certified.actor.profile",
			JSON:       `{"displayName":"Certified Author","description":"Profile record","createdAt":"2025-01-02T03:04:05Z"}`,
			RKey:       "self",
		},
	}); err != nil {
		t.Fatalf("setupCertifiedProfileGraphQLTestDB: insert records: %v", err)
	}

	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := db.ExternalLabels.PersistEvent(ctx, url, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: profileURI, CID: &profileCID, Val: "test-account", Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("setupCertifiedProfileGraphQLTestDB: persist profile label: %v", err)
	}

	repos := &resolver.Repositories{Records: db.Records, ExternalLabels: db.ExternalLabels}
	return resolver.WithRepositories(ctx, repos)
}

func assertCertifiedProfileData(t *testing.T, record map[string]interface{}, path string) {
	t.Helper()

	profile, ok := record["certifiedProfileData"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s certifiedProfileData is %T, want map[string]interface{}", path, record["certifiedProfileData"])
	}
	if got := profile["displayName"]; got != "Certified Author" {
		t.Fatalf("%s profile displayName = %v, want Certified Author", path, got)
	}
	labels, ok := profile["externalLabels"].([]interface{})
	if !ok || len(labels) != 1 {
		t.Fatalf("%s profile externalLabels = %#v, want one label", path, profile["externalLabels"])
	}
	label, ok := labels[0].(map[string]interface{})
	if !ok {
		t.Fatalf("%s profile label is %T, want map[string]interface{}", path, labels[0])
	}
	if got := label["val"]; got != "test-account" {
		t.Fatalf("%s profile label val = %v, want test-account", path, got)
	}
}

func buildExternalLabelsTestSchema(t *testing.T) *graphql.Schema {
	t.Helper()

	registry := lexicon.NewRegistry()
	registry.Register(&lexicon.Lexicon{
		ID: "com.example.label.record",
		Defs: lexicon.Defs{
			Main: &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{Name: "text", Property: lexicon.Property{Type: lexicon.TypeString}},
				},
			},
		},
	})

	schema, err := NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("buildExternalLabelsTestSchema: failed to build schema: %v", err)
	}
	return schema
}

func setupExternalLabelsLargePageGraphQLTestDB(t *testing.T) (context.Context, map[string]string) {
	t.Helper()

	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	records := db.Records
	externalLabels := db.ExternalLabels

	const recordCount = 1000
	allRecords := make([]*repositories.Record, 0, recordCount)
	labelInputs := []repositories.ExternalLabelInput{}
	wantLabelsByURI := make(map[string]string)
	labeledIndexes := map[int]string{
		0:   "first-record",
		250: "second-batch-record",
		999: "last-record",
	}

	for i := range recordCount {
		rkey := fmt.Sprintf("rkey%04d", i)
		uri := "at://did:plc:test/com.example.label.record/" + rkey
		cid := fmt.Sprintf("bafyrecord%04d", i)
		allRecords = append(allRecords, &repositories.Record{
			URI:        uri,
			CID:        cid,
			DID:        "did:plc:test",
			Collection: "com.example.label.record",
			JSON:       fmt.Sprintf(`{"text":"hello %04d"}`, i),
			RKey:       rkey,
		})
		if val, ok := labeledIndexes[i]; ok {
			wantLabelsByURI[uri] = val
			labelInputs = append(labelInputs, repositories.ExternalLabelInput{
				LabelIndex: int64(len(labelInputs)),
				Src:        "did:plc:labeler",
				URI:        uri,
				CID:        stringPtr(cid),
				Val:        val,
				Cts:        fmt.Sprintf("2025-01-02T03:04:%02dZ", len(labelInputs)),
				RawJSON:    `{}`,
			})
		}
	}

	if err := records.BatchInsert(ctx, allRecords); err != nil {
		t.Fatalf("setupExternalLabelsLargePageGraphQLTestDB: failed to insert records: %v", err)
	}

	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := externalLabels.PersistEvent(ctx, url, 1, labelInputs); err != nil {
		t.Fatalf("setupExternalLabelsLargePageGraphQLTestDB: failed to persist labels: %v", err)
	}

	repos := &resolver.Repositories{Records: records, ExternalLabels: externalLabels}
	return resolver.WithRepositories(ctx, repos), wantLabelsByURI
}

func setupExternalLabelsGraphQLTestDB(t *testing.T) context.Context {
	t.Helper()

	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	records := db.Records
	externalLabels := db.ExternalLabels
	const recordURI = "at://did:plc:test/com.example.label.record/rkey1"
	const recordCID = "bafyrecordcid"
	if err := records.BatchInsert(ctx, []*repositories.Record{{
		URI:        recordURI,
		CID:        recordCID,
		DID:        "did:plc:test",
		Collection: "com.example.label.record",
		JSON:       `{"text":"hello"}`,
		RKey:       "rkey1",
	}}); err != nil {
		t.Fatalf("setupExternalLabelsGraphQLTestDB: failed to insert record: %v", err)
	}

	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := externalLabels.PersistEvent(ctx, url, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: recordURI, CID: stringPtr(recordCID), Val: "high-quality", Cts: "2025-01-02T03:04:05Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:labeler", URI: recordURI, CID: stringPtr(recordCID), Val: "draft", Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("setupExternalLabelsGraphQLTestDB: failed to persist first labels: %v", err)
	}
	if err := externalLabels.PersistEvent(ctx, url, 2, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: recordURI, CID: stringPtr(recordCID), Val: "draft", Neg: true, Cts: "2025-01-02T03:06:05Z", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("setupExternalLabelsGraphQLTestDB: failed to persist second labels: %v", err)
	}

	repos := &resolver.Repositories{
		Records:        records,
		ExternalLabels: externalLabels,
	}
	return resolver.WithRepositories(ctx, repos)
}

func stringPtr(value string) *string {
	return &value
}

func TestExternalLabelsGraphQLWhereFilterAndPagination(t *testing.T) {
	schema := buildExternalLabelsTestSchema(t)
	ctx, uris := setupExternalLabelsWhereTestDB(t)

	query := `{
		comExampleLabelRecord(
			first: 2
			where: { externalLabels: { has: { val: { eq: "high-quality" } }, none: { val: { eq: "spam" } } } }
		) {
			edges {
				cursor
				node {
					uri
					externalLabels(values: ["high-quality"]) { val }
				}
			}
			pageInfo { hasNextPage endCursor }
		}
	}`

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["comExampleLabelRecord"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	if len(edges) != 2 {
		t.Fatalf("first page edges = %d, want 2: %v", len(edges), edges)
	}
	firstNode := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	secondNode := edges[1].(map[string]interface{})["node"].(map[string]interface{})
	if firstNode["uri"] != uris["five"] || secondNode["uri"] != uris["three"] {
		t.Fatalf("first page URIs = [%v, %v], want [%s, %s]", firstNode["uri"], secondNode["uri"], uris["five"], uris["three"])
	}
	pageInfo := conn["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != true {
		t.Fatalf("hasNextPage = %v, want true", pageInfo["hasNextPage"])
	}
	endCursor, ok := pageInfo["endCursor"].(string)
	if !ok || endCursor == "" {
		t.Fatalf("endCursor = %v, want non-empty string", pageInfo["endCursor"])
	}

	query = fmt.Sprintf(`{
		comExampleLabelRecord(
			first: 2
			after: %q
			where: { externalLabels: { has: { val: { eq: "high-quality" } }, none: { val: { eq: "spam" } } } }
		) {
			edges { node { uri } }
			pageInfo { hasNextPage }
		}
	}`, endCursor)
	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("second page GraphQL returned errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["comExampleLabelRecord"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	if len(edges) != 1 {
		t.Fatalf("second page edges = %d, want 1: %v", len(edges), edges)
	}
	node := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if node["uri"] != uris["one"] {
		t.Fatalf("second page URI = %v, want %s", node["uri"], uris["one"])
	}
	pageInfo = conn["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != false {
		t.Fatalf("second page hasNextPage = %v, want false", pageInfo["hasNextPage"])
	}
}

func TestExternalLabelsGraphQLWhereNoneAndSourceFilter(t *testing.T) {
	schema := buildExternalLabelsTestSchema(t)
	ctx, uris := setupExternalLabelsWhereTestDB(t)

	query := `{
		comExampleLabelRecord(
			first: 10
			where: {
				externalLabels: {
					has: { src: { in: ["did:plc:labeler"] }, val: { eq: "high-quality" } }
					none: { val: { eq: "spam" } }
				}
			}
		) {
			edges { node { uri } }
		}
	}`

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["comExampleLabelRecord"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	got := make(map[string]bool, len(edges))
	for _, edge := range edges {
		node := edge.(map[string]interface{})["node"].(map[string]interface{})
		got[node["uri"].(string)] = true
	}
	want := []string{uris["one"], uris["three"], uris["five"]}
	if len(got) != len(want) {
		t.Fatalf("filtered URI count = %d, want %d: %v", len(got), len(want), got)
	}
	for _, uri := range want {
		if !got[uri] {
			t.Fatalf("missing URI %s in filtered result %v", uri, got)
		}
	}
	if got[uris["four"]] {
		t.Fatalf("spam-labelled URI %s should have been excluded", uris["four"])
	}
}

func TestAuthorLabelsGraphQLWhereFilterPaginationAndCount(t *testing.T) {
	schema := buildExternalLabelsTestSchema(t)
	ctx, uris := setupAuthorLabelsWhereTestDB(t)

	query := `{
		comExampleLabelRecord(
			first: 2
			where: { authorLabels: { has: { val: { eq: "high-quality" } } } }
		) {
			totalCount
			edges { cursor node { uri did } }
			pageInfo { hasNextPage endCursor }
		}
	}`

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["comExampleLabelRecord"].(map[string]interface{})
	if conn["totalCount"] != float64(3) && conn["totalCount"] != 3 {
		t.Fatalf("totalCount = %v, want 3", conn["totalCount"])
	}
	edges := conn["edges"].([]interface{})
	if len(edges) != 2 {
		t.Fatalf("first page edges = %d, want 2: %v", len(edges), edges)
	}
	firstNode := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	secondNode := edges[1].(map[string]interface{})["node"].(map[string]interface{})
	if firstNode["uri"] != uris["five"] || secondNode["uri"] != uris["three"] {
		t.Fatalf("first page URIs = [%v, %v], want [%s, %s]", firstNode["uri"], secondNode["uri"], uris["five"], uris["three"])
	}
	pageInfo := conn["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != true {
		t.Fatalf("hasNextPage = %v, want true", pageInfo["hasNextPage"])
	}
	endCursor, ok := pageInfo["endCursor"].(string)
	if !ok || endCursor == "" {
		t.Fatalf("endCursor = %v, want non-empty string", pageInfo["endCursor"])
	}

	query = fmt.Sprintf(`{
		comExampleLabelRecord(
			first: 2
			after: %q
			where: { authorLabels: { has: { val: { eq: "high-quality" } } } }
		) {
			edges { node { uri } }
			pageInfo { hasNextPage }
		}
	}`, endCursor)
	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("second page GraphQL returned errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["comExampleLabelRecord"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	if len(edges) != 1 {
		t.Fatalf("second page edges = %d, want 1: %v", len(edges), edges)
	}
	node := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if node["uri"] != uris["one"] {
		t.Fatalf("second page URI = %v, want %s", node["uri"], uris["one"])
	}
	pageInfo = conn["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != false {
		t.Fatalf("second page hasNextPage = %v, want false", pageInfo["hasNextPage"])
	}
}

func TestAuthorLabelsGraphQLWhereNoneAndCombinedRecordFilter(t *testing.T) {
	schema := buildExternalLabelsTestSchema(t)
	ctx, uris := setupAuthorLabelsWhereTestDB(t)

	query := `{
		comExampleLabelRecord(
			first: 10
			where: { authorLabels: { none: { val: { eq: "likely-test" } } } }
		) {
			totalCount
			edges { node { uri did } }
		}
	}`

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL returned errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})
	conn := data["comExampleLabelRecord"].(map[string]interface{})
	if conn["totalCount"] != float64(4) && conn["totalCount"] != 4 {
		t.Fatalf("authorLabels none totalCount = %v, want 4", conn["totalCount"])
	}
	edges := conn["edges"].([]interface{})
	if len(edges) != 4 {
		t.Fatalf("authorLabels none edges = %d, want 4", len(edges))
	}
	for _, edge := range edges {
		node := edge.(map[string]interface{})["node"].(map[string]interface{})
		if node["uri"] == uris["four"] {
			t.Fatalf("likely-test author URI %s should have been excluded", uris["four"])
		}
	}

	query = `{
		comExampleLabelRecord(
			first: 10
			where: {
				externalLabels: { has: { val: { eq: "verified-impact" } } }
				authorLabels: { none: { val: { eq: "likely-test" } } }
			}
		) {
			edges { node { uri } }
		}
	}`
	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("combined GraphQL returned errors: %v", result.Errors)
	}
	data = result.Data.(map[string]interface{})
	conn = data["comExampleLabelRecord"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	if len(edges) != 1 {
		t.Fatalf("combined edges = %d, want 1: %v", len(edges), edges)
	}
	node := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if node["uri"] != uris["one"] {
		t.Fatalf("combined URI = %v, want %s", node["uri"], uris["one"])
	}
}

func setupAuthorLabelsWhereTestDB(t *testing.T) (context.Context, map[string]string) {
	t.Helper()

	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	records := db.Records
	externalLabels := db.ExternalLabels
	uris := map[string]string{}
	for i, rkey := range []string{"one", "two", "three", "four", "five"} {
		did := "did:plc:author-" + rkey
		uri := "at://" + did + "/com.example.label.record/" + rkey
		uris[rkey] = uri
		_, err := records.Insert(ctx, uri, "cid-"+rkey, did, "com.example.label.record", `{"text":"hello `+rkey+`"}`)
		if err != nil {
			t.Fatalf("setupAuthorLabelsWhereTestDB: insert %s: %v", rkey, err)
		}
		indexedAt := fmt.Sprintf("2025-01-02T03:04:%02dZ", i+1)
		if _, err := db.Executor.DB().ExecContext(ctx, "UPDATE record SET indexed_at = ? WHERE uri = ?", indexedAt, uri); err != nil {
			t.Fatalf("setupAuthorLabelsWhereTestDB: set indexed_at %s: %v", rkey, err)
		}
	}

	cidSpecificAccountLabel := "account-label-cid"
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := externalLabels.PersistEvent(ctx, url, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: "did:plc:author-one", Val: "high-quality", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:labeler", URI: "did:plc:author-three", Val: "high-quality", Cts: "2025-01-02T03:05:03Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:labeler", URI: "did:plc:author-four", Val: "likely-test", Cts: "2025-01-02T03:05:04Z", RawJSON: `{}`},
		{LabelIndex: 3, Src: "did:plc:labeler", URI: "did:plc:author-five", Val: "high-quality", Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
		{LabelIndex: 4, Src: "did:plc:labeler", URI: uris["two"], Val: "high-quality", Cts: "2025-01-02T03:05:06Z", RawJSON: `{}`},
		{LabelIndex: 5, Src: "did:plc:labeler", URI: uris["one"], Val: "verified-impact", Cts: "2025-01-02T03:05:07Z", RawJSON: `{}`},
		{LabelIndex: 6, Src: "did:plc:labeler", URI: uris["four"], Val: "verified-impact", Cts: "2025-01-02T03:05:08Z", RawJSON: `{}`},
		{LabelIndex: 7, Src: "did:plc:labeler", URI: "did:plc:author-two", CID: &cidSpecificAccountLabel, Val: "high-quality", Cts: "2025-01-02T03:05:09Z", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("setupAuthorLabelsWhereTestDB: persist labels: %v", err)
	}

	repos := &resolver.Repositories{Records: records, ExternalLabels: externalLabels}
	return resolver.WithRepositories(ctx, repos), uris
}

func setupExternalLabelsWhereTestDB(t *testing.T) (context.Context, map[string]string) {
	t.Helper()

	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	records := db.Records
	externalLabels := db.ExternalLabels
	uris := map[string]string{}
	for i, rkey := range []string{"one", "two", "three", "four", "five"} {
		uri := "at://did:plc:test/com.example.label.record/" + rkey
		uris[rkey] = uri
		_, err := records.Insert(ctx, uri, "cid-"+rkey, "did:plc:test", "com.example.label.record", `{"text":"hello `+rkey+`"}`)
		if err != nil {
			t.Fatalf("setupExternalLabelsWhereTestDB: insert %s: %v", rkey, err)
		}
		indexedAt := fmt.Sprintf("2025-01-02T03:04:%02dZ", i+1)
		if _, err := db.Executor.DB().ExecContext(ctx, "UPDATE record SET indexed_at = ? WHERE uri = ?", indexedAt, uri); err != nil {
			t.Fatalf("setupExternalLabelsWhereTestDB: set indexed_at %s: %v", rkey, err)
		}
	}

	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := externalLabels.PersistEvent(ctx, url, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uris["one"], Val: "high-quality", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:labeler", URI: uris["three"], Val: "high-quality", Cts: "2025-01-02T03:05:03Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:labeler", URI: uris["four"], Val: "high-quality", Cts: "2025-01-02T03:05:04Z", RawJSON: `{}`},
		{LabelIndex: 3, Src: "did:plc:labeler", URI: uris["four"], Val: "spam", Cts: "2025-01-02T03:06:04Z", RawJSON: `{}`},
		{LabelIndex: 4, Src: "did:plc:labeler", URI: uris["five"], Val: "high-quality", Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("setupExternalLabelsWhereTestDB: persist labels: %v", err)
	}

	repos := &resolver.Repositories{Records: records, ExternalLabels: externalLabels}
	return resolver.WithRepositories(ctx, repos), uris
}

func buildRecordTimelineTestSchema(t *testing.T) *graphql.Schema {
	t.Helper()
	lexicons, err := loadLexiconsFromDir("../../../testdata/lexicons")
	if err != nil {
		t.Fatalf("failed to load test lexicons: %v", err)
	}
	registry := lexicon.NewRegistry()
	for _, lex := range lexicons {
		registry.Register(lex)
	}
	schema, err := NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("failed to build record timeline schema: %v", err)
	}
	return schema
}

func setupRecordTimelineGraphQLTest(t *testing.T) (*graphql.Schema, context.Context) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	records := []*repositories.Record{
		{URI: "at://did:plc:alice/org.hypercerts.collection/alice-old", CID: "cid-alice-old", DID: "did:plc:alice", Collection: "org.hypercerts.collection", JSON: `{"title":"Alice old","createdAt":"2026-01-15T10:00:00Z"}`},
		{URI: "at://did:plc:bob/org.hypercerts.collection/bob-new", CID: "cid-bob-new", DID: "did:plc:bob", Collection: "org.hypercerts.collection", JSON: `{"title":"Bob new","createdAt":"2026-01-15T12:00:00Z"}`},
		{URI: "at://did:plc:alice/app.certified.actor.profile/self", CID: "cid-alice-profile", DID: "did:plc:alice", Collection: "app.certified.actor.profile", JSON: `{"displayName":"Alice Profile","createdAt":"2026-01-01T00:00:00Z"}`},
		{URI: "at://did:plc:bob/app.certified.actor.profile/self", CID: "cid-bob-profile", DID: "did:plc:bob", Collection: "app.certified.actor.profile", JSON: `{"displayName":"Bob Profile","createdAt":"2026-01-01T00:00:00Z"}`},
	}
	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("failed to insert timeline test records: %v", err)
	}
	if _, err := db.Executor.DB().ExecContext(ctx, `
		INSERT INTO record (uri, cid, did, collection, json, record_created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		"at://did:plc:bob/org.hypercerts.collection/bob-malformed-newest",
		"cid-bob-malformed-newest",
		"did:plc:bob",
		"org.hypercerts.collection",
		`{"createdAt":`,
		"2026-01-15T13:00:00.000Z",
	); err != nil {
		t.Fatalf("failed to insert malformed timeline row: %v", err)
	}
	repos := &resolver.Repositories{Records: db.Records, ExternalLabels: db.ExternalLabels}
	return buildRecordTimelineTestSchema(t), resolver.WithRepositories(ctx, repos)
}

func TestRecordTimelineGraphQL(t *testing.T) {
	schema, ctx := setupRecordTimelineGraphQLTest(t)

	query := `{
		recordTimeline(where: { did: { in: ["did:plc:alice", "did:plc:bob"] }, collection: { in: ["org.hypercerts.collection"] } }, first: 1) {
			edges {
				cursor
				node {
					uri
					cid
					did
					collection
					rkey
					createdAt
					indexedAt
					json
					certifiedProfileData { displayName }
				}
			}
			pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
		}
	}`
	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("recordTimeline query errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})
	conn := data["recordTimeline"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	if len(edges) != 1 {
		t.Fatalf("edges length = %d, want 1", len(edges))
	}
	edge := edges[0].(map[string]interface{})
	node := edge["node"].(map[string]interface{})
	if got := node["uri"]; got != "at://did:plc:bob/org.hypercerts.collection/bob-new" {
		t.Fatalf("first timeline uri = %v, want bob newest", got)
	}
	if got := node["createdAt"]; got != "2026-01-15T12:00:00.000Z" {
		t.Fatalf("createdAt = %v, want normalized timestamp", got)
	}
	profile := node["certifiedProfileData"].(map[string]interface{})
	if got := profile["displayName"]; got != "Bob Profile" {
		t.Fatalf("certifiedProfileData.displayName = %v, want Bob Profile", got)
	}
	pageInfo := conn["pageInfo"].(map[string]interface{})
	if got := pageInfo["hasNextPage"]; got != true {
		t.Fatalf("hasNextPage = %v, want true", got)
	}
	if got := pageInfo["hasPreviousPage"]; got != false {
		t.Fatalf("hasPreviousPage = %v, want false", got)
	}

	after := edge["cursor"].(string)
	page2Query := fmt.Sprintf(`{
		recordTimeline(where: { did: { in: ["did:plc:alice", "did:plc:bob"] }, collection: { in: ["org.hypercerts.collection"] } }, first: 1, after: %q) {
			edges { node { uri createdAt certifiedProfileData { displayName } } }
			pageInfo { hasNextPage hasPreviousPage }
		}
	}`, after)
	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: page2Query, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("recordTimeline page 2 query errors: %v", result.Errors)
	}
	conn = result.Data.(map[string]interface{})["recordTimeline"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	if len(edges) != 1 {
		t.Fatalf("page 2 edges length = %d, want 1", len(edges))
	}
	node = edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if got := node["uri"]; got != "at://did:plc:alice/org.hypercerts.collection/alice-old" {
		t.Fatalf("page 2 uri = %v, want alice older", got)
	}
	pageInfo = conn["pageInfo"].(map[string]interface{})
	if got := pageInfo["hasNextPage"]; got != false {
		t.Fatalf("page 2 hasNextPage = %v, want false", got)
	}
	if got := pageInfo["hasPreviousPage"]; got != true {
		t.Fatalf("page 2 hasPreviousPage = %v, want true", got)
	}
}

func TestRecordTimelineGraphQLValidationAndShape(t *testing.T) {
	schema, ctx := setupRecordTimelineGraphQLTest(t)

	result := graphql.Do(graphql.Params{Schema: *schema, RequestString: `{ recordTimeline(where: { collection: { in: [] } }, first: 1) { edges { cursor } } }`, Context: ctx})
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "where.collection.in must include at least one") {
		t.Fatalf("empty collections errors = %v, want clear validation error", result.Errors)
	}

	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: `{ recordTimeline(where: { did: { in: [] }, collection: { in: ["org.hypercerts.collection"] } }, first: 1) { edges { cursor } pageInfo { hasNextPage startCursor endCursor } } }`, Context: ctx})
	if len(result.Errors) > 0 {
		t.Fatalf("empty authors query errors: %v", result.Errors)
	}
	conn := result.Data.(map[string]interface{})["recordTimeline"].(map[string]interface{})
	if edges := conn["edges"].([]interface{}); len(edges) != 0 {
		t.Fatalf("where.did.in: [] edges length = %d, want 0", len(edges))
	}

	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: `{ recordTimeline(where: { did: { in: [] }, collection: { in: ["org.hypercerts.collection"] } }, first: 1, after: "not-a-cursor") { edges { cursor } } }`, Context: ctx})
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "invalid recordTimeline cursor") {
		t.Fatalf("empty where.did.in malformed cursor errors = %v, want cursor validation error", result.Errors)
	}

	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: `{ recordTimeline(where: { collection: { in: ["org.hypercerts.collection"] } }, first: 101) { edges { cursor } } }`, Context: ctx})
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "first must be between 1 and 100") {
		t.Fatalf("invalid first errors = %v, want range error", result.Errors)
	}

	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: `{ recordTimeline(where: { collection: { in: ["org.hypercerts.collection"] } }) { totalCount } }`, Context: ctx})
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "totalCount") {
		t.Fatalf("totalCount query errors = %v, want unknown field error", result.Errors)
	}

	result = graphql.Do(graphql.Params{Schema: *schema, RequestString: `{ recordTimeline(where: { collection: { in: [] } }, first: 1) { edges { cursor } } }`, Context: context.Background()})
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "where.collection.in must include at least one") {
		t.Fatalf("no-repository validation errors = %v, want validation before empty connection", result.Errors)
	}
}
