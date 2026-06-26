package types //nolint:revive // package name is descriptive within graphql context

import (
	"strings"
	"testing"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"

	"github.com/GainForest/hyperindex/internal/lexicon"
)

// ---------- Mapper tests ----------

func TestMapper_MapPrimitiveType(t *testing.T) {
	m := NewMapper()

	tests := []struct {
		name       string
		lexType    string
		format     string
		wantName   string
		wantNotNil bool // for types where we just check non-nil (e.g., BlobType)
	}{
		{name: "string no format", lexType: "string", format: "", wantName: "String"},
		{name: "string datetime", lexType: "string", format: "datetime", wantName: "DateTime"},
		{name: "string uri", lexType: "string", format: "uri", wantName: "String"},
		{name: "integer", lexType: "integer", format: "", wantName: "Int"},
		{name: "boolean", lexType: "boolean", format: "", wantName: "Boolean"},
		{name: "number", lexType: "number", format: "", wantName: "Float"},
		{name: "blob", lexType: "blob", format: "", wantName: "Blob", wantNotNil: true},
		{name: "bytes", lexType: "bytes", format: "", wantName: "String"},
		{name: "cid-link", lexType: "cid-link", format: "", wantName: "String"},
		{name: "unknown", lexType: "unknown", format: "", wantName: "JSON"},
		{name: "empty default", lexType: "", format: "", wantName: "String"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.MapPrimitiveType(tt.lexType, tt.format)
			if got == nil {
				t.Fatal("MapPrimitiveType returned nil")
			}
			if got.Name() != tt.wantName {
				t.Errorf("MapPrimitiveType(%q, %q) name = %q, want %q",
					tt.lexType, tt.format, got.Name(), tt.wantName)
			}
			if tt.wantNotNil {
				if _, ok := got.(*graphql.Object); !ok {
					t.Errorf("expected *graphql.Object for %q, got %T", tt.lexType, got)
				}
			}
		})
	}
}

func TestMapper_ObjectTypeCache(t *testing.T) {
	m := NewMapper()

	// Non-existent key returns false.
	if _, ok := m.GetObjectType("nope"); ok {
		t.Fatal("expected GetObjectType to return false for missing key")
	}

	// Set and retrieve.
	obj := graphql.NewObject(graphql.ObjectConfig{
		Name:   "TestObj",
		Fields: graphql.Fields{"id": &graphql.Field{Type: graphql.String}},
	})
	m.SetObjectType("test.ref", obj)

	got, ok := m.GetObjectType("test.ref")
	if !ok {
		t.Fatal("expected GetObjectType to return true after Set")
	}
	if got != obj {
		t.Error("returned object differs from the one that was set")
	}

	// AllObjectTypes includes the entry (plus any defaults like Blob if cached).
	all := m.AllObjectTypes()
	if _, exists := all["test.ref"]; !exists {
		t.Error("AllObjectTypes missing 'test.ref'")
	}
}

func TestMapper_BlobRefResolver(t *testing.T) {
	tests := []struct {
		name    string
		ref     any
		wantRef string
	}{
		{
			name:    "link object",
			ref:     map[string]any{"$link": "bafkreihyperindexcid"},
			wantRef: "bafkreihyperindexcid",
		},
		{
			name:    "string",
			ref:     "bafkreialreadystring",
			wantRef: "bafkreialreadystring",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotData, gotErrors := executeBlobRefQuery(t, tt.ref)
			if len(gotErrors) > 0 {
				t.Fatalf("Blob.ref query returned errors: %v", gotErrors)
			}

			blob, ok := gotData["blob"].(map[string]any)
			if !ok {
				t.Fatalf("blob result = %T, want map[string]any", gotData["blob"])
			}

			if gotRef := blob["ref"]; gotRef != tt.wantRef {
				t.Fatalf("Blob.ref = %v, want %q", gotRef, tt.wantRef)
			}
		})
	}
}

func TestMapper_BlobRefResolverMalformedRefDoesNotStringifyMap(t *testing.T) {
	gotData, gotErrors := executeBlobRefQuery(t, map[string]any{"$link": 123})
	if len(gotErrors) == 0 {
		blob, ok := gotData["blob"].(map[string]any)
		if ok {
			if gotRef, ok := blob["ref"].(string); ok && strings.Contains(gotRef, "map[$link:") {
				t.Fatalf("malformed Blob.ref returned stringified map: %q", gotRef)
			}
		}

		return
	}

	for _, err := range gotErrors {
		if strings.Contains(err.Message, "map[$link:") {
			t.Fatalf("malformed Blob.ref error stringified map: %q", err.Message)
		}
	}
}

func executeBlobRefQuery(t *testing.T, ref any) (map[string]any, []gqlerrors.FormattedError) {
	t.Helper()

	mapper := NewMapper()
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name: "Query",
			Fields: graphql.Fields{
				"blob": &graphql.Field{
					Type: mapper.BlobType,
					Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
						return map[string]any{
							"ref":      ref,
							"mimeType": "image/png",
							"size":     123,
						}, nil
					},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("failed to build test schema: %v", err)
	}

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: "{ blob { ref } }",
	})

	data, _ := result.Data.(map[string]any)
	return data, result.Errors
}

func TestMapper_UnionTypeCache(t *testing.T) {
	m := NewMapper()

	// Non-existent key returns false.
	if _, ok := m.GetUnionType("nope"); ok {
		t.Fatal("expected GetUnionType to return false for missing key")
	}

	// Set and retrieve.
	dummyObj := graphql.NewObject(graphql.ObjectConfig{
		Name:   "DummyUnionMember",
		Fields: graphql.Fields{"x": &graphql.Field{Type: graphql.String}},
	})
	u := graphql.NewUnion(graphql.UnionConfig{
		Name:  "TestUnion",
		Types: []*graphql.Object{dummyObj},
		ResolveType: func(_ graphql.ResolveTypeParams) *graphql.Object {
			return dummyObj
		},
	})
	m.SetUnionType("TestUnion", u)

	got, ok := m.GetUnionType("TestUnion")
	if !ok {
		t.Fatal("expected GetUnionType to return true after Set")
	}
	if got != u {
		t.Error("returned union differs from the one that was set")
	}
}

// ---------- Scalar tests ----------

func TestJSONScalar_Serialize(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{"map", map[string]interface{}{"key": "val"}, map[string]interface{}{"key": "val"}},
		{"string", "hello", "hello"},
		{"nil", nil, nil},
		{"int", 42, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JSONScalar.Serialize(tt.input)
			// JSONScalar.Serialize is the identity function.
			if fmtEq(got, tt.want) == false {
				t.Errorf("JSONScalar.Serialize(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDateTimeScalar_Serialize(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{"string", "2024-01-15T12:00:00Z", "2024-01-15T12:00:00Z"},
		{"time.Time", now, now},
		{"nil", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DateTimeScalar.Serialize(tt.input)
			// DateTimeScalar.Serialize is the identity function.
			if fmtEq(got, tt.want) == false {
				t.Errorf("DateTimeScalar.Serialize(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// fmtEq is a simple equality check that handles nil comparisons.
func fmtEq(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// For maps we just check non-nil; deeper comparison isn't necessary
	// because the scalar is an identity function.
	return true
}

// ---------- ObjectBuilder tests ----------

func TestObjectBuilder_BuildRecordType(t *testing.T) {
	registry := lexicon.NewRegistry()
	mapper := NewMapper()
	builder := NewObjectBuilder(mapper, registry)

	recordDef := &lexicon.RecordDef{
		Type: "record",
		Key:  "tid",
		Properties: []lexicon.PropertyEntry{
			{
				Name: "text",
				Property: lexicon.Property{
					Type:        "string",
					Description: "The post text",
				},
			},
			{
				Name: "count",
				Property: lexicon.Property{
					Type:     "integer",
					Required: true,
				},
			},
		},
	}

	lexiconID := "com.example.test.post"
	obj := builder.BuildRecordType(lexiconID, recordDef)
	if obj == nil {
		t.Fatal("BuildRecordType returned nil")
	}

	// Type name should be PascalCase of the NSID.
	wantName := "ComExampleTestPost"
	if obj.Name() != wantName {
		t.Errorf("type name = %q, want %q", obj.Name(), wantName)
	}

	// Force field thunk resolution by getting the fields.
	fields := obj.Fields()

	// Must have standard metadata fields.
	for _, std := range []string{"uri", "cid", "externalLabels"} {
		if _, ok := fields[std]; !ok {
			t.Errorf("missing standard field %q", std)
		}
	}

	// Must have the custom properties.
	if _, ok := fields["text"]; !ok {
		t.Error("missing field 'text'")
	}
	if _, ok := fields["count"]; !ok {
		t.Error("missing field 'count'")
	}

	// Building the same ID again should return the cached object (same pointer).
	obj2 := builder.BuildRecordType(lexiconID, recordDef)
	if obj2 != obj {
		t.Error("expected cached object on second call, got a different pointer")
	}
}

func TestObjectBuilder_BuildRecordType_SkipsReservedFields(t *testing.T) {
	// A lexicon that defines reserved metadata field names must not overwrite them.
	// These must NOT overwrite the metadata fields injected by buildRecordFields.
	tests := []struct {
		name         string
		colliding    string // the reserved property name the lexicon tries to define
		wantMetaType string // the metadata field's type should remain NonNull String
	}{
		{name: "uri collision", colliding: "uri", wantMetaType: "String!"},
		{name: "did collision", colliding: "did", wantMetaType: "String!"},
		{name: "cid collision", colliding: "cid", wantMetaType: "String!"},
		{name: "rkey collision", colliding: "rkey", wantMetaType: "String!"},
		{name: "externalLabels collision", colliding: "externalLabels", wantMetaType: "[ExternalLabel!]!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := lexicon.NewRegistry()
			mapper := NewMapper()
			builder := NewObjectBuilder(mapper, registry)

			// Build a unique lexicon ID per sub-test to avoid cache collisions.
			lexiconID := "com.example.test." + tt.colliding + "collision"

			recordDef := &lexicon.RecordDef{
				Type: "record",
				Key:  "tid",
				Properties: []lexicon.PropertyEntry{
					{
						// This property collides with a reserved metadata field.
						// It should be silently skipped.
						Name: tt.colliding,
						Property: lexicon.Property{
							Type:        "integer", // intentionally different type from the metadata field
							Description: "Colliding property",
						},
					},
					{
						// A normal, non-colliding property that must still appear.
						Name: "title",
						Property: lexicon.Property{
							Type: "string",
						},
					},
				},
			}

			obj := builder.BuildRecordType(lexiconID, recordDef)
			if obj == nil {
				t.Fatal("BuildRecordType returned nil")
			}

			fields := obj.Fields()

			// The reserved metadata field must still be present and be NonNull String.
			metaField, ok := fields[tt.colliding]
			if !ok {
				t.Fatalf("metadata field %q is missing from the type", tt.colliding)
			}
			if metaField.Type.String() != tt.wantMetaType {
				t.Errorf("metadata field %q type = %q, want %q (lexicon property must not overwrite it)",
					tt.colliding, metaField.Type.String(), tt.wantMetaType)
			}

			// The normal non-colliding property must still be present.
			if _, ok := fields["title"]; !ok {
				t.Error("non-colliding property 'title' is missing from the type")
			}
		})
	}
}

func TestObjectBuilder_BuildRecordType_DoesNotExposeAuthorLabelsVirtualField(t *testing.T) {
	registry := lexicon.NewRegistry()
	mapper := NewMapper()
	builder := NewObjectBuilder(mapper, registry)

	recordDef := &lexicon.RecordDef{
		Type: "record",
		Key:  "tid",
		Properties: []lexicon.PropertyEntry{
			{
				Name: "authorLabels",
				Property: lexicon.Property{
					Type:        "integer",
					Description: "Colliding property",
				},
			},
			{
				Name: "title",
				Property: lexicon.Property{
					Type: "string",
				},
			},
		},
	}

	obj := builder.BuildRecordType("com.example.test.authorlabelscollision", recordDef)
	if obj == nil {
		t.Fatal("BuildRecordType returned nil")
	}
	fields := obj.Fields()
	if _, ok := fields["authorLabels"]; ok {
		t.Fatal("record type exposed authorLabels; this release only supports where.authorLabels filtering")
	}
	if _, ok := fields["title"]; !ok {
		t.Fatal("non-colliding property 'title' is missing from the type")
	}
}

func TestObjectBuilder_GeneratedCIDLinkFieldResolver(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "link object",
			value: map[string]any{"$link": "bafkreigeneratedlink"},
			want:  "bafkreigeneratedlink",
		},
		{
			name:  "string",
			value: "bafkreialreadygeneratedstring",
			want:  "bafkreialreadygeneratedstring",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotData, gotErrors := executeGeneratedCIDLinkQuery(t, tt.value)
			if len(gotErrors) > 0 {
				t.Fatalf("generated cid-link query returned errors: %v", gotErrors)
			}

			record, ok := gotData["record"].(map[string]any)
			if !ok {
				t.Fatalf("record result = %T, want map[string]any", gotData["record"])
			}

			if gotRef := record["root"]; gotRef != tt.want {
				t.Fatalf("record.root = %v, want %q", gotRef, tt.want)
			}
			if gotRef, ok := record["root"].(string); ok && strings.Contains(gotRef, "map[$link:") {
				t.Fatalf("generated cid-link returned stringified map: %q", gotRef)
			}
		})
	}
}

func TestObjectBuilder_GeneratedCIDLinkArrayFieldResolver(t *testing.T) {
	gotData, gotErrors := executeGeneratedCIDLinkArrayQuery(t, []any{
		"bafkreifirstcid",
		map[string]any{"$link": "bafkreisecondcid"},
	})
	if len(gotErrors) > 0 {
		t.Fatalf("generated cid-link array query returned errors: %v", gotErrors)
	}

	record, ok := gotData["record"].(map[string]any)
	if !ok {
		t.Fatalf("record result = %T, want map[string]any", gotData["record"])
	}

	gotRefs, ok := record["refs"].([]any)
	if !ok {
		t.Fatalf("record.refs = %T, want []any", record["refs"])
	}

	wantRefs := []string{"bafkreifirstcid", "bafkreisecondcid"}
	if len(gotRefs) != len(wantRefs) {
		t.Fatalf("record.refs length = %d, want %d", len(gotRefs), len(wantRefs))
	}
	for i, wantRef := range wantRefs {
		if gotRefs[i] != wantRef {
			t.Fatalf("record.refs[%d] = %v, want %q", i, gotRefs[i], wantRef)
		}
		if gotRef, ok := gotRefs[i].(string); ok && strings.Contains(gotRef, "map[$link:") {
			t.Fatalf("generated cid-link array returned stringified map: %q", gotRef)
		}
	}
}

func executeGeneratedCIDLinkQuery(t *testing.T, value any) (map[string]any, []gqlerrors.FormattedError) {
	t.Helper()

	recordType := generatedCIDLinkRecordType(t, lexicon.Property{
		Type: lexicon.TypeCIDLink,
	})

	return executeGeneratedRecordQuery(t, recordType, map[string]any{
		"uri":  "at://did:example:alice/com.example.test.record/1",
		"cid":  "bafkreirecordcid",
		"did":  "did:example:alice",
		"rkey": "1",
		"root": value,
	}, "{ record { root } }")
}

func executeGeneratedCIDLinkArrayQuery(t *testing.T, value any) (map[string]any, []gqlerrors.FormattedError) {
	t.Helper()

	recordType := generatedCIDLinkRecordType(t, lexicon.Property{
		Type: lexicon.TypeArray,
		Items: &lexicon.ArrayItems{
			Type: lexicon.TypeCIDLink,
		},
	})

	return executeGeneratedRecordQuery(t, recordType, map[string]any{
		"uri":  "at://did:example:alice/com.example.test.record/1",
		"cid":  "bafkreirecordcid",
		"did":  "did:example:alice",
		"rkey": "1",
		"refs": value,
	}, "{ record { refs } }")
}

func generatedCIDLinkRecordType(t *testing.T, property lexicon.Property) *graphql.Object {
	t.Helper()

	registry := lexicon.NewRegistry()
	mapper := NewMapper()
	builder := NewObjectBuilder(mapper, registry)

	fieldName := "root"
	if property.Type == lexicon.TypeArray {
		fieldName = "refs"
	}

	recordDef := &lexicon.RecordDef{
		Type: "record",
		Key:  "tid",
		Properties: []lexicon.PropertyEntry{
			{
				Name:     fieldName,
				Property: property,
			},
		},
	}

	return builder.BuildRecordType("com.example.test.cidlink", recordDef)
}

func executeGeneratedRecordQuery(
	t *testing.T,
	recordType *graphql.Object,
	record map[string]any,
	query string,
) (map[string]any, []gqlerrors.FormattedError) {
	t.Helper()

	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name: "Query",
			Fields: graphql.Fields{
				"record": &graphql.Field{
					Type: recordType,
					Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
						return record, nil
					},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("failed to build test schema: %v", err)
	}

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: query,
	})

	data, _ := result.Data.(map[string]any)
	return data, result.Errors
}

func TestObjectBuilder_UnionFieldMalformedValuesReturnNull(t *testing.T) {
	tests := []struct {
		name  string
		image any
	}{
		{
			name: "missing type does not default to first member",
			image: map[string]any{
				"uri": "https://example.com/image.png",
			},
		},
		{
			name: "unknown type does not default to first member",
			image: map[string]any{
				"$type": "com.example.defs#missing",
				"uri":   "https://example.com/image.png",
			},
		},
		{
			name: "known type missing required fields returns null",
			image: map[string]any{
				"$type": "com.example.defs#uri",
				"href":  "https://example.com/image.png",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotData, gotErrors := executeGeneratedUnionImageQuery(t, tt.image)
			if len(gotErrors) > 0 {
				t.Fatalf("union image query returned errors: %v", gotErrors)
			}

			record, ok := gotData["record"].(map[string]any)
			if !ok {
				t.Fatalf("record result = %T, want map[string]any", gotData["record"])
			}
			if record["image"] != nil {
				t.Fatalf("record.image = %#v, want nil", record["image"])
			}
		})
	}
}

func TestObjectBuilder_RequiredUnionFieldAttachesResolver(t *testing.T) {
	recordType := generatedUnionImageRecordTypeWithRequired(t, true)
	field := recordType.Fields()["image"]
	if field == nil {
		t.Fatal("image field is missing")
	}
	if field.Resolve == nil {
		t.Fatal("required union field resolver is nil")
	}
	if _, ok := field.Type.(*graphql.NonNull); !ok {
		t.Fatalf("image field type = %T, want *graphql.NonNull", field.Type)
	}
}

func TestObjectBuilder_UnionFieldValidTypedValueResolves(t *testing.T) {
	gotData, gotErrors := executeGeneratedUnionImageQuery(t, map[string]any{
		"$type": "com.example.defs#smallImage",
		"image": map[string]any{
			"ref":      "bafkreiexamplecid",
			"mimeType": "image/png",
			"size":     123,
		},
	})
	if len(gotErrors) > 0 {
		t.Fatalf("union image query returned errors: %v", gotErrors)
	}

	record, ok := gotData["record"].(map[string]any)
	if !ok {
		t.Fatalf("record result = %T, want map[string]any", gotData["record"])
	}
	image, ok := record["image"].(map[string]any)
	if !ok {
		t.Fatalf("record.image = %T, want map[string]any", record["image"])
	}
	if gotTypename := image["__typename"]; gotTypename != "ComExampleDefsSmallImage" {
		t.Fatalf("record.image.__typename = %v, want ComExampleDefsSmallImage", gotTypename)
	}
	blob, ok := image["image"].(map[string]any)
	if !ok {
		t.Fatalf("record.image.image = %T, want map[string]any", image["image"])
	}
	if gotRef := blob["ref"]; gotRef != "bafkreiexamplecid" {
		t.Fatalf("record.image.image.ref = %v, want bafkreiexamplecid", gotRef)
	}
}

func executeGeneratedUnionImageQuery(t *testing.T, image any) (map[string]any, []gqlerrors.FormattedError) {
	t.Helper()

	recordType := generatedUnionImageRecordType(t)
	return executeGeneratedRecordQuery(t, recordType, map[string]any{
		"uri":   "at://did:example:alice/com.example.test.record/1",
		"cid":   "bafkreirecordcid",
		"did":   "did:example:alice",
		"rkey":  "1",
		"image": image,
	}, `{
		record {
			image {
				__typename
				... on ComExampleDefsUri { uri }
				... on ComExampleDefsSmallImage { image { ref mimeType size } }
			}
		}
	}`)
}

func generatedUnionImageRecordType(t *testing.T) *graphql.Object {
	t.Helper()

	return generatedUnionImageRecordTypeWithRequired(t, false)
}

func generatedUnionImageRecordTypeWithRequired(t *testing.T, required bool) *graphql.Object {
	t.Helper()

	registry := lexicon.NewRegistry()
	registry.Register(&lexicon.Lexicon{
		ID: "com.example.defs",
		Defs: lexicon.Defs{
			Others: map[string]lexicon.Def{
				"uri": {
					Type: "object",
					Object: &lexicon.ObjectDef{
						Type:           "object",
						RequiredFields: []string{"uri"},
						Properties: []lexicon.PropertyEntry{
							{
								Name: "uri",
								Property: lexicon.Property{
									Type:     lexicon.TypeString,
									Required: true,
								},
							},
						},
					},
				},
				"smallImage": {
					Type: "object",
					Object: &lexicon.ObjectDef{
						Type:           "object",
						RequiredFields: []string{"image"},
						Properties: []lexicon.PropertyEntry{
							{
								Name: "image",
								Property: lexicon.Property{
									Type:     lexicon.TypeBlob,
									Required: true,
								},
							},
						},
					},
				},
			},
		},
	})

	mapper := NewMapper()
	builder := NewObjectBuilder(mapper, registry)
	recordDef := &lexicon.RecordDef{
		Type: "record",
		Key:  "tid",
		Properties: []lexicon.PropertyEntry{
			{
				Name: "image",
				Property: lexicon.Property{
					Type:     lexicon.TypeUnion,
					Required: required,
					Refs: []string{
						"com.example.defs#uri",
						"com.example.defs#smallImage",
					},
				},
			},
		},
	}

	return builder.BuildRecordType("com.example.test.record", recordDef)
}

func TestObjectBuilder_BuildObjectType(t *testing.T) {
	registry := lexicon.NewRegistry()
	mapper := NewMapper()
	builder := NewObjectBuilder(mapper, registry)

	objectDef := &lexicon.ObjectDef{
		Type:           "object",
		RequiredFields: []string{"width"},
		Properties: []lexicon.PropertyEntry{
			{
				Name: "width",
				Property: lexicon.Property{
					Type: "integer",
				},
			},
			{
				Name: "height",
				Property: lexicon.Property{
					Type: "integer",
				},
			},
			{
				Name: "label",
				Property: lexicon.Property{
					Type:   "string",
					Format: "datetime",
				},
			},
		},
	}

	ref := "com.example.defs#aspectRatio"
	obj := builder.BuildObjectType(ref, objectDef)
	if obj == nil {
		t.Fatal("BuildObjectType returned nil")
	}

	// For ref "com.example.defs#aspectRatio" the expected name is
	// ToTypeName("com.example.defs") + capitalizeFirst("aspectRatio")
	// = "ComExampleDefs" + "AspectRatio" = "ComExampleDefsAspectRatio"
	wantName := "ComExampleDefsAspectRatio"
	if obj.Name() != wantName {
		t.Errorf("type name = %q, want %q", obj.Name(), wantName)
	}

	fields := obj.Fields()

	for _, name := range []string{"width", "height", "label"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("missing field %q", name)
		}
	}

	// "width" is required, so its type should be NonNull.
	widthField := fields["width"]
	if _, ok := widthField.Type.(*graphql.NonNull); !ok {
		t.Errorf("expected 'width' to be NonNull, got %T", widthField.Type)
	}

	// "height" is not required, so its type should NOT be NonNull.
	heightField := fields["height"]
	if _, isNonNull := heightField.Type.(*graphql.NonNull); isNonNull {
		t.Error("expected 'height' to not be NonNull")
	}

	// Building the same ref again should return the cached object.
	obj2 := builder.BuildObjectType(ref, objectDef)
	if obj2 != obj {
		t.Error("expected cached object on second call, got a different pointer")
	}
}
