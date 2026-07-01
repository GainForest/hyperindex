package lexicon

import (
	"encoding/json"
	"fmt"
)

// Parse parses a lexicon JSON string into a Lexicon struct.
func Parse(jsonStr string) (*Lexicon, error) {
	return ParseBytes([]byte(jsonStr))
}

// ParseBytes parses lexicon JSON bytes into a Lexicon struct.
func ParseBytes(data []byte) (*Lexicon, error) {
	var raw rawLexicon
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse lexicon JSON: %w", err)
	}

	return convertRawLexicon(&raw)
}

// rawLexicon is the raw JSON structure for parsing.
type rawLexicon struct {
	Lexicon int                        `json:"lexicon"`
	ID      string                     `json:"id"`
	Defs    map[string]json.RawMessage `json:"defs"`
}

// rawRecordDef is used to parse record-type definitions.
type rawRecordDef struct {
	Type   string          `json:"type"`
	Key    string          `json:"key,omitempty"`
	Record json.RawMessage `json:"record,omitempty"`
}

// rawObjectDef is used to parse object-type definitions.
type rawObjectDef struct {
	Type       string                     `json:"type"`
	Required   []string                   `json:"required,omitempty"`
	Properties map[string]json.RawMessage `json:"properties,omitempty"`
}

// rawProperty is used to parse property definitions.
type rawProperty struct {
	Type        string                     `json:"type"`
	Format      string                     `json:"format,omitempty"`
	Ref         string                     `json:"ref,omitempty"`
	Refs        []string                   `json:"refs,omitempty"`
	Items       json.RawMessage            `json:"items,omitempty"`
	Required    []string                   `json:"required,omitempty"`
	Properties  map[string]json.RawMessage `json:"properties,omitempty"`
	Description string                     `json:"description,omitempty"`
	Default     any                        `json:"default,omitempty"`
	Minimum     *float64                   `json:"minimum,omitempty"`
	Maximum     *float64                   `json:"maximum,omitempty"`
	MinLength   *int                       `json:"minLength,omitempty"`
	MaxLength   *int                       `json:"maxLength,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
	Const       string                     `json:"const,omitempty"`
	KnownValues []string                   `json:"knownValues,omitempty"`
}

// rawArrayItems is used to parse array item definitions.
type rawArrayItems struct {
	Type string   `json:"type"`
	Ref  string   `json:"ref,omitempty"`
	Refs []string `json:"refs,omitempty"`
}

func convertRawLexicon(raw *rawLexicon) (*Lexicon, error) {
	if raw.ID == "" {
		return nil, fmt.Errorf("lexicon missing required 'id' field")
	}

	lexicon := &Lexicon{
		ID: raw.ID,
		Defs: Defs{
			Others: make(map[string]Def),
		},
	}

	// Process each definition
	for name, defData := range raw.Defs {
		if name == "main" {
			// Parse main definition
			mainDef, err := parseMainDef(defData)
			if err != nil {
				return nil, fmt.Errorf("failed to parse main definition: %w", err)
			}
			lexicon.Defs.Main = mainDef
		} else {
			// Parse other definitions
			def, err := parseDef(defData)
			if err != nil {
				return nil, fmt.Errorf("failed to parse definition '%s': %w", name, err)
			}
			lexicon.Defs.Others[name] = *def
		}
	}

	return lexicon, nil
}

// parseMainDef parses the main definition which can be either a record or object type.
func parseMainDef(data json.RawMessage) (*RecordDef, error) {
	// First, determine the type
	var typeCheck struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return nil, err
	}

	switch typeCheck.Type {
	case "record":
		return parseRecordDef(data)
	case "object":
		// Object-type main definitions are treated as RecordDefs without a key
		return parseObjectAsRecordDef(data)
	default:
		return nil, fmt.Errorf("unsupported main definition type: %s", typeCheck.Type)
	}
}

// parseRecordDef parses a record-type definition.
func parseRecordDef(data json.RawMessage) (*RecordDef, error) {
	var raw rawRecordDef
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	recordDef := &RecordDef{
		Type: raw.Type,
		Key:  raw.Key,
	}

	// Parse the inner record object
	if len(raw.Record) > 0 {
		var innerObj rawObjectDef
		if err := json.Unmarshal(raw.Record, &innerObj); err != nil {
			return nil, fmt.Errorf("failed to parse record object: %w", err)
		}

		props, err := parseProperties(innerObj.Properties, innerObj.Required)
		if err != nil {
			return nil, err
		}
		recordDef.Properties = props
	}

	return recordDef, nil
}

// parseObjectAsRecordDef parses an object-type definition as a RecordDef.
// This is used for main definitions that are objects (not wrapped in "record").
func parseObjectAsRecordDef(data json.RawMessage) (*RecordDef, error) {
	var raw rawObjectDef
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	props, err := parseProperties(raw.Properties, raw.Required)
	if err != nil {
		return nil, err
	}

	return &RecordDef{
		Type:       raw.Type,
		Properties: props,
	}, nil
}

// parseDef parses a non-main definition (can be record or object).
func parseDef(data json.RawMessage) (*Def, error) {
	// First, determine the type
	var typeCheck struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return nil, err
	}

	switch typeCheck.Type {
	case "record":
		recordDef, err := parseRecordDef(data)
		if err != nil {
			return nil, err
		}
		return &Def{
			Type:   "record",
			Record: recordDef,
		}, nil

	case "object":
		objDef, err := parseObjectDef(data)
		if err != nil {
			return nil, err
		}
		return &Def{
			Type:   "object",
			Object: objDef,
		}, nil

	default:
		// Treat unknown types as objects for now
		objDef, err := parseObjectDef(data)
		if err != nil {
			return nil, err
		}
		return &Def{
			Type:   typeCheck.Type,
			Object: objDef,
		}, nil
	}
}

// parseObjectDef parses an object definition.
func parseObjectDef(data json.RawMessage) (*ObjectDef, error) {
	var raw rawObjectDef
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	props, err := parseProperties(raw.Properties, raw.Required)
	if err != nil {
		return nil, err
	}

	return &ObjectDef{
		Type:           raw.Type,
		RequiredFields: raw.Required,
		Properties:     props,
	}, nil
}

// parseProperties parses a map of property definitions.
func parseProperties(propsMap map[string]json.RawMessage, required []string) ([]PropertyEntry, error) {
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r] = true
	}

	var props []PropertyEntry
	for name, propData := range propsMap {
		prop, err := parseProperty(propData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse property '%s': %w", name, err)
		}
		prop.Required = requiredSet[name]
		props = append(props, PropertyEntry{
			Name:     name,
			Property: *prop,
		})
	}

	return props, nil
}

// parseProperty parses a single property definition.
func parseProperty(data json.RawMessage) (*Property, error) {
	var raw rawProperty
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	prop := &Property{
		Type:        raw.Type,
		Format:      raw.Format,
		Ref:         raw.Ref,
		Refs:        raw.Refs,
		Description: raw.Description,
		Default:     raw.Default,
		Minimum:     raw.Minimum,
		Maximum:     raw.Maximum,
		MinLength:   raw.MinLength,
		MaxLength:   raw.MaxLength,
		Enum:        raw.Enum,
		Const:       raw.Const,
		KnownValues: raw.KnownValues,
	}

	// Parse items if present (for arrays)
	if len(raw.Items) > 0 {
		items, err := parseArrayItems(raw.Items)
		if err != nil {
			return nil, fmt.Errorf("failed to parse items: %w", err)
		}
		prop.Items = items
	}

	if raw.Type == TypeObject && len(raw.Properties) > 0 {
		props, err := parseProperties(raw.Properties, raw.Required)
		if err != nil {
			return nil, err
		}
		prop.InlineObject = &ObjectDef{Type: TypeObject, RequiredFields: raw.Required, Properties: props}
	}

	return prop, nil
}

// parseArrayItems parses array item definitions.
func parseArrayItems(data json.RawMessage) (*ArrayItems, error) {
	var raw rawArrayItems
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return &ArrayItems{
		Type: raw.Type,
		Ref:  raw.Ref,
		Refs: raw.Refs,
	}, nil
}
