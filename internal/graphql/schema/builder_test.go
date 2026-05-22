package schema

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/lexicon"
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
	}{
		{name: "uri collision", colliding: "uri"},
		{name: "did collision", colliding: "did"},
		{name: "cid collision", colliding: "cid"},
		{name: "rkey collision", colliding: "rkey"},
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

			// The reserved metadata field must still be present and be NonNull String.
			metaField, ok := fields[tt.colliding]
			if !ok {
				t.Fatalf("metadata field %q is missing from the type", tt.colliding)
			}
			if metaField.Type.String() != "String!" {
				t.Errorf("metadata field %q type = %q, want %q (lexicon property must not overwrite it)",
					tt.colliding, metaField.Type.String(), "String!")
			}

			// The normal non-colliding property must still be present.
			if _, ok := fields["title"]; !ok {
				t.Error("non-colliding property 'title' is missing from the type")
			}
		})
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

			// The colliding property must NOT appear as a filter field in the WhereInput.
			// (The reserved metadata field "uri"/"cid"/"rkey" is not added to WhereInput
			// by default — only "did" is added as a metadata filter.)
			// So the colliding property should simply be absent.
			if _, exists := inputFields[tt.colliding]; exists {
				t.Errorf("WhereInput has field %q which should have been skipped (reserved name collision)", tt.colliding)
			}

			// The normal non-colliding property must still appear as a filter.
			if _, exists := inputFields["title"]; !exists {
				t.Error("non-colliding property 'title' is missing from WhereInput")
			}
		})
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

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("setupSchemaRecordTestDB: failed to create SQLite executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("setupSchemaRecordTestDB: failed to run migrations: %v", err)
	}

	records := repositories.NewRecordsRepository(exec)
	if err := records.BatchInsert(ctx, []*repositories.Record{rec}); err != nil {
		t.Fatalf("setupSchemaRecordTestDB: failed to insert record: %v", err)
	}

	repos := &resolver.Repositories{
		Records: records,
	}
	return resolver.WithRepositories(ctx, repos)
}

func buildAuditRecordEventsSchema(t *testing.T) *graphql.Schema {
	t.Helper()

	schema, err := NewBuilder(lexicon.NewRegistry()).Build()
	if err != nil {
		t.Fatalf("buildAuditRecordEventsSchema: failed to build schema: %v", err)
	}
	return schema
}

func setupAuditRecordEventsTestDB(t *testing.T) context.Context {
	t.Helper()

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("setupAuditRecordEventsTestDB: failed to create SQLite executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("setupAuditRecordEventsTestDB: failed to run migrations: %v", err)
	}

	for id := 1; id <= 4; id++ {
		if _, err := exec.DB().ExecContext(ctx, `INSERT INTO raw_tap_events (id, tap_delivery_id, type, received_at, payload)
			VALUES (?, ?, 'record', ?, ?)`, id, 1000+id, fmt.Sprintf("2026-01-01T00:00:0%dZ", id), fmt.Sprintf(`{"id":%d,"type":"record"}`, 1000+id)); err != nil {
			t.Fatalf("setupAuditRecordEventsTestDB: insert raw event %d: %v", id, err)
		}
	}

	auditRows := []struct {
		id         int
		deliveryID int
		receivedAt string
		live       int
		rev        string
		did        string
		collection string
		rkey       string
		action     string
		cid        interface{}
		record     interface{}
	}{
		{1, 1001, "2026-01-01T00:00:01Z", 1, "rev1", "did:plc:alice", "app.example.record", "one", "create", "cid1", `{"text":"one","version":1}`},
		{2, 1002, "2026-01-01T00:00:02Z", 1, "rev2", "did:plc:alice", "app.example.record", "one", "update", "cid2", `{"text":"two","version":2}`},
		{3, 1003, "2026-01-01T00:00:03Z", 1, "rev3", "did:plc:alice", "app.example.record", "two", "delete", nil, nil},
		{4, 1004, "2026-01-01T00:00:04Z", 0, "rev4", "did:plc:bob", "app.other.record", "three", "create", "cid3", `{"text":"bob"}`},
	}
	for _, row := range auditRows {
		uri := fmt.Sprintf("at://%s/%s/%s", row.did, row.collection, row.rkey)
		if _, err := exec.DB().ExecContext(ctx, `INSERT INTO record_events (
				id, event_key, tap_delivery_id, raw_event_id, received_at, live, rev, did, collection, rkey, uri, action, cid, record
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.id, fmt.Sprintf("event-%d", row.id), row.deliveryID, row.id, row.receivedAt, row.live, row.rev, row.did, row.collection, row.rkey, uri, row.action, row.cid, row.record); err != nil {
			t.Fatalf("setupAuditRecordEventsTestDB: insert record event %d: %v", row.id, err)
		}
	}

	repos := &resolver.Repositories{Audit: repositories.NewAuditRepository(exec)}
	return resolver.WithRepositories(ctx, repos)
}

func TestAuditRecordEventsQueryFilters(t *testing.T) {
	schema := buildAuditRecordEventsSchema(t)
	ctx := setupAuditRecordEventsTestDB(t)

	result := graphql.Do(graphql.Params{
		Schema: *schema,
		RequestString: `{
			uriTrail: auditRecordEvents(
				first: 10
				where: { uri: { eq: "at://did:plc:alice/app.example.record/one" } }
				orderBy: { field: ID, direction: ASC }
			) {
				edges { node { id action uri cid record } }
				pageInfo { hasNextPage hasPreviousPage }
			}
			didTrail: auditRecordEvents(
				first: 10
				where: { did: { eq: "did:plc:alice" } }
				orderBy: { field: ID, direction: ASC }
			) {
				edges { node { id did action } }
			}
			deletes: auditRecordEvents(
				first: 10
				where: { collection: { eq: "app.example.record" }, action: { eq: DELETE } }
				orderBy: { field: ID, direction: DESC }
			) {
				edges { node { id action uri record } }
			}
		}`,
		Context: ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("TestAuditRecordEventsQueryFilters: unexpected GraphQL errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})

	uriNodes := auditConnectionNodes(t, data["uriTrail"], "uriTrail")
	if got := auditNodeIDs(uriNodes); strings.Join(got, ",") != "1,2" {
		t.Fatalf("uriTrail ids = %v, want [1 2]", got)
	}
	if uriNodes[0]["action"] != "CREATE" || uriNodes[1]["action"] != "UPDATE" {
		t.Fatalf("uriTrail actions = %v, %v; want CREATE, UPDATE", uriNodes[0]["action"], uriNodes[1]["action"])
	}
	if uriNodes[0]["cid"] != "cid1" {
		t.Fatalf("uriTrail first cid = %v, want cid1", uriNodes[0]["cid"])
	}
	record, ok := uriNodes[0]["record"].(map[string]interface{})
	if !ok {
		t.Fatalf("uriTrail first record is %T, want JSON object", uriNodes[0]["record"])
	}
	if record["text"] != "one" {
		t.Fatalf("uriTrail first record.text = %v, want one", record["text"])
	}

	didNodes := auditConnectionNodes(t, data["didTrail"], "didTrail")
	if got := auditNodeIDs(didNodes); strings.Join(got, ",") != "1,2,3" {
		t.Fatalf("didTrail ids = %v, want [1 2 3]", got)
	}

	deleteNodes := auditConnectionNodes(t, data["deletes"], "deletes")
	if got := auditNodeIDs(deleteNodes); strings.Join(got, ",") != "3" {
		t.Fatalf("deletes ids = %v, want [3]", got)
	}
	if deleteNodes[0]["action"] != "DELETE" {
		t.Fatalf("delete action = %v, want DELETE", deleteNodes[0]["action"])
	}
	if deleteNodes[0]["record"] != nil {
		t.Fatalf("delete record = %v, want nil", deleteNodes[0]["record"])
	}
}

func TestAuditRecordEventsQueryCursorPagination(t *testing.T) {
	schema := buildAuditRecordEventsSchema(t)
	ctx := setupAuditRecordEventsTestDB(t)

	firstPageResult := graphql.Do(graphql.Params{
		Schema: *schema,
		RequestString: `{
			auditRecordEvents(first: 2, orderBy: { field: ID, direction: ASC }) {
				edges { cursor node { id } }
				pageInfo { hasNextPage hasPreviousPage endCursor }
			}
		}`,
		Context: ctx,
	})
	if len(firstPageResult.Errors) > 0 {
		t.Fatalf("first page GraphQL errors: %v", firstPageResult.Errors)
	}
	firstConnection := firstPageResult.Data.(map[string]interface{})["auditRecordEvents"].(map[string]interface{})
	firstNodes := auditConnectionNodes(t, firstConnection, "first page")
	if got := auditNodeIDs(firstNodes); strings.Join(got, ",") != "1,2" {
		t.Fatalf("first page ids = %v, want [1 2]", got)
	}
	firstPageInfo := firstConnection["pageInfo"].(map[string]interface{})
	if firstPageInfo["hasNextPage"] != true || firstPageInfo["hasPreviousPage"] != false {
		t.Fatalf("first pageInfo = %v, want hasNextPage true and hasPreviousPage false", firstPageInfo)
	}
	endCursor, ok := firstPageInfo["endCursor"].(string)
	if !ok || endCursor == "" {
		t.Fatalf("first page endCursor = %v, want non-empty string", firstPageInfo["endCursor"])
	}

	secondPageResult := graphql.Do(graphql.Params{
		Schema: *schema,
		RequestString: `query($after: String!) {
			auditRecordEvents(first: 2, after: $after, orderBy: { field: ID, direction: ASC }) {
				edges { node { id } }
				pageInfo { hasNextPage hasPreviousPage }
			}
		}`,
		VariableValues: map[string]interface{}{"after": endCursor},
		Context:        ctx,
	})
	if len(secondPageResult.Errors) > 0 {
		t.Fatalf("second page GraphQL errors: %v", secondPageResult.Errors)
	}
	secondConnection := secondPageResult.Data.(map[string]interface{})["auditRecordEvents"].(map[string]interface{})
	secondNodes := auditConnectionNodes(t, secondConnection, "second page")
	if got := auditNodeIDs(secondNodes); strings.Join(got, ",") != "3,4" {
		t.Fatalf("second page ids = %v, want [3 4]", got)
	}
	secondPageInfo := secondConnection["pageInfo"].(map[string]interface{})
	if secondPageInfo["hasNextPage"] != false || secondPageInfo["hasPreviousPage"] != true {
		t.Fatalf("second pageInfo = %v, want hasNextPage false and hasPreviousPage true", secondPageInfo)
	}

	descResult := graphql.Do(graphql.Params{
		Schema: *schema,
		RequestString: `{
			auditRecordEvents(first: 2, orderBy: { field: ID, direction: DESC }) {
				edges { node { id } }
			}
		}`,
		Context: ctx,
	})
	if len(descResult.Errors) > 0 {
		t.Fatalf("desc page GraphQL errors: %v", descResult.Errors)
	}
	descNodes := auditConnectionNodes(t, descResult.Data.(map[string]interface{})["auditRecordEvents"], "desc page")
	if got := auditNodeIDs(descNodes); strings.Join(got, ",") != "4,3" {
		t.Fatalf("desc page ids = %v, want [4 3]", got)
	}
}

func TestAuditRecordEventsQueryTotalCount(t *testing.T) {
	schema := buildAuditRecordEventsSchema(t)
	ctx := setupAuditRecordEventsTestDB(t)

	result := graphql.Do(graphql.Params{
		Schema: *schema,
		RequestString: `{
			auditRecordEvents(
				first: 1
				where: { did: { eq: "did:plc:alice" } }
				orderBy: { field: ID, direction: ASC }
			) {
				edges { node { id } }
				totalCount
			}
			missing: auditRecordEvents(
				first: 1
				where: { collection: { eq: "app.missing.record" } }
			) {
				edges { node { id } }
				totalCount
			}
		}`,
		Context: ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("totalCount GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	aliceConnection := data["auditRecordEvents"].(map[string]interface{})
	aliceNodes := auditConnectionNodes(t, aliceConnection, "alice totalCount")
	if got := auditNodeIDs(aliceNodes); strings.Join(got, ",") != "1" {
		t.Fatalf("alice page ids = %v, want [1]", got)
	}
	if aliceConnection["totalCount"] != 3 {
		t.Fatalf("alice totalCount = %v, want 3", aliceConnection["totalCount"])
	}

	missingConnection := data["missing"].(map[string]interface{})
	missingEdges := missingConnection["edges"].([]interface{})
	if len(missingEdges) != 0 {
		t.Fatalf("missing edges = %v, want empty", missingEdges)
	}
	if missingConnection["totalCount"] != 0 {
		t.Fatalf("missing totalCount = %v, want 0", missingConnection["totalCount"])
	}
}

func TestAuditRecordEventsQueryWithoutRepositoryReturnsEmptyConnection(t *testing.T) {
	schema := buildAuditRecordEventsSchema(t)

	result := graphql.Do(graphql.Params{
		Schema: *schema,
		RequestString: `{
			auditRecordEvents(first: 10) {
				edges { node { id } }
				pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
				totalCount
			}
		}`,
		Context: context.Background(),
	})
	if len(result.Errors) > 0 {
		t.Fatalf("empty audit repository GraphQL errors: %v", result.Errors)
	}
	connection := result.Data.(map[string]interface{})["auditRecordEvents"].(map[string]interface{})
	if edges := connection["edges"].([]interface{}); len(edges) != 0 {
		t.Fatalf("empty repository edges = %v, want empty", edges)
	}
	pageInfo := connection["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != false || pageInfo["hasPreviousPage"] != false || pageInfo["startCursor"] != nil || pageInfo["endCursor"] != nil {
		t.Fatalf("empty repository pageInfo = %v, want empty page info", pageInfo)
	}
	if connection["totalCount"] != 0 {
		t.Fatalf("empty repository totalCount = %v, want 0", connection["totalCount"])
	}
}

func auditConnectionNodes(t *testing.T, connectionValue interface{}, fieldName string) []map[string]interface{} {
	t.Helper()

	connection, ok := connectionValue.(map[string]interface{})
	if !ok {
		t.Fatalf("%s is %T, want map[string]interface{}", fieldName, connectionValue)
	}
	edges, ok := connection["edges"].([]interface{})
	if !ok {
		t.Fatalf("%s edges is %T, want []interface{}", fieldName, connection["edges"])
	}
	nodes := make([]map[string]interface{}, 0, len(edges))
	for i, edgeValue := range edges {
		edge, ok := edgeValue.(map[string]interface{})
		if !ok {
			t.Fatalf("%s edge %d is %T, want map[string]interface{}", fieldName, i, edgeValue)
		}
		node, ok := edge["node"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s edge %d node is %T, want map[string]interface{}", fieldName, i, edge["node"])
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func auditNodeIDs(nodes []map[string]interface{}) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, fmt.Sprint(node["id"]))
	}
	return ids
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
