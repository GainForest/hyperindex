package lexicon

import (
	"testing"
)

func TestParseSimpleRecordLexicon(t *testing.T) {
	json := `{
		"lexicon": 1,
		"id": "xyz.statusphere.status",
		"defs": {
			"main": {
				"type": "record",
				"record": {
					"type": "object",
					"required": ["text"],
					"properties": {
						"text": {"type": "string"},
						"createdAt": {"type": "string"}
					}
				}
			}
		}
	}`

	lexicon, err := Parse(json)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if lexicon.ID != "xyz.statusphere.status" {
		t.Errorf("Expected ID 'xyz.statusphere.status', got '%s'", lexicon.ID)
	}

	if lexicon.Defs.Main == nil {
		t.Fatal("Expected main definition, got nil")
	}

	if lexicon.Defs.Main.Type != "record" {
		t.Errorf("Expected main type 'record', got '%s'", lexicon.Defs.Main.Type)
	}

	if len(lexicon.Defs.Main.Properties) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(lexicon.Defs.Main.Properties))
	}

	// Check that text is required
	textProp := lexicon.Defs.Main.GetProperty("text")
	if textProp == nil {
		t.Fatal("Expected 'text' property")
		return
	}
	if !textProp.Required {
		t.Error("Expected 'text' to be required")
	}

	// Check that createdAt is not required
	createdAtProp := lexicon.Defs.Main.GetProperty("createdAt")
	if createdAtProp == nil {
		t.Fatal("Expected 'createdAt' property")
		return
	}
	if createdAtProp.Required {
		t.Error("Expected 'createdAt' to be optional")
	}
}

func TestParseLexiconWithOptionalFields(t *testing.T) {
	json := `{
		"lexicon": 1,
		"id": "xyz.statusphere.profile",
		"defs": {
			"main": {
				"type": "record",
				"record": {
					"type": "object",
					"properties": {
						"displayName": {"type": "string"},
						"bio": {"type": "string"}
					}
				}
			}
		}
	}`

	lexicon, err := Parse(json)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if lexicon.ID != "xyz.statusphere.profile" {
		t.Errorf("Expected ID 'xyz.statusphere.profile', got '%s'", lexicon.ID)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	json := "{invalid json"

	_, err := Parse(json)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestParseLexiconMissingID(t *testing.T) {
	json := `{
		"lexicon": 1,
		"defs": {
			"main": {
				"type": "record"
			}
		}
	}`

	_, err := Parse(json)
	if err == nil {
		t.Error("Expected error for missing ID")
	}
}

func TestParseArrayWithRefItems(t *testing.T) {
	json := `{
		"lexicon": 1,
		"id": "fm.teal.alpha.feed.track",
		"defs": {
			"main": {
				"type": "record",
				"record": {
					"type": "object",
					"properties": {
						"artists": {
							"type": "array",
							"items": {
								"type": "ref",
								"ref": "fm.teal.alpha.feed.defs#artist"
							}
						}
					}
				}
			}
		}
	}`

	lexicon, err := Parse(json)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	artistsProp := lexicon.Defs.Main.GetProperty("artists")
	if artistsProp == nil {
		t.Fatal("Expected 'artists' property")
	}

	if artistsProp.Type != "array" {
		t.Errorf("Expected type 'array', got '%s'", artistsProp.Type)
	}

	if artistsProp.Items == nil {
		t.Fatal("Expected items definition")
	}

	if artistsProp.Items.Type != "ref" {
		t.Errorf("Expected items type 'ref', got '%s'", artistsProp.Items.Type)
	}

	if artistsProp.Items.Ref != "fm.teal.alpha.feed.defs#artist" {
		t.Errorf("Expected items ref 'fm.teal.alpha.feed.defs#artist', got '%s'", artistsProp.Items.Ref)
	}
}

func TestParseArrayWithUnionItems(t *testing.T) {
	json := `{
		"lexicon": 1,
		"id": "fm.teal.alpha.feed.track",
		"defs": {
			"main": {
				"type": "record",
				"record": {
					"type": "object",
					"properties": {
						"creators": {
							"type": "array",
							"items": {
								"type": "union",
								"refs": [
									"fm.teal.alpha.feed.defs#artist",
									"fm.teal.alpha.feed.defs#band"
								]
							}
						}
					}
				}
			}
		}
	}`

	lexicon, err := Parse(json)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	creatorsProp := lexicon.Defs.Main.GetProperty("creators")
	if creatorsProp == nil {
		t.Fatal("Expected 'creators' property")
	}

	if creatorsProp.Items == nil {
		t.Fatal("Expected items definition")
	}

	if creatorsProp.Items.Type != "union" {
		t.Errorf("Expected items type 'union', got '%s'", creatorsProp.Items.Type)
	}

	if len(creatorsProp.Items.Refs) != 2 {
		t.Errorf("Expected 2 refs, got %d", len(creatorsProp.Items.Refs))
	}
}

func TestParseUnionProperty(t *testing.T) {
	json := `{
		"lexicon": 1,
		"id": "app.bsky.feed.post",
		"defs": {
			"main": {
				"type": "record",
				"record": {
					"type": "object",
					"properties": {
						"embed": {
							"type": "union",
							"refs": [
								"app.bsky.embed.images",
								"app.bsky.embed.video"
							]
						}
					}
				}
			}
		}
	}`

	lexicon, err := Parse(json)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	embedProp := lexicon.Defs.Main.GetProperty("embed")
	if embedProp == nil {
		t.Fatal("Expected 'embed' property")
	}

	if embedProp.Type != "union" {
		t.Errorf("Expected type 'union', got '%s'", embedProp.Type)
	}

	if len(embedProp.Refs) != 2 {
		t.Errorf("Expected 2 refs, got %d", len(embedProp.Refs))
	}

	if !embedProp.IsUnion() {
		t.Error("Expected IsUnion() to return true")
	}
}

func TestParseLexiconWithoutMain(t *testing.T) {
	json := `{
		"lexicon": 1,
		"id": "com.atproto.label.defs",
		"defs": {
			"selfLabels": {
				"type": "object",
				"required": ["values"],
				"properties": {
					"values": {
						"type": "array",
						"items": {"ref": "#selfLabel", "type": "ref"},
						"maxLength": 10
					}
				}
			},
			"selfLabel": {
				"type": "object",
				"required": ["val"],
				"properties": {
					"val": {"type": "string", "maxLength": 128}
				}
			}
		}
	}`

	lexicon, err := Parse(json)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if lexicon.ID != "com.atproto.label.defs" {
		t.Errorf("Expected ID 'com.atproto.label.defs', got '%s'", lexicon.ID)
	}

	if lexicon.Defs.Main != nil {
		t.Error("Expected main to be nil")
	}

	if len(lexicon.Defs.Others) != 2 {
		t.Errorf("Expected 2 other definitions, got %d", len(lexicon.Defs.Others))
	}

	selfLabels, ok := lexicon.Defs.Others["selfLabels"]
	if !ok {
		t.Fatal("Expected 'selfLabels' definition")
	}

	if !selfLabels.IsObject() {
		t.Error("Expected selfLabels to be an object definition")
	}
}

func TestParseObjectMainWithOthers(t *testing.T) {
	json := `{
		"lexicon": 1,
		"id": "app.bsky.richtext.facet",
		"defs": {
			"main": {
				"type": "object",
				"required": ["index", "features"],
				"properties": {
					"index": {"ref": "#byteSlice", "type": "ref"},
					"features": {
						"type": "array",
						"items": {
							"refs": ["#mention", "#link", "#tag"],
							"type": "union"
						}
					}
				}
			},
			"mention": {
				"type": "object",
				"required": ["did"],
				"properties": {
					"did": {"type": "string", "format": "did"}
				}
			},
			"link": {
				"type": "object",
				"required": ["uri"],
				"properties": {
					"uri": {"type": "string", "format": "uri"}
				}
			},
			"tag": {
				"type": "object",
				"required": ["tag"],
				"properties": {
					"tag": {"type": "string"}
				}
			},
			"byteSlice": {
				"type": "object",
				"required": ["byteStart", "byteEnd"],
				"properties": {
					"byteStart": {"type": "integer"},
					"byteEnd": {"type": "integer"}
				}
			}
		}
	}`

	lexicon, err := Parse(json)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if lexicon.ID != "app.bsky.richtext.facet" {
		t.Errorf("Expected ID 'app.bsky.richtext.facet', got '%s'", lexicon.ID)
	}

	if lexicon.Defs.Main == nil {
		t.Fatal("Expected main definition")
	}

	if lexicon.Defs.Main.Type != "object" {
		t.Errorf("Expected main type 'object', got '%s'", lexicon.Defs.Main.Type)
	}

	if len(lexicon.Defs.Others) != 4 {
		t.Errorf("Expected 4 other definitions, got %d", len(lexicon.Defs.Others))
	}

	// Check each other def exists
	for _, name := range []string{"mention", "link", "tag", "byteSlice"} {
		if _, ok := lexicon.Defs.Others[name]; !ok {
			t.Errorf("Expected '%s' definition", name)
		}
	}
}
