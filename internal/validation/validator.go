package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/GainForest/hyperindex/internal/lexicon"
)

// Validator validates records with an in-memory lexicon registry and exact-byte
// hashes for the saved lexicon JSON documents that populated that registry.
type Validator struct {
	registry *lexicon.Registry
	mu       sync.RWMutex
	hashes   map[string]string
}

// NewValidator creates a local validator from a lexicon registry and a map of
// collection NSID to exact saved lexicon JSON hash. The hash map is copied so
// callers can safely mutate their source map after construction.
func NewValidator(registry *lexicon.Registry, hashes map[string]string) *Validator {
	copied := make(map[string]string, len(hashes))
	for collection, hash := range hashes {
		copied[collection] = hash
	}
	return &Validator{registry: registry, hashes: copied}
}

// NewValidatorFromLexiconBytes parses saved lexicon JSON bytes into a registry
// and hashes each document exactly as provided. Use this when the caller has the
// canonical saved bytes available from the lexicon repository or filesystem.
func NewValidatorFromLexiconBytes(saved map[string][]byte) (*Validator, error) {
	registry := lexicon.NewRegistry()
	hashes := make(map[string]string, len(saved))
	for collection, raw := range saved {
		parsed, err := lexicon.ParseBytes(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse saved lexicon for collection %s: %w", collection, err)
		}
		registry.Register(parsed)
		hashes[parsed.ID] = HashLexiconJSON(raw)
	}
	return NewValidator(registry, hashes), nil
}

// HashLexiconJSON returns the sha256 hash of the exact saved lexicon JSON bytes.
// It intentionally does not canonicalize JSON, so formatting-only changes are
// treated as a new schema version for validation refresh purposes.
func HashLexiconJSON(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// LexiconHash returns the exact-byte saved lexicon hash for a collection.
func (v *Validator) LexiconHash(collection string) (string, bool) {
	if v == nil {
		return "", false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	hash, ok := v.hashes[collection]
	return hash, ok
}

// SetLexiconHash records the current exact-byte saved lexicon hash for a
// collection after upload or registration.
func (v *Validator) SetLexiconHash(collection, hash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hashes[collection] = hash
}

// DeleteLexiconHash removes the saved lexicon hash for a collection after the
// lexicon is deleted.
func (v *Validator) DeleteLexiconHash(collection string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.hashes, collection)
}

// ValidateRecord validates raw record JSON against the saved lexicon for its
// collection and returns a status suitable for persistence on the record row.
func (v *Validator) ValidateRecord(collection, rkey string, rawJSON []byte) Result {
	if v == nil || v.registry == nil {
		return Result{Status: StatusValidationError, Error: "validator is not configured; initialize it with the saved lexicon registry"}
	}

	hash, ok := v.LexiconHash(collection)
	if !ok {
		return Result{Status: StatusUnknownSchema, Error: fmt.Sprintf("no saved lexicon for collection %s", collection)}
	}

	def, ok := v.registry.GetRecordDef(collection)
	if !ok || def == nil {
		return Result{Status: StatusValidationError, Error: fmt.Sprintf("saved lexicon for collection %s has no record definition", collection), LexiconHash: hash}
	}

	var record map[string]any
	if err := json.Unmarshal(rawJSON, &record); err != nil {
		return Result{Status: StatusValidationError, Error: fmt.Sprintf("failed to parse record JSON for collection %s: %v", collection, err), LexiconHash: hash}
	}
	if record == nil {
		return Result{Status: StatusInvalid, Error: "record JSON must be an object", LexiconHash: hash}
	}

	if err := v.validateRecordObject(collection, def, record, "record"); err != nil {
		return Result{Status: StatusInvalid, Error: err.Error(), LexiconHash: hash}
	}
	return Result{Status: StatusValid, LexiconHash: hash}
}

func (v *Validator) validateRecordObject(collection string, def *lexicon.RecordDef, value map[string]any, path string) error {
	for _, entry := range def.Properties {
		fieldValue, exists := value[entry.Name]
		if entry.Property.Required && !exists {
			return fmt.Errorf("missing required field: %s", fieldPath(path, entry.Name))
		}
		if exists {
			if err := v.validateProperty(collection, entry.Property, fieldValue, fieldPath(path, entry.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *Validator) validateObject(collection string, def *lexicon.ObjectDef, value map[string]any, path string) error {
	for _, name := range def.RequiredFields {
		if _, exists := value[name]; !exists {
			return fmt.Errorf("missing required field: %s", fieldPath(path, name))
		}
	}
	for _, entry := range def.Properties {
		fieldValue, exists := value[entry.Name]
		if !exists {
			continue
		}
		if err := v.validateProperty(collection, entry.Property, fieldValue, fieldPath(path, entry.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) validateProperty(collection string, prop lexicon.Property, value any, path string) error {
	if value == nil {
		return fmt.Errorf("field %s expected %s, got null", path, describeType(prop.Type))
	}

	switch prop.Type {
	case lexicon.TypeString:
		s, ok := value.(string)
		if !ok {
			return typeError(path, "string", value)
		}
		if prop.Const != "" && s != prop.Const {
			return fmt.Errorf("field %s expected constant %q, got %q", path, prop.Const, s)
		}
		if len(prop.Enum) > 0 && !contains(prop.Enum, s) {
			return fmt.Errorf("field %s expected one of %s, got %q", path, strings.Join(prop.Enum, ", "), s)
		}
		if prop.MinLength != nil && len(s) < *prop.MinLength {
			return fmt.Errorf("field %s expected length >= %d", path, *prop.MinLength)
		}
		if prop.MaxLength != nil && len(s) > *prop.MaxLength {
			return fmt.Errorf("field %s expected length <= %d", path, *prop.MaxLength)
		}
	case lexicon.TypeInteger:
		n, ok := value.(float64)
		if !ok || math.Trunc(n) != n {
			return typeError(path, "integer", value)
		}
		if prop.Minimum != nil && n < *prop.Minimum {
			return fmt.Errorf("field %s expected integer >= %v", path, *prop.Minimum)
		}
		if prop.Maximum != nil && n > *prop.Maximum {
			return fmt.Errorf("field %s expected integer <= %v", path, *prop.Maximum)
		}
	case lexicon.TypeBoolean:
		if _, ok := value.(bool); !ok {
			return typeError(path, "boolean", value)
		}
	case lexicon.TypeArray:
		items, ok := value.([]any)
		if !ok {
			return typeError(path, "array", value)
		}
		if prop.MaxLength != nil && len(items) > *prop.MaxLength {
			return fmt.Errorf("field %s expected array length <= %d", path, *prop.MaxLength)
		}
		if prop.Items != nil {
			for i, item := range items {
				if err := v.validateArrayItem(collection, *prop.Items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case lexicon.TypeRef:
		return v.validateRef(collection, prop.Ref, value, path)
	case lexicon.TypeUnion:
		return v.validateUnion(collection, prop.Refs, value, path)
	case lexicon.TypeObject:
		if _, ok := value.(map[string]any); !ok {
			return typeError(path, "object", value)
		}
	case lexicon.TypeBlob, lexicon.TypeCIDLink:
		if _, ok := value.(map[string]any); !ok {
			return typeError(path, "object", value)
		}
	case lexicon.TypeBytes:
		if _, ok := value.(string); !ok {
			return typeError(path, "string", value)
		}
	case "", lexicon.TypeUnknown:
		return nil
	default:
		return fmt.Errorf("field %s uses unsupported lexicon type %q; update the local validator", path, prop.Type)
	}
	return nil
}

func (v *Validator) validateArrayItem(collection string, item lexicon.ArrayItems, value any, path string) error {
	prop := lexicon.Property{Type: item.Type, Ref: item.Ref, Refs: item.Refs}
	return v.validateProperty(collection, prop, value, path)
}

func (v *Validator) validateRef(collection, ref string, value any, path string) error {
	obj, ok := value.(map[string]any)
	if !ok {
		return typeError(path, "object", value)
	}
	if ref == "" {
		return fmt.Errorf("field %s has ref type without a ref target in saved lexicon", path)
	}
	resolved, ok := v.registry.ResolveRef(ref, collection)
	if !ok {
		return fmt.Errorf("field %s references unknown saved lexicon type %s", path, lexicon.ResolveLocalRef(ref, collection))
	}
	return v.validateResolvedRef(collection, resolved, obj, path)
}

func (v *Validator) validateUnion(collection string, refs []string, value any, path string) error {
	obj, ok := value.(map[string]any)
	if !ok {
		return typeError(path, "object", value)
	}
	typeValue, ok := obj["$type"].(string)
	if !ok || typeValue == "" {
		return fmt.Errorf("field %s union object missing required $type", path)
	}
	for _, ref := range refs {
		resolvedRef := lexicon.ResolveLocalRef(ref, collection)
		if typeValue != resolvedRef {
			continue
		}
		resolved, ok := v.registry.ResolveRef(ref, collection)
		if !ok {
			return fmt.Errorf("field %s references unknown saved lexicon type %s", path, resolvedRef)
		}
		return v.validateResolvedRef(collection, resolved, obj, path)
	}
	return fmt.Errorf("field %s union type %q is not one of %s", path, typeValue, strings.Join(resolveRefs(collection, refs), ", "))
}

func (v *Validator) validateResolvedRef(collection string, resolved any, obj map[string]any, path string) error {
	switch def := resolved.(type) {
	case *lexicon.ObjectDef:
		return v.validateObject(collection, def, obj, path)
	case *lexicon.RecordDef:
		return v.validateRecordObject(collection, def, obj, path)
	default:
		return fmt.Errorf("field %s resolved to unsupported lexicon definition", path)
	}
}

func typeError(path, expected string, value any) error {
	return fmt.Errorf("field %s expected %s, got %s", path, expected, jsonType(value))
}

func jsonType(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func describeType(t string) string {
	if t == "" {
		return "value"
	}
	return t
}

func fieldPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func resolveRefs(collection string, refs []string) []string {
	resolved := make([]string, len(refs))
	for i, ref := range refs {
		resolved[i] = lexicon.ResolveLocalRef(ref, collection)
	}
	return resolved
}
