// Package lexicon provides types and utilities for working with AT Protocol lexicons.
//
// Lexicons are JSON schema documents that define record types, object types,
// and other definitions used in the AT Protocol ecosystem.
package lexicon

// Lexicon represents a parsed AT Protocol lexicon document.
type Lexicon struct {
	// ID is the NSID (e.g., "app.bsky.feed.post")
	ID string `json:"id"`

	// Defs contains all definitions in this lexicon
	Defs Defs `json:"defs"`
}

// Defs holds the definitions within a lexicon.
type Defs struct {
	// Main is the primary definition (optional - some lexicons only have others)
	Main *RecordDef `json:"main,omitempty"`

	// Others contains additional named definitions (e.g., "aspectRatio", "image")
	Others map[string]Def `json:"others,omitempty"`
}

// Def represents a definition which can be either a record or an object.
type Def struct {
	// Type indicates whether this is a "record" or "object" definition
	Type string

	// Record contains the record definition if Type == "record"
	Record *RecordDef

	// Object contains the object definition if Type == "object"
	Object *ObjectDef
}

// IsRecord returns true if this definition is a record type.
func (d *Def) IsRecord() bool {
	return d.Type == "record" && d.Record != nil
}

// IsObject returns true if this definition is an object type.
func (d *Def) IsObject() bool {
	return d.Type == "object" && d.Object != nil
}

// RecordDef defines a record type (a collection/record in AT Protocol).
type RecordDef struct {
	// Type is typically "record" or "object"
	Type string `json:"type"`

	// Key is the record key pattern (optional, e.g., "tid")
	Key string `json:"key,omitempty"`

	// Properties are the fields of this record
	Properties []PropertyEntry `json:"properties"`
}

// ObjectDef defines an object type (a nested structure within a record).
type ObjectDef struct {
	// Type is typically "object"
	Type string `json:"type"`

	// RequiredFields lists the names of required properties
	RequiredFields []string `json:"required,omitempty"`

	// Properties are the fields of this object
	Properties []PropertyEntry `json:"properties"`
}

// PropertyEntry is a named property with its definition.
type PropertyEntry struct {
	Name     string
	Property Property
}

// Property defines a single property/field.
type Property struct {
	// Type is the property type: "string", "integer", "boolean", "array", "ref", "union", "blob", "bytes", "cid-link", "unknown"
	Type string `json:"type"`

	// Required indicates if this property is required
	Required bool `json:"required,omitempty"`

	// Format is an optional format hint (e.g., "datetime", "uri", "did", "at-uri", "handle")
	Format string `json:"format,omitempty"`

	// Ref is a reference to another type (e.g., "app.bsky.embed.images#image")
	Ref string `json:"ref,omitempty"`

	// Refs is a list of references for union types
	Refs []string `json:"refs,omitempty"`

	// Items defines the type of array elements (for array properties)
	Items *ArrayItems `json:"items,omitempty"`

	// Description is an optional description of the property
	Description string `json:"description,omitempty"`

	// Default is the default value (if any)
	Default any `json:"default,omitempty"`

	// Minimum value (for integer/number types)
	Minimum *float64 `json:"minimum,omitempty"`

	// Maximum value (for integer/number types)
	Maximum *float64 `json:"maximum,omitempty"`

	// MinLength for strings
	MinLength *int `json:"minLength,omitempty"`

	// MaxLength for strings or arrays
	MaxLength *int `json:"maxLength,omitempty"`

	// Enum lists allowed values
	Enum []string `json:"enum,omitempty"`

	// Const is a constant required value
	Const string `json:"const,omitempty"`

	// KnownValues lists known values (for open enums)
	KnownValues []string `json:"knownValues,omitempty"`

	// InlineObject carries the nested schema for properties declared directly as
	// type object instead of by reference. It is nil for non-object properties and
	// for object properties without an inline schema.
	InlineObject *ObjectDef `json:"-"`
}

// ArrayItems defines the type of items in an array property.
type ArrayItems struct {
	// Type is the item type: "string", "integer", "ref", "union", etc.
	Type string `json:"type"`

	// Ref is a reference to another type (for ref items)
	Ref string `json:"ref,omitempty"`

	// Refs is a list of references (for union items)
	Refs []string `json:"refs,omitempty"`
}

// PropertyType constants for common property types.
const (
	TypeString  = "string"
	TypeInteger = "integer"
	TypeBoolean = "boolean"
	TypeArray   = "array"
	TypeRef     = "ref"
	TypeUnion   = "union"
	TypeBlob    = "blob"
	TypeBytes   = "bytes"
	TypeCIDLink = "cid-link"
	TypeUnknown = "unknown"
	TypeObject  = "object"
	TypeRecord  = "record"
)

// Format constants for common formats.
const (
	FormatDatetime  = "datetime"
	FormatURI       = "uri"
	FormatATURI     = "at-uri"
	FormatDID       = "did"
	FormatHandle    = "handle"
	FormatCID       = "cid"
	FormatNSID      = "nsid"
	FormatLanguage  = "language"
	FormatRecordKey = "record-key"
	FormatTID       = "tid"
)

// GetProperty returns the property with the given name, or nil if not found.
func (r *RecordDef) GetProperty(name string) *Property {
	for i := range r.Properties {
		if r.Properties[i].Name == name {
			return &r.Properties[i].Property
		}
	}
	return nil
}

// GetProperty returns the property with the given name, or nil if not found.
func (o *ObjectDef) GetProperty(name string) *Property {
	for i := range o.Properties {
		if o.Properties[i].Name == name {
			return &o.Properties[i].Property
		}
	}
	return nil
}

// IsRequired returns true if the named property is required.
func (o *ObjectDef) IsRequired(name string) bool {
	for _, req := range o.RequiredFields {
		if req == name {
			return true
		}
	}
	return false
}

// HasRef returns true if this property references another type.
func (p *Property) HasRef() bool {
	return p.Ref != "" || len(p.Refs) > 0
}

// IsUnion returns true if this property is a union type.
func (p *Property) IsUnion() bool {
	return p.Type == TypeUnion || len(p.Refs) > 0
}

// IsArray returns true if this property is an array type.
func (p *Property) IsArray() bool {
	return p.Type == TypeArray
}

// GetRefs returns all refs for this property (either single ref or union refs).
func (p *Property) GetRefs() []string {
	if len(p.Refs) > 0 {
		return p.Refs
	}
	if p.Ref != "" {
		return []string{p.Ref}
	}
	return nil
}

// ZeroValueForType returns the appropriate Go zero value for a given lexicon property type.
// For complex types (ref, union, blob, bytes, cid-link, unknown, object) it returns nil,
// as these cannot have a meaningful scalar zero value.
func ZeroValueForType(propType string) interface{} {
	switch propType {
	case TypeString:
		return ""
	case TypeInteger:
		return 0
	case TypeBoolean:
		return false
	case TypeArray:
		return []interface{}{}
	default:
		// ref, union, blob, bytes, cid-link, unknown, object — return nil
		// These are complex types that cannot have a meaningful zero value
		return nil
	}
}

// RequiredProperties returns all properties that are marked as required.
func (r *RecordDef) RequiredProperties() []PropertyEntry {
	var result []PropertyEntry
	for _, entry := range r.Properties {
		if entry.Property.Required {
			result = append(result, entry)
		}
	}
	return result
}
