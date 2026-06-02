//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"testing"
)

const (
	activityWhereInputType = "OrgHypercertsClaimActivityWhereInput"
	presenceFilterType     = "PresenceFilterInput"
)

const smokeActivityPresenceSchemaQuery = `
query SmokeActivityPresenceSchema {
  whereInput: __type(name: "OrgHypercertsClaimActivityWhereInput") {
    inputFields {
      name
      type {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
          }
        }
      }
    }
  }
  presenceInput: __type(name: "PresenceFilterInput") {
    inputFields {
      name
      type {
        kind
        name
        ofType {
          kind
          name
        }
      }
    }
  }
}`

type activityPresenceSchemaResponse struct {
	WhereInput    schemaInputType `json:"whereInput"`
	PresenceInput schemaInputType `json:"presenceInput"`
}

type schemaInputType struct {
	InputFields []schemaInputField `json:"inputFields"`
}

type schemaInputField struct {
	Name string        `json:"name"`
	Type schemaTypeRef `json:"type"`
}

type activityPresenceConnection struct {
	Edges []struct {
		Node struct {
			URI              string                 `json:"uri"`
			Title            string                 `json:"title"`
			ShortDescription string                 `json:"shortDescription"`
			Image            *activityPresenceImage `json:"image"`
		} `json:"node"`
	} `json:"edges"`
}

type activityPresenceImage struct {
	Typename string `json:"__typename"`
}

func TestActivityImagePresenceFilterSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	ctx := context.Background()

	typedField, ok := config.expectations.TypedQueryFields[activityCollection]
	if !ok {
		t.Fatalf("smoke expectations are missing typed field for %q", activityCollection)
	}
	if typedField != "orgHypercertsClaimActivity" {
		t.Fatalf("smoke expectations typed field for %q = %q, want orgHypercertsClaimActivity", activityCollection, typedField)
	}

	assertActivityImagePresenceSchema(t, ctx, config)

	response := postGraphQL(t, ctx, config, "SmokeActivityImagePresenceFilter", `
query SmokeActivityImagePresenceFilter($first: Int!) {
  orgHypercertsClaimActivity(first: $first, where: { image: { isNull: false } }) {
    edges {
      node {
        uri
        title
        shortDescription
        image {
          __typename
        }
      }
    }
  }
}`, map[string]any{"first": 1})

	var payload struct {
		Activity activityPresenceConnection `json:"orgHypercertsClaimActivity"`
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeActivityImagePresenceFilter: decode response data: %v", err)
	}
	if len(payload.Activity.Edges) == 0 {
		t.Fatal("SmokeActivityImagePresenceFilter: image presence filter returned no activity records, want at least one record with image present")
	}
	for index, edge := range payload.Activity.Edges {
		if edge.Node.URI == "" {
			t.Fatalf("SmokeActivityImagePresenceFilter: edge %d node.uri is empty", index)
		}
		if edge.Node.Title == "" {
			t.Fatalf("SmokeActivityImagePresenceFilter: edge %d node.title is empty for uri %q", index, edge.Node.URI)
		}
		if edge.Node.Image == nil || edge.Node.Image.Typename == "" {
			t.Fatalf("SmokeActivityImagePresenceFilter: edge %d node.image is absent for uri %q", index, edge.Node.URI)
		}
	}

	smokeLog("✓ %s image presence filter works", activityCollection)
}

func assertActivityImagePresenceSchema(t testing.TB, ctx context.Context, config smokeConfig) {
	t.Helper()

	response := postGraphQL(t, ctx, config, "SmokeActivityPresenceSchema", smokeActivityPresenceSchemaQuery, nil)

	var payload activityPresenceSchemaResponse
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeActivityPresenceSchema: decode response data: %v", err)
	}
	if len(payload.WhereInput.InputFields) == 0 {
		t.Fatalf("SmokeActivityPresenceSchema: %s not found or has no inputFields", activityWhereInputType)
	}
	if len(payload.PresenceInput.InputFields) == 0 {
		t.Fatalf("SmokeActivityPresenceSchema: %s not found or has no inputFields", presenceFilterType)
	}

	whereFields := inputFieldsByName(payload.WhereInput.InputFields)
	imageField, ok := whereFields["image"]
	if !ok {
		t.Fatalf("SmokeActivityPresenceSchema: %s is missing image input field", activityWhereInputType)
	}
	if got := namedTypeName(imageField.Type); got != presenceFilterType {
		t.Fatalf("SmokeActivityPresenceSchema: %s.image type = %q, want %q", activityWhereInputType, got, presenceFilterType)
	}

	presenceFields := inputFieldsByName(payload.PresenceInput.InputFields)
	isNullField, ok := presenceFields["isNull"]
	if !ok {
		t.Fatalf("SmokeActivityPresenceSchema: %s is missing isNull input field", presenceFilterType)
	}
	if isNullField.Type.Kind != "NON_NULL" || isNullField.Type.OfType == nil || isNullField.Type.OfType.Name != "Boolean" {
		t.Fatalf("SmokeActivityPresenceSchema: %s.isNull type = %+v, want Boolean!", presenceFilterType, isNullField.Type)
	}
}

func inputFieldsByName(fields []schemaInputField) map[string]schemaInputField {
	indexed := make(map[string]schemaInputField, len(fields))
	for _, field := range fields {
		indexed[field.Name] = field
	}
	return indexed
}
