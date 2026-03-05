package lexicon

import (
	"reflect"
	"testing"
)

func TestZeroValueForType(t *testing.T) {
	tests := []struct {
		name     string
		propType string
		want     interface{}
	}{
		{name: "string no format", propType: "string", want: ""},
		{name: "string datetime format", propType: "string", want: ""},
		{name: "integer", propType: "integer", want: 0},
		{name: "boolean", propType: "boolean", want: false},
		{name: "array", propType: "array", want: []interface{}{}},
		{name: "ref", propType: "ref", want: nil},
		{name: "union", propType: "union", want: nil},
		{name: "blob", propType: "blob", want: nil},
		{name: "bytes", propType: "bytes", want: nil},
		{name: "cid-link", propType: "cid-link", want: nil},
		{name: "unknown", propType: "unknown", want: nil},
		{name: "object", propType: "object", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ZeroValueForType(tt.propType)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ZeroValueForType(%q) = %v (%T), want %v (%T)",
					tt.propType, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestRecordDefRequiredProperties(t *testing.T) {
	rec := &RecordDef{
		Type: "record",
		Properties: []PropertyEntry{
			{Name: "title", Property: Property{Type: TypeString, Required: true}},
			{Name: "description", Property: Property{Type: TypeString, Required: false}},
			{Name: "count", Property: Property{Type: TypeInteger, Required: true}},
			{Name: "tags", Property: Property{Type: TypeArray, Required: false}},
		},
	}

	required := rec.RequiredProperties()

	if len(required) != 2 {
		t.Fatalf("expected 2 required properties, got %d", len(required))
	}

	names := make([]string, len(required))
	for i, e := range required {
		names[i] = e.Name
	}

	if names[0] != "title" {
		t.Errorf("expected first required property to be 'title', got %q", names[0])
	}
	if names[1] != "count" {
		t.Errorf("expected second required property to be 'count', got %q", names[1])
	}

	// Verify non-required properties are excluded
	for _, e := range required {
		if !e.Property.Required {
			t.Errorf("property %q is not required but was returned by RequiredProperties()", e.Name)
		}
	}
}

func TestRecordDefRequiredPropertiesEmpty(t *testing.T) {
	rec := &RecordDef{
		Type: "record",
		Properties: []PropertyEntry{
			{Name: "optional", Property: Property{Type: TypeString, Required: false}},
		},
	}

	required := rec.RequiredProperties()
	if len(required) != 0 {
		t.Errorf("expected 0 required properties, got %d", len(required))
	}
}
