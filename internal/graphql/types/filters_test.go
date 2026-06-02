package types //nolint:revive // package name is descriptive within graphql context

import (
	"testing"

	"github.com/graphql-go/graphql"
)

// TestFilterInputTypes verifies that each shared filter InputObject type has
// the expected operator fields.
func TestFilterInputTypes(t *testing.T) {
	tests := []struct {
		name       string
		inputObj   *graphql.InputObject
		wantFields []string
		wantAbsent []string
	}{
		{
			name:       "StringFilterInput fields",
			inputObj:   StringFilterInput,
			wantFields: []string{"eq", "neq", "in", "contains", "startsWith", "isNull"},
		},
		{
			name:       "IntFilterInput fields",
			inputObj:   IntFilterInput,
			wantFields: []string{"eq", "neq", "gt", "lt", "gte", "lte", "in", "isNull"},
			wantAbsent: []string{"contains", "startsWith"},
		},
		{
			name:       "FloatFilterInput fields",
			inputObj:   FloatFilterInput,
			wantFields: []string{"eq", "neq", "gt", "lt", "gte", "lte", "isNull"},
			wantAbsent: []string{"in", "contains", "startsWith"},
		},
		{
			name:       "BooleanFilterInput fields",
			inputObj:   BooleanFilterInput,
			wantFields: []string{"eq", "isNull"},
			wantAbsent: []string{"neq", "gt", "lt", "gte", "lte", "in", "contains", "startsWith"},
		},
		{
			name:       "DateTimeFilterInput fields",
			inputObj:   DateTimeFilterInput,
			wantFields: []string{"eq", "neq", "gt", "lt", "gte", "lte", "isNull"},
			wantAbsent: []string{"in", "contains", "startsWith"},
		},
		{
			name:       "PresenceFilterInput fields",
			inputObj:   PresenceFilterInput,
			wantFields: []string{"isNull"},
			wantAbsent: []string{"eq", "neq", "gt", "lt", "gte", "lte", "in", "contains", "startsWith"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.inputObj == nil {
				t.Fatal("input object is nil")
			}

			fields := tt.inputObj.Fields()

			for _, fieldName := range tt.wantFields {
				if _, ok := fields[fieldName]; !ok {
					t.Errorf("expected field %q to be present in %s", fieldName, tt.inputObj.Name())
				}
			}

			for _, fieldName := range tt.wantAbsent {
				if _, ok := fields[fieldName]; ok {
					t.Errorf("expected field %q to be absent from %s", fieldName, tt.inputObj.Name())
				}
			}
		})
	}
}

// TestStringFilterInput_FieldTypes verifies the GraphQL types of StringFilterInput fields.
func TestStringFilterInput_FieldTypes(t *testing.T) {
	fields := StringFilterInput.Fields()

	// eq, neq, contains, startsWith should be String
	for _, name := range []string{"eq", "neq", "contains", "startsWith"} {
		f, ok := fields[name]
		if !ok {
			t.Fatalf("missing field %q", name)
		}
		if f.Type != graphql.String {
			t.Errorf("field %q type = %v, want String", name, f.Type)
		}
	}

	// in should be [String!]
	inField, ok := fields["in"]
	if !ok {
		t.Fatal("missing field 'in'")
	}
	list, ok := inField.Type.(*graphql.List)
	if !ok {
		t.Fatalf("field 'in' type = %T, want *graphql.List", inField.Type)
	}
	if _, ok := list.OfType.(*graphql.NonNull); !ok {
		t.Errorf("field 'in' list element type = %T, want *graphql.NonNull", list.OfType)
	}

	// isNull should be Boolean
	isNullField, ok := fields["isNull"]
	if !ok {
		t.Fatal("missing field 'isNull'")
	}
	if isNullField.Type != graphql.Boolean {
		t.Errorf("field 'isNull' type = %v, want Boolean", isNullField.Type)
	}
}

// TestIntFilterInput_FieldTypes verifies the GraphQL types of IntFilterInput fields.
func TestIntFilterInput_FieldTypes(t *testing.T) {
	fields := IntFilterInput.Fields()

	// eq, neq, gt, lt, gte, lte should be Int
	for _, name := range []string{"eq", "neq", "gt", "lt", "gte", "lte"} {
		f, ok := fields[name]
		if !ok {
			t.Fatalf("missing field %q", name)
		}
		if f.Type != graphql.Int {
			t.Errorf("field %q type = %v, want Int", name, f.Type)
		}
	}

	// in should be [Int!]
	inField, ok := fields["in"]
	if !ok {
		t.Fatal("missing field 'in'")
	}
	list, ok := inField.Type.(*graphql.List)
	if !ok {
		t.Fatalf("field 'in' type = %T, want *graphql.List", inField.Type)
	}
	if _, ok := list.OfType.(*graphql.NonNull); !ok {
		t.Errorf("field 'in' list element type = %T, want *graphql.NonNull", list.OfType)
	}
}

// TestFloatFilterInput_FieldTypes verifies the GraphQL types of FloatFilterInput fields.
func TestFloatFilterInput_FieldTypes(t *testing.T) {
	fields := FloatFilterInput.Fields()

	for _, name := range []string{"eq", "neq", "gt", "lt", "gte", "lte"} {
		f, ok := fields[name]
		if !ok {
			t.Fatalf("missing field %q", name)
		}
		if f.Type != graphql.Float {
			t.Errorf("field %q type = %v, want Float", name, f.Type)
		}
	}
}

// TestDateTimeFilterInput_FieldTypes verifies that DateTimeFilterInput uses DateTimeScalar.
func TestDateTimeFilterInput_FieldTypes(t *testing.T) {
	fields := DateTimeFilterInput.Fields()

	for _, name := range []string{"eq", "neq", "gt", "lt", "gte", "lte"} {
		f, ok := fields[name]
		if !ok {
			t.Fatalf("missing field %q", name)
		}
		if f.Type != DateTimeScalar {
			t.Errorf("field %q type = %v, want DateTimeScalar", name, f.Type)
		}
	}
}

func TestPresenceFilterInput_FieldTypes(t *testing.T) {
	fields := PresenceFilterInput.Fields()

	if len(fields) != 1 {
		t.Fatalf("PresenceFilterInput fields length = %d, want 1", len(fields))
	}

	isNullField, ok := fields["isNull"]
	if !ok {
		t.Fatal("PresenceFilterInput: missing field 'isNull'")
	}

	nonNull, ok := isNullField.Type.(*graphql.NonNull)
	if !ok {
		t.Fatalf("PresenceFilterInput: isNull type = %T, want *graphql.NonNull", isNullField.Type)
	}
	if nonNull.OfType != graphql.Boolean {
		t.Errorf("PresenceFilterInput: isNull inner type = %v, want Boolean", nonNull.OfType)
	}
}

// TestFilterInputForLexiconType verifies the scalar-only mapping from lexicon types to filter inputs.
func TestFilterInputForLexiconType(t *testing.T) {
	tests := []struct {
		name        string
		lexiconType string
		format      string
		wantInput   *graphql.InputObject
		wantNil     bool
	}{
		// Filterable types
		{name: "string no format", lexiconType: "string", format: "", wantInput: StringFilterInput},
		{name: "string uri format", lexiconType: "string", format: "uri", wantInput: StringFilterInput},
		{name: "string handle format", lexiconType: "string", format: "handle", wantInput: StringFilterInput},
		{name: "string datetime format", lexiconType: "string", format: "datetime", wantInput: DateTimeFilterInput},
		{name: "integer", lexiconType: "integer", format: "", wantInput: IntFilterInput},
		{name: "number", lexiconType: "number", format: "", wantInput: FloatFilterInput},
		{name: "boolean", lexiconType: "boolean", format: "", wantInput: BooleanFilterInput},

		// Non-filterable types — must return nil
		{name: "blob", lexiconType: "blob", format: "", wantNil: true},
		{name: "bytes", lexiconType: "bytes", format: "", wantNil: true},
		{name: "unknown", lexiconType: "unknown", format: "", wantNil: true},
		{name: "ref", lexiconType: "ref", format: "", wantNil: true},
		{name: "union", lexiconType: "union", format: "", wantNil: true},
		{name: "array", lexiconType: "array", format: "", wantNil: true},
		{name: "object", lexiconType: "object", format: "", wantNil: true},
		{name: "cid-link", lexiconType: "cid-link", format: "", wantNil: true},
		{name: "record", lexiconType: "record", format: "", wantNil: true},
		{name: "empty type", lexiconType: "", format: "", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterInputForLexiconType(tt.lexiconType, tt.format)

			if tt.wantNil {
				if got != nil {
					t.Errorf("FilterInputForLexiconType(%q, %q) = %v, want nil",
						tt.lexiconType, tt.format, got.Name())
				}
				return
			}

			if got == nil {
				t.Fatalf("FilterInputForLexiconType(%q, %q) = nil, want %v",
					tt.lexiconType, tt.format, tt.wantInput.Name())
			}

			if got != tt.wantInput {
				t.Errorf("FilterInputForLexiconType(%q, %q) = %v, want %v",
					tt.lexiconType, tt.format, got.Name(), tt.wantInput.Name())
			}
		})
	}
}

func TestFilterInputForLexiconProperty(t *testing.T) {
	tests := []struct {
		name        string
		lexiconType string
		format      string
		wantInput   *graphql.InputObject
		wantNil     bool
	}{
		{name: "string no format", lexiconType: "string", format: "", wantInput: StringFilterInput},
		{name: "string datetime format", lexiconType: "string", format: "datetime", wantInput: DateTimeFilterInput},
		{name: "integer", lexiconType: "integer", format: "", wantInput: IntFilterInput},
		{name: "number", lexiconType: "number", format: "", wantInput: FloatFilterInput},
		{name: "boolean", lexiconType: "boolean", format: "", wantInput: BooleanFilterInput},
		{name: "array", lexiconType: "array", format: "", wantInput: PresenceFilterInput},
		{name: "ref", lexiconType: "ref", format: "", wantInput: PresenceFilterInput},
		{name: "union", lexiconType: "union", format: "", wantInput: PresenceFilterInput},
		{name: "object", lexiconType: "object", format: "", wantInput: PresenceFilterInput},
		{name: "blob", lexiconType: "blob", format: "", wantInput: PresenceFilterInput},
		{name: "bytes", lexiconType: "bytes", format: "", wantInput: PresenceFilterInput},
		{name: "unknown", lexiconType: "unknown", format: "", wantInput: PresenceFilterInput},
		{name: "cid-link", lexiconType: "cid-link", format: "", wantInput: PresenceFilterInput},
		{name: "record", lexiconType: "record", format: "", wantNil: true},
		{name: "empty type", lexiconType: "", format: "", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterInputForLexiconProperty(tt.lexiconType, tt.format)

			if tt.wantNil {
				if got != nil {
					t.Errorf("FilterInputForLexiconProperty(%q, %q) = %v, want nil",
						tt.lexiconType, tt.format, got.Name())
				}
				return
			}

			if got == nil {
				t.Fatalf("FilterInputForLexiconProperty(%q, %q) = nil, want %v",
					tt.lexiconType, tt.format, tt.wantInput.Name())
			}

			if got != tt.wantInput {
				t.Errorf("FilterInputForLexiconProperty(%q, %q) = %v, want %v",
					tt.lexiconType, tt.format, got.Name(), tt.wantInput.Name())
			}
		})
	}
}

// TestFilterInputNames verifies the Name() of each filter input type.
func TestFilterInputNames(t *testing.T) {
	tests := []struct {
		inputObj *graphql.InputObject
		wantName string
	}{
		{StringFilterInput, "StringFilterInput"},
		{IntFilterInput, "IntFilterInput"},
		{FloatFilterInput, "FloatFilterInput"},
		{BooleanFilterInput, "BooleanFilterInput"},
		{DateTimeFilterInput, "DateTimeFilterInput"},
		{DIDFilterInput, "DIDFilterInput"},
		{PresenceFilterInput, "PresenceFilterInput"},
	}

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			if tt.inputObj.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", tt.inputObj.Name(), tt.wantName)
			}
		})
	}
}

// TestDIDFilterInput verifies that DIDFilterInput has exactly eq and in fields,
// and that unsupported operators (contains, startsWith, neq, isNull, gt, lt) are absent.
func TestDIDFilterInput(t *testing.T) {
	if DIDFilterInput == nil {
		t.Fatal("DIDFilterInput is nil")
	}

	fields := DIDFilterInput.Fields()

	// Must have eq and in
	wantPresent := []string{"eq", "in"}
	for _, name := range wantPresent {
		if _, ok := fields[name]; !ok {
			t.Errorf("DIDFilterInput: expected field %q to be present", name)
		}
	}

	// Must NOT have any other operators
	wantAbsent := []string{"neq", "contains", "startsWith", "isNull", "gt", "lt", "gte", "lte"}
	for _, name := range wantAbsent {
		if _, ok := fields[name]; ok {
			t.Errorf("DIDFilterInput: expected field %q to be absent", name)
		}
	}
}

// TestDIDFilterInput_FieldTypes verifies the GraphQL types of DIDFilterInput fields.
func TestDIDFilterInput_FieldTypes(t *testing.T) {
	fields := DIDFilterInput.Fields()

	// eq should be String
	eqField, ok := fields["eq"]
	if !ok {
		t.Fatal("DIDFilterInput: missing field 'eq'")
	}
	if eqField.Type != graphql.String {
		t.Errorf("DIDFilterInput: field 'eq' type = %v, want String", eqField.Type)
	}

	// in should be [String!]
	inField, ok := fields["in"]
	if !ok {
		t.Fatal("DIDFilterInput: missing field 'in'")
	}
	list, ok := inField.Type.(*graphql.List)
	if !ok {
		t.Fatalf("DIDFilterInput: field 'in' type = %T, want *graphql.List", inField.Type)
	}
	if _, ok := list.OfType.(*graphql.NonNull); !ok {
		t.Errorf("DIDFilterInput: field 'in' list element type = %T, want *graphql.NonNull", list.OfType)
	}
}
