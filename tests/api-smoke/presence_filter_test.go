//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"testing"
)

const activityWhereInputType = "OrgHypercertsClaimActivityWhereInput"

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

	_ = ctx // Kept in the signature so callers can share one smoke-test context.
	schema := fetchGraphQLSchema(t, config)
	types := typesByName(schema.Types)

	whereInput := requireSchemaType(t, types, activityWhereInputType)
	if len(whereInput.InputFields) == 0 {
		t.Fatalf("SmokeActivityPresenceSchema: %s not found or has no inputFields", activityWhereInputType)
	}

	imageField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "image")
	imageFilterType := namedTypeName(imageField.Type)
	if imageFilterType == "" {
		t.Fatalf("SmokeActivityPresenceSchema: %s.image has no named input type: %+v", activityWhereInputType, imageField.Type)
	}

	imageFilterInput := requireSchemaType(t, types, imageFilterType)
	isNullField := requireSchemaInputField(t, inputFieldsByName(imageFilterInput.InputFields), "isNull")
	if got := namedTypeName(isNullField.Type); got != "Boolean" {
		t.Fatalf("SmokeActivityPresenceSchema: %s.isNull type = %+v, want Boolean", imageFilterType, isNullField.Type)
	}
}

func inputFieldsByName(fields []schemaInputField) map[string]schemaInputField {
	indexed := make(map[string]schemaInputField, len(fields))
	for _, field := range fields {
		indexed[field.Name] = field
	}
	return indexed
}
