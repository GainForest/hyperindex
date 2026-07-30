package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bluesky-social/indigo/atproto/atdata"
	indigolexicon "github.com/bluesky-social/indigo/atproto/lexicon"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"github.com/GainForest/hyperindex/internal/lexicon"
)

// Validator validates records against the fixed set of saved Lexicons loaded at
// startup. Indigo handles AT Protocol data and Lexicon conformance; the local
// registry is retained for record-key rules and transitive hash calculation.
type Validator struct {
	registry       *lexicon.Registry
	catalog        *indigolexicon.BaseCatalog
	hashes         map[string]string
	referenceGraph map[string][]string
}

// NewValidatorFromLexiconBytes builds a validator from the exact saved Lexicon
// JSON selected at startup. The map key must match each document's Lexicon NSID.
func NewValidatorFromLexiconBytes(saved map[string][]byte) (*Validator, error) {
	registry := lexicon.NewRegistry()
	catalog := indigolexicon.NewBaseCatalog()
	hashes := make(map[string]string, len(saved))

	ids := make([]string, 0, len(saved))
	for id := range saved {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		raw := saved[id]
		parsed, schemaFile, err := parseLexiconBytes(id, raw)
		if err != nil {
			return nil, err
		}
		if err := addSchemaFileToIndigoCatalog(&catalog, schemaFile); err != nil {
			return nil, fmt.Errorf("saved lexicon %s is not valid according to Indigo: %w", id, err)
		}
		registry.Register(parsed)
		hashes[id] = HashLexiconJSON(raw)
	}
	referenceGraph, err := buildReferenceGraph(saved)
	if err != nil {
		return nil, err
	}
	if err := validateReferenceClosure(referenceGraph); err != nil {
		return nil, fmt.Errorf("startup Lexicon set has unresolved references: %w", err)
	}

	return &Validator{
		registry:       registry,
		catalog:        &catalog,
		hashes:         hashes,
		referenceGraph: referenceGraph,
	}, nil
}

// CheckLexiconSet verifies a complete prospective startup Lexicon set,
// including schema validity and local reference closure.
func CheckLexiconSet(saved map[string][]byte) error {
	_, err := NewValidatorFromLexiconBytes(saved)
	return err
}

// CheckLexiconBytes verifies one Lexicon document structurally. Call
// CheckLexiconSet before persistence when the document may reference other
// saved Lexicons.
func CheckLexiconBytes(expectedID string, raw []byte) error {
	_, schemaFile, err := parseLexiconBytes(expectedID, raw)
	if err != nil {
		return err
	}
	catalog := indigolexicon.NewBaseCatalog()
	if err := addSchemaFileToIndigoCatalog(&catalog, schemaFile); err != nil {
		return fmt.Errorf("lexicon %s is not valid according to Indigo: %w", expectedID, err)
	}
	return nil
}

// addSchemaFileToIndigoCatalog adapts the ATProto record-key rule to the
// pinned Indigo version, which validates record bodies correctly but only
// accepts the equivalent "any" record key spelling. Hyperindex keeps the
// original rule in its registry and validates the actual rkey separately.
func addSchemaFileToIndigoCatalog(catalog *indigolexicon.BaseCatalog, schemaFile indigolexicon.SchemaFile) error {
	for name, def := range schemaFile.Defs {
		record, ok := def.Inner.(indigolexicon.SchemaRecord)
		if !ok || record.Key != "record-key" {
			continue
		}
		record.Key = "any"
		def.Inner = record
		schemaFile.Defs[name] = def
	}
	return catalog.AddSchemaFile(schemaFile)
}

func parseLexiconBytes(expectedID string, raw []byte) (*lexicon.Lexicon, indigolexicon.SchemaFile, error) {
	parsed, err := lexicon.ParseBytes(raw)
	if err != nil {
		return nil, indigolexicon.SchemaFile{}, fmt.Errorf("failed to parse lexicon %s for GraphQL: %w", expectedID, err)
	}
	if parsed.ID != expectedID {
		return nil, indigolexicon.SchemaFile{}, fmt.Errorf("lexicon ID mismatch: expected %s, document contains %s", expectedID, parsed.ID)
	}

	var schemaFile indigolexicon.SchemaFile
	if err := json.Unmarshal(raw, &schemaFile); err != nil {
		return nil, indigolexicon.SchemaFile{}, fmt.Errorf("failed to parse lexicon %s for Indigo: %w", expectedID, err)
	}
	if schemaFile.ID != expectedID {
		return nil, indigolexicon.SchemaFile{}, fmt.Errorf("lexicon ID mismatch: expected %s, document contains %s", expectedID, schemaFile.ID)
	}
	return parsed, schemaFile, nil
}

// GraphQLRegistry returns the startup registry built from the same exact saved
// Lexicon documents as the Indigo validation catalog.
func (v *Validator) GraphQLRegistry() *lexicon.Registry {
	if v == nil {
		return nil
	}
	return v.registry
}

// HashLexiconJSON returns the sha256 hash of the exact saved Lexicon JSON bytes.
// It intentionally does not canonicalize JSON, so formatting-only changes are
// treated as a new schema version for validation refresh purposes.
func HashLexiconJSON(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// LexiconHash returns the validation fingerprint for a collection. The
// fingerprint includes the collection Lexicon and every saved Lexicon reached
// through ref or union properties.
func (v *Validator) LexiconHash(collection string) (string, bool) {
	if v == nil {
		return "", false
	}
	if _, ok := v.hashes[collection]; !ok {
		return "", false
	}
	ids := v.referencedLexiconIDs(collection)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		hash, ok := v.hashes[id]
		if !ok {
			continue
		}
		parts = append(parts, id+"="+hash)
	}
	return HashLexiconJSON([]byte(strings.Join(parts, "\n"))), true
}

// ValidateRecord validates raw record JSON against the startup Lexicon set and
// returns a status suitable for persistence on the record row.
func (v *Validator) ValidateRecord(collection, rkey string, rawJSON []byte) Result {
	if v == nil || v.registry == nil || v.catalog == nil {
		return Result{Status: StatusValidationError, Error: "validator is not configured; restart Hyperindex after loading saved Lexicons"}
	}

	hash, ok := v.LexiconHash(collection)
	if !ok {
		return Result{Status: StatusUnknownSchema, Error: fmt.Sprintf("no saved lexicon for collection %s in the startup schema", collection)}
	}

	def, ok := v.registry.GetRecordDef(collection)
	if !ok || def == nil {
		return Result{Status: StatusValidationError, Error: fmt.Sprintf("saved lexicon for collection %s has no record definition", collection), LexiconHash: hash}
	}

	record, err := atdata.UnmarshalJSON(rawJSON)
	if err != nil {
		if !json.Valid(rawJSON) {
			return Result{Status: StatusValidationError, Error: fmt.Sprintf("failed to parse record JSON for collection %s: %v", collection, err), LexiconHash: hash}
		}
		return Result{Status: StatusInvalid, Error: fmt.Sprintf("record is not valid AT Protocol data: %v", err), LexiconHash: hash}
	}

	if err := validateRecordKey(def.Key, rkey); err != nil {
		return Result{Status: StatusInvalid, Error: err.Error(), LexiconHash: hash}
	}

	if err := indigolexicon.ValidateRecord(v.catalog, record, collection, 0); err != nil {
		if isCatalogResolutionError(err) {
			return Result{Status: StatusValidationError, Error: fmt.Sprintf("startup Lexicon set is incomplete for collection %s: %v", collection, err), LexiconHash: hash}
		}
		return Result{Status: StatusInvalid, Error: fmt.Sprintf("record does not conform to lexicon %s: %v", collection, err), LexiconHash: hash}
	}
	return Result{Status: StatusValid, LexiconHash: hash}
}

func isCatalogResolutionError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "schema not found in catalog") ||
		strings.Contains(message, "could not resolve known union variant")
}

type rawReferenceDocument struct {
	ID   string                     `json:"id"`
	Defs map[string]json.RawMessage `json:"defs"`
}

func buildReferenceGraph(saved map[string][]byte) (map[string][]string, error) {
	graph := make(map[string][]string)
	ids := make([]string, 0, len(saved))
	for id := range saved {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, expectedID := range ids {
		var document rawReferenceDocument
		if err := json.Unmarshal(saved[expectedID], &document); err != nil {
			return nil, fmt.Errorf("failed to inspect references in saved lexicon %s: %w", expectedID, err)
		}
		if document.ID != expectedID {
			return nil, fmt.Errorf("lexicon ID mismatch: expected %s, document contains %s", expectedID, document.ID)
		}

		defNames := make([]string, 0, len(document.Defs))
		for name := range document.Defs {
			defNames = append(defNames, name)
		}
		sort.Strings(defNames)
		for _, name := range defNames {
			node := definitionRef(expectedID, name)
			refs, err := collectDefinitionRefs(document.Defs[name], expectedID)
			if err != nil {
				return nil, fmt.Errorf("inspect references in %s: %w", node, err)
			}
			graph[node] = refs
		}
	}
	return graph, nil
}

func collectDefinitionRefs(raw json.RawMessage, contextID string) ([]string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	collectRefs(value, contextID, seen)
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs, nil
}

func collectRefs(value any, contextID string, refs map[string]struct{}) {
	switch value := value.(type) {
	case map[string]any:
		if ref, ok := value["ref"].(string); ok && ref != "" {
			refs[normalizeDefinitionRef(ref, contextID)] = struct{}{}
		}
		if rawRefs, ok := value["refs"].([]any); ok {
			for _, rawRef := range rawRefs {
				if ref, ok := rawRef.(string); ok && ref != "" {
					refs[normalizeDefinitionRef(ref, contextID)] = struct{}{}
				}
			}
		}
		for _, child := range value {
			collectRefs(child, contextID, refs)
		}
	case []any:
		for _, child := range value {
			collectRefs(child, contextID, refs)
		}
	}
}

func definitionRef(id, name string) string {
	if name == "main" {
		return id
	}
	return id + "#" + name
}

func normalizeDefinitionRef(ref, contextID string) string {
	if strings.HasPrefix(ref, "#") {
		ref = contextID + ref
	}
	return strings.TrimSuffix(ref, "#main")
}

func validateReferenceClosure(graph map[string][]string) error {
	nodes := make([]string, 0, len(graph))
	for node := range graph {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		for _, ref := range graph[node] {
			if _, ok := graph[ref]; !ok {
				return fmt.Errorf("%s references %s, which is not present in the saved Lexicon set", node, ref)
			}
		}
	}
	return nil
}

func validateRecordKey(rule, rkey string) error {
	switch {
	case rule == "any" || rule == "record-key":
		if _, err := syntax.ParseRecordKey(rkey); err != nil {
			return fmt.Errorf("record key expected record-key for rule %s, got %q: %w", rule, rkey, err)
		}
	case rule == "tid":
		if _, err := syntax.ParseTID(rkey); err != nil {
			return fmt.Errorf("record key expected tid, got %q: %w", rkey, err)
		}
	case rule == "nsid":
		if _, err := syntax.ParseNSID(rkey); err != nil {
			return fmt.Errorf("record key expected nsid, got %q: %w", rkey, err)
		}
	case strings.HasPrefix(rule, "literal:"):
		literal := strings.TrimPrefix(rule, "literal:")
		if rkey != literal {
			return fmt.Errorf("record key expected literal %q, got %q", literal, rkey)
		}
	default:
		return fmt.Errorf("saved lexicon uses unsupported record key rule %q; update the Lexicon and restart Hyperindex", rule)
	}
	return nil
}

func (v *Validator) referencedLexiconIDs(collection string) []string {
	lexiconIDs := make(map[string]struct{})
	visited := make(map[string]struct{})
	stack := []string{collection}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := visited[node]; ok {
			continue
		}
		visited[node] = struct{}{}
		lexiconID, _, _ := strings.Cut(node, "#")
		lexiconIDs[lexiconID] = struct{}{}
		stack = append(stack, v.referenceGraph[node]...)
	}

	result := make([]string, 0, len(lexiconIDs))
	for id := range lexiconIDs {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
