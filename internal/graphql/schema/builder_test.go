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

	"github.com/GainForest/hypergoat/internal/database/migrations"
	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/database/sqlite"
	"github.com/GainForest/hypergoat/internal/graphql/resolver"
	"github.com/GainForest/hypergoat/internal/lexicon"
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

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("setupCoercionTestDB: failed to create SQLite executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("setupCoercionTestDB: failed to run migrations: %v", err)
	}

	records := repositories.NewRecordsRepository(exec)
	rec := &repositories.Record{
		URI:        "at://did:plc:test/org.hypercerts.claim.activity/rkey1",
		CID:        "bafyreiabc123",
		DID:        "did:plc:test",
		Collection: "org.hypercerts.claim.activity",
		JSON:       recordJSON,
		RKey:       "rkey1",
	}
	if err := records.BatchInsert(ctx, []*repositories.Record{rec}); err != nil {
		t.Fatalf("setupCoercionTestDB: failed to insert record: %v", err)
	}

	repos := &resolver.Repositories{
		Records: records,
	}
	return resolver.WithRepositories(ctx, repos)
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
