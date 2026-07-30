package validation

import (
	"strings"
	"testing"

	indigolexicon "github.com/bluesky-social/indigo/atproto/lexicon"
)

func TestValidatorValidateRecordWithIndigo(t *testing.T) {
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
						"tags": {"type": "array", "items": {"type": "string", "maxLength": 3}},
						"author": {"type": "ref", "ref": "#author"},
						"embed": {"type": "union", "closed": true, "refs": ["#image"]},
						"languages": {"type": "array", "items": {"type": "string", "format": "language"}},
						"createdAt": {"type": "string", "format": "datetime"},
						"handle": {"type": "string", "format": "handle"},
						"cidText": {"type": "string", "format": "cid"},
						"short": {"type": "string", "maxGraphemes": 2}
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
	wantHash := HashLexiconJSON([]byte("app.example.post=" + HashLexiconJSON([]byte(schema))))

	tests := []struct {
		name      string
		json      string
		status    Status
		wantError string
	}{
		{
			name:   "valid record with scalar array ref and union shapes",
			json:   `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":["go","at"],"author":{"did":"did:plc:abc"},"embed":{"$type":"app.example.post#image","url":"https://example.com/a.png"}}`,
			status: StatusValid,
		},
		{
			name:   "valid formatted strings and RFC3339Nano datetime",
			json:   `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"languages":["en-US"],"createdAt":"2026-01-02T03:04:05.123456789Z","handle":"example.com","cidText":"bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi","short":"👍🏽a"}`,
			status: StatusValid,
		},
		{
			name:      "top level type is required",
			json:      `{"text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "record data missing $type",
		},
		{
			name:      "missing required top level field",
			json:      `{"$type":"app.example.post","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "required field missing: text",
		},
		{
			name:      "wrong scalar type",
			json:      `{"$type":"app.example.post","text":"hello","count":"2","published":true,"tags":[],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "expected an integer",
		},
		{
			name:      "array item constraints are checked",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":["long"],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "string length outside specified range",
		},
		{
			name:      "ref required field is checked",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{}}`,
			status:    StatusInvalid,
			wantError: "required field missing: did",
		},
		{
			name:      "closed union rejects unknown type",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"embed":{"$type":"app.example.post#video"}}`,
			status:    StatusInvalid,
			wantError: "did not match any variant of closed union",
		},
		{
			name:      "array item format is checked",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"languages":["not_a_language"]}`,
			status:    StatusInvalid,
			wantError: "record does not conform",
		},
		{
			name:      "datetime format is checked",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"createdAt":"not-a-time"}`,
			status:    StatusInvalid,
			wantError: "record does not conform",
		},
		{
			name:      "handle format is checked",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"handle":"not a handle"}`,
			status:    StatusInvalid,
			wantError: "record does not conform",
		},
		{
			name:      "cid string format is checked",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"cidText":"not-a-cid"}`,
			status:    StatusInvalid,
			wantError: "record does not conform",
		},
		{
			name:      "grapheme limit is checked",
			json:      `{"$type":"app.example.post","text":"hello","count":2,"published":true,"tags":[],"author":{"did":"did:plc:abc"},"short":"abc"}`,
			status:    StatusInvalid,
			wantError: "record does not conform",
		},
		{
			name:      "malformed json is validation error",
			json:      `{"$type":`,
			status:    StatusValidationError,
			wantError: "failed to parse record JSON for collection app.example.post",
		},
		{
			name:      "fractional number is invalid AT Protocol data",
			json:      `{"$type":"app.example.post","text":"hello","count":1.5,"published":true,"tags":[],"author":{"did":"did:plc:abc"}}`,
			status:    StatusInvalid,
			wantError: "record is not valid AT Protocol data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.ValidateRecord("app.example.post", "3jui7kd54zh2y", []byte(tt.json))
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

func TestValidatorUsesATProtoDataShapes(t *testing.T) {
	const schema = `{
		"lexicon":1,
		"id":"app.example.data",
		"defs":{"main":{"type":"record","key":"any","record":{"type":"object",
			"required":["count","link","payload","image"],
			"properties":{
				"count":{"type":"integer"},
				"link":{"type":"cid-link"},
				"payload":{"type":"bytes"},
				"image":{"type":"blob","accept":["image/*"],"maxSize":1000}
			}
		}}}
	}`
	validator, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.data": []byte(schema)})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}

	const cid = "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
	record := `{"$type":"app.example.data","count":2,"link":{"$link":"` + cid + `"},"payload":{"$bytes":"AQI"},"image":{"$type":"blob","ref":{"$link":"` + cid + `"},"mimeType":"image/png","size":10}}`
	got := validator.ValidateRecord("app.example.data", "self", []byte(record))
	if got.Status != StatusValid {
		t.Fatalf("Status = %q, want %q (error %q)", got.Status, StatusValid, got.Error)
	}
}

func TestValidatorUnknownSchema(t *testing.T) {
	validator, err := NewValidatorFromLexiconBytes(nil)
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}

	got := validator.ValidateRecord("app.example.missing", "rkey", []byte(`{"$type":"app.example.missing","text":"hello"}`))
	if got.Status != StatusUnknownSchema {
		t.Fatalf("Status = %q, want %q", got.Status, StatusUnknownSchema)
	}
	if !strings.Contains(got.Error, "no saved lexicon for collection app.example.missing") {
		t.Fatalf("Error = %q", got.Error)
	}
	if got.LexiconHash != "" {
		t.Fatalf("LexiconHash = %q, want empty", got.LexiconHash)
	}
}

func TestValidatorCatalogResolutionFailureIsValidationError(t *testing.T) {
	const schema = `{"lexicon":1,"id":"com.example.record","defs":{"main":{"type":"record","key":"any","record":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}}}}`
	validator, err := NewValidatorFromLexiconBytes(map[string][]byte{"com.example.record": []byte(schema)})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}
	emptyCatalog := indigolexicon.NewBaseCatalog()
	validator.catalog = &emptyCatalog

	got := validator.ValidateRecord("com.example.record", "any-key", []byte(`{"$type":"com.example.record","name":"test"}`))
	if got.Status != StatusValidationError || !strings.Contains(got.Error, "startup Lexicon set is incomplete") {
		t.Fatalf("ValidateRecord() = status:%q error:%q, want catalog resolution validation_error", got.Status, got.Error)
	}
}

func TestValidatorLexiconHash(t *testing.T) {
	first := []byte(`{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","key":"any","record":{"type":"object","properties":{}}}}}`)
	second := []byte(`{
		"lexicon":1,
		"id":"app.example.post",
		"defs":{"main":{"type":"record","key":"any","record":{"type":"object","properties":{}}}}
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
	want := HashLexiconJSON([]byte("app.example.post=" + HashLexiconJSON(first)))
	if got != want {
		t.Fatalf("LexiconHash() = %q, want validation fingerprint", got)
	}
}

func TestValidatorRejectsInvalidSavedLexicons(t *testing.T) {
	t.Run("missing id", func(t *testing.T) {
		_, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.bad": []byte(`{"defs":{}}`)})
		if err == nil || !strings.Contains(err.Error(), "failed to parse lexicon app.example.bad for GraphQL") {
			t.Fatalf("error = %v, want GraphQL parse error", err)
		}
	})

	t.Run("id mismatch", func(t *testing.T) {
		raw := []byte(`{"lexicon":1,"id":"app.example.actual","defs":{"main":{"type":"record","key":"any","record":{"type":"object","properties":{}}}}}`)
		if err := CheckLexiconBytes("app.example.expected", raw); err == nil || !strings.Contains(err.Error(), "lexicon ID mismatch") {
			t.Fatalf("error = %v, want ID mismatch", err)
		}
	})

	t.Run("Indigo schema check", func(t *testing.T) {
		raw := []byte(`{"lexicon":1,"id":"app.example.bad","defs":{"main":{"type":"record","record":{"type":"object","properties":{}}}}}`)
		if err := CheckLexiconBytes("app.example.bad", raw); err == nil || !strings.Contains(err.Error(), "record key specifier is required") {
			t.Fatalf("error = %v, want Indigo record key error", err)
		}
	})

	t.Run("missing referenced Lexicon", func(t *testing.T) {
		raw := []byte(`{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","key":"any","record":{"type":"object","properties":{"embed":{"type":"union","refs":["app.example.missing#main"]}}}}}}`)
		_, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.post": raw})
		if err == nil || !strings.Contains(err.Error(), "app.example.missing") {
			t.Fatalf("error = %v, want unresolved reference", err)
		}
	})
}

func TestValidatorValidateRecordKey(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		rule       string
		validRKey  string
		invalidKey string
	}{
		{name: "literal", id: "app.example.profile", rule: "literal:self", validRKey: "self", invalidKey: "other"},
		{name: "tid", id: "app.example.post", rule: "tid", validRKey: "3jui7kd54zh2y", invalidKey: "bad/rkey"},
		{name: "any", id: "app.example.any", rule: "any", validRKey: "self", invalidKey: "bad/rkey"},
		{name: "record-key compatibility", id: "app.example.recordkey", rule: "record-key", validRKey: "self", invalidKey: "bad/rkey"},
		{name: "nsid", id: "app.example.nsid", rule: "nsid", validRKey: "com.example.value", invalidKey: "not-an-nsid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := []byte(`{"lexicon":1,"id":"` + tt.id + `","defs":{"main":{"type":"record","key":"` + tt.rule + `","record":{"type":"object","properties":{}}}}}`)
			validator, err := NewValidatorFromLexiconBytes(map[string][]byte{tt.id: schema})
			if err != nil {
				t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
			}
			record := []byte(`{"$type":"` + tt.id + `"}`)
			if got := validator.ValidateRecord(tt.id, tt.validRKey, record); got.Status != StatusValid {
				t.Fatalf("valid Status = %q, want %q (error %q)", got.Status, StatusValid, got.Error)
			}
			if got := validator.ValidateRecord(tt.id, tt.invalidKey, record); got.Status != StatusInvalid {
				t.Fatalf("invalid Status = %q, want %q", got.Status, StatusInvalid)
			}
		})
	}
}

func TestValidatorLexiconHashIncludesTransitiveMainReferences(t *testing.T) {
	collection := []byte(`{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","properties":{"embed":{"type":"ref","ref":"app.example.embed#main"}}}}}}`)
	embed := []byte(`{"lexicon":1,"id":"app.example.embed","defs":{"main":{"type":"object","properties":{"detail":{"type":"ref","ref":"app.example.detail#main"}}}}}`)
	firstDetail := []byte(`{"lexicon":1,"id":"app.example.detail","defs":{"main":{"type":"object","properties":{"text":{"type":"string"}}}}}`)
	secondDetail := []byte(`{"lexicon":1,"id":"app.example.detail","defs":{"main":{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}}}`)

	firstValidator, err := NewValidatorFromLexiconBytes(map[string][]byte{
		"app.example.post": collection, "app.example.embed": embed, "app.example.detail": firstDetail,
	})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes(first) error = %v", err)
	}
	secondValidator, err := NewValidatorFromLexiconBytes(map[string][]byte{
		"app.example.post": collection, "app.example.embed": embed, "app.example.detail": secondDetail,
	})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes(second) error = %v", err)
	}

	firstHash, _ := firstValidator.LexiconHash("app.example.post")
	secondHash, _ := secondValidator.LexiconHash("app.example.post")
	if firstHash == secondHash {
		t.Fatal("LexiconHash() did not change when a transitively referenced #main Lexicon changed")
	}
}

func TestValidatorLexiconHashIncludesRefsNestedInNonMainDefs(t *testing.T) {
	collection := []byte(`{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","properties":{"embed":{"type":"ref","ref":"app.example.embed#view"}}}}}}`)
	embed := []byte(`{"lexicon":1,"id":"app.example.embed","defs":{"view":{"type":"object","properties":{"detail":{"type":"union","refs":["app.example.detail#view"]}}}}}`)
	firstDetail := []byte(`{"lexicon":1,"id":"app.example.detail","defs":{"view":{"type":"object","properties":{"text":{"type":"string"}}}}}`)
	secondDetail := []byte(`{"lexicon":1,"id":"app.example.detail","defs":{"view":{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}}}`)

	firstValidator, err := NewValidatorFromLexiconBytes(map[string][]byte{
		"app.example.post": collection, "app.example.embed": embed, "app.example.detail": firstDetail,
	})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes(first) error = %v", err)
	}
	secondValidator, err := NewValidatorFromLexiconBytes(map[string][]byte{
		"app.example.post": collection, "app.example.embed": embed, "app.example.detail": secondDetail,
	})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes(second) error = %v", err)
	}
	firstHash, _ := firstValidator.LexiconHash("app.example.post")
	secondHash, _ := secondValidator.LexiconHash("app.example.post")
	if firstHash == secondHash {
		t.Fatal("LexiconHash() did not change when a Lexicon referenced through a non-main union changed")
	}
}

func TestValidatorLexiconHashIncludesReferencedLexicons(t *testing.T) {
	collection := []byte(`{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","required":["embed"],"properties":{"embed":{"type":"ref","ref":"app.example.embed#main"}}}}}}`)
	firstEmbed := []byte(`{"lexicon":1,"id":"app.example.embed","defs":{"main":{"type":"object","required":["url"],"properties":{"url":{"type":"string"}}}}}`)
	secondEmbed := []byte(`{"lexicon":1,"id":"app.example.embed","defs":{"main":{"type":"object","required":["url","alt"],"properties":{"url":{"type":"string"},"alt":{"type":"string"}}}}}`)

	firstValidator, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.post": collection, "app.example.embed": firstEmbed})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes(first) error = %v", err)
	}
	secondValidator, err := NewValidatorFromLexiconBytes(map[string][]byte{"app.example.post": collection, "app.example.embed": secondEmbed})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes(second) error = %v", err)
	}

	firstHash, ok := firstValidator.LexiconHash("app.example.post")
	if !ok {
		t.Fatal("first LexiconHash() ok = false")
	}
	secondHash, ok := secondValidator.LexiconHash("app.example.post")
	if !ok {
		t.Fatal("second LexiconHash() ok = false")
	}
	if firstHash == secondHash {
		t.Fatal("LexiconHash() did not change when a referenced Lexicon changed")
	}
}
