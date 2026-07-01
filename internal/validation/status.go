// Package validation contains the local record validation gate types shared by
// ingestion, repositories, and GraphQL serving code.
package validation

// Status describes whether a raw AT Protocol record is safe to expose through
// typed GraphQL fields generated from Hyperindex's saved lexicons.
type Status string

const (
	// StatusValid means the record conforms to the saved lexicon for its collection.
	StatusValid Status = "valid"
	// StatusInvalid means a saved lexicon exists, but the record does not conform to it.
	StatusInvalid Status = "invalid"
	// StatusUnknownSchema means Hyperindex has no saved lexicon for the collection.
	StatusUnknownSchema Status = "unknown_schema"
	// StatusValidationError means validation could not complete because of an internal parsing or validation error.
	StatusValidationError Status = "validation_error"
)

// Result is the persisted outcome of validating one raw record against the
// saved lexicon currently known for its collection.
type Result struct {
	Status      Status
	Error       string
	LexiconHash string
}

// RecordValidator classifies raw AT Protocol records using only Hyperindex's
// local saved lexicons. Implementations must not perform remote schema lookup
// during ingestion.
type RecordValidator interface {
	ValidateRecord(collection string, rkey string, rawJSON []byte) Result
	LexiconHash(collection string) (string, bool)
}
