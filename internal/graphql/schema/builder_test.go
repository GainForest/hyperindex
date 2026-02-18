package schema

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/graphql-go/graphql"

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
			filters, didFilter := extractFilters(tt.whereArg, "com.example.test", registry)

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
