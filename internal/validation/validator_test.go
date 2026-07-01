package validation

import (
	"strings"
	"testing"
)

func TestValidatorValidateRecord(t *testing.T) {
	const schema = `{
		"lexicon": 1,
		"id": "app.example.post",
		"defs": {
			"main": {
				"type": "record",
				"key": "tid",
				"record": {
					"type": "object",
					"required": ["text", "count", "published", "tags", "author"],
					"properties": {
						"text": {"type": "string", "maxLength": 10},
						"count": {"type": "integer", "minimum": 1},
						"published": {"type": "boolean"},
						"tags": {"type": "array", "items": {"type": "string"}},
						"author": {"type": "ref", "ref": "#author"},
						"embed": {"type": "union", "refs": ["#image"]}
					}
				}
			},
			"author": {
				"type": "object",
				"required": ["did"],
				"properties": {"did": {"type": "string", "format": "did"}}
			},
			"image": {
				"type": "object",
				"required": ["url"],
				"properties": {"url": {"type": "string", "format": "uri"}}
			}
		}
	}`

	validator, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.post": []byte(schema)})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}
	wantHash := HashLexiconJSON([]byte(schema))

	tests := []struct {
		name      string
		json      string
		status    Status
		wantError string
	}{
		{
			name:   "valid record with scalar array ref and union shapes",
			json:   `{"text":"hello","count":2,"published":true,"tags":["go","at"],"author":{"did":"did:plc:abc"},"embed":{"$type":"app.example.post#image","url":"https://example.com/a.png"}}`,
			status: StatusValid,
		},
		{
			name:      "missing required top level field",
			json:      `{"count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "missing required field: record.text",
		},
		{
			name:      "wrong scalar type",
			json:      `{"text":"hello","count":"2","published":true,"tags":[],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "field record.count expected integer, got string",
		},
		{
			name:      "array item type is checked",
			json:      `{"text":"hello","count":2,"published":true,"tags":[1],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "field record.tags[0] expected string, got number",
		},
		{
			name:      "ref required field is checked",
			json:      `{"text":"hello","count":2,"published":true,"tags":[],"author":{}}`,
			status:    StatusInvalid,
			wantError: "missing required field: record.author.did",
		},
		{
			name:      "union requires known type",
			json:      `{"text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"embed":{"$type":"app.example.post#video"}}`,
			status:    StatusInvalid,
			wantError: "field record.embed union type \"app.example.post#video\" is not one of app.example.post#image",
		},
		{
			name:      "malformed json is validation error",
			json:      `{"text":`,
			status:    StatusValidationError,
			wantError: "failed to parse record JSON for collection app.example.post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.ValidateRecord("app.example.post", "rkey", []byte(tt.json))
			if got.Status != tt.status {
				t.Fatalf("Status = %q, want %q (error %q)", got.Status, tt.status, got.Error)
			}
			if got.LexiconHash != wantHash {
				t.Fatalf("LexiconHash = %q, want %q", got.LexiconHash, wantHash)
			}
			if tt.wantError == "" && got.Error != "" {
				t.Fatalf("Error = %q, want empty", got.Error)
			}
			if tt.wantError != "" && !strings.Contains(got.Error, tt.wantError) {
				t.Fatalf("Error = %q, want containing %q", got.Error, tt.wantError)
			}
		})
	}
}

func TestValidatorUnknownSchema(t *testing.T) {
	validator, err := NewValidatorFromLexiconBytes(map[string][]byte{})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}

	got := validator.ValidateRecord("app.example.missing", "rkey", []byte(`{"text":"hello"}`))
	if got.Status != StatusUnknownSchema {
		t.Fatalf("Status = %q, want %q", got.Status, StatusUnknownSchema)
	}
	if got.Error != "no saved lexicon for collection app.example.missing" {
		t.Fatalf("Error = %q", got.Error)
	}
	if got.LexiconHash != "" {
		t.Fatalf("LexiconHash = %q, want empty", got.LexiconHash)
	}
}

func TestValidatorLexiconHash(t *testing.T) {
	first := []byte(`{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","record":{"type":"object","properties":{}}}}}`)
	second := []byte(`{
		"lexicon":1,
		"id":"app.example.post",
		"defs":{"main":{"type":"record","record":{"type":"object","properties":{}}}}
	}`)

	if HashLexiconJSON(first) == HashLexiconJSON(second) {
		t.Fatal("HashLexiconJSON canonicalized JSON; want exact-byte hash to change when formatting changes")
	}

	validator, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.post": first})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}
	got, ok := validator.LexiconHash("app.example.post")
	if !ok {
		t.Fatal("LexiconHash() ok = false, want true")
	}
	if got != HashLexiconJSON(first) {
		t.Fatalf("LexiconHash() = %q, want exact hash", got)
	}
}

func TestValidatorParseSavedLexiconError(t *testing.T) {
	_, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.bad": []byte(`{"defs":{}}`)})
	if err == nil {
		t.Fatal("NewValidatorFromLexiconBytes() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "failed to parse saved lexicon for collection app.example.bad") {
		t.Fatalf("error = %q", err.Error())
	}
}
