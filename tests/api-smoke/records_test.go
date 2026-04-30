//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const smokeRecordsQuery = `
query SmokeRecords($collection: String!, $first: Int!) {
  records(collection: $collection, first: $first) {
    edges {
      node {
        uri
        cid
        did
        collection
        rkey
        value
      }
    }
  }
}`

type recordsQueryResponse struct {
	Records recordConnection `json:"records"`
}

type recordConnection struct {
	Edges []recordEdge `json:"edges"`
}

type recordEdge struct {
	Node Record `json:"node"`
}

type typedByURIRecord struct {
	URI  string `json:"uri"`
	DID  string `json:"did"`
	CID  string `json:"cid"`
	RKey string `json:"rkey"`
}

func TestRequiredCollectionsAreQueryable(t *testing.T) {
	config := loadSmokeConfig(t)

	for collection := range config.expectations.TypedQueryFields {
		collection := collection
		t.Run(collection, func(t *testing.T) {
			t.Logf("generic records check collection=%q first=%d", collection, 1)
			response := fetchGenericRecords(t, config, collection, 1)

			for edgeIndex, edge := range response.Records.Edges {
				assertGenericRecordShape(t, collection, edgeIndex, edge.Node)
			}
		})
	}
}

func TestDataBearingCollectionRecordShape(t *testing.T) {
	config := loadSmokeConfig(t)

	for _, collection := range config.expectations.DataBearingCollections {
		collection := collection
		t.Run(collection.NSID, func(t *testing.T) {
			t.Logf("data-bearing record shape check collection=%q first=%d", collection.NSID, 10)
			response := fetchGenericRecords(t, config, collection.NSID, 10)
			if len(response.Records.Edges) != 10 {
				t.Fatalf("records(%q, first: 10) returned %d edges, want exactly 10", collection.NSID, len(response.Records.Edges))
			}

			for edgeIndex, edge := range response.Records.Edges {
				assertGenericRecordShape(t, collection.NSID, edgeIndex, edge.Node)
			}
		})
	}
}

func TestTypedByURIRoundTrip(t *testing.T) {
	config := loadSmokeConfig(t)

	for _, collection := range config.expectations.DataBearingCollections {
		collection := collection
		t.Run(collection.NSID, func(t *testing.T) {
			typedField := config.expectations.TypedQueryFields[collection.NSID]
			t.Logf("typed ByUri roundtrip check collection=%q typedField=%q", collection.NSID, typedField)
			genericResponse := fetchGenericRecords(t, config, collection.NSID, 1)
			if len(genericResponse.Records.Edges) != 1 {
				t.Fatalf("records(%q, first: 1) returned %d edges, want exactly 1", collection.NSID, len(genericResponse.Records.Edges))
			}

			genericRecord := genericResponse.Records.Edges[0].Node
			assertGenericRecordShape(t, collection.NSID, 0, genericRecord)

			typedRecord := fetchTypedRecordByURI(t, config, typedField, genericRecord.URI)
			if typedRecord == nil {
				t.Fatalf("%sByUri(%q) returned null", typedField, genericRecord.URI)
			}

			assertMatchingRecordMetadata(t, typedField+"ByUri", genericRecord, *typedRecord)
		})
	}
}

func fetchGenericRecords(t testing.TB, config smokeConfig, collection string, first int) recordsQueryResponse {
	t.Helper()

	response := postGraphQL(t, context.Background(), config, "SmokeRecords", smokeRecordsQuery, map[string]any{
		"collection": collection,
		"first":      first,
	})

	var decoded recordsQueryResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeRecords data for collection %q: %v", collection, err)
	}

	return decoded
}

func fetchTypedRecordByURI(t testing.TB, config smokeConfig, typedField string, uri string) *typedByURIRecord {
	t.Helper()

	typedByURIField := typedField + "ByUri"
	query := fmt.Sprintf(`
query SmokeByUri($uri: String!) {
  %s(uri: $uri) {
    uri
    did
    cid
    rkey
  }
}`, typedByURIField)

	response := postGraphQL(t, context.Background(), config, "SmokeByUri", query, map[string]any{
		"uri": uri,
	})

	var decoded map[string]*typedByURIRecord
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeByUri data for %s(%q): %v", typedByURIField, uri, err)
	}

	return decoded[typedByURIField]
}

func assertGenericRecordShape(t testing.TB, collection string, edgeIndex int, record Record) {
	t.Helper()
	location := fmt.Sprintf("collection %q edge %d uri=%q did=%q", collection, edgeIndex, record.URI, record.DID)

	if !strings.HasPrefix(record.URI, "at://") {
		t.Fatalf("record shape %s: uri want at:// prefix", location)
	}
	if !strings.HasPrefix(record.DID, "did:") {
		t.Fatalf("record shape %s: did want did: prefix", location)
	}
	if record.Collection != collection {
		t.Fatalf("record shape %s: collection = %q, want %q", location, record.Collection, collection)
	}
	if !strings.Contains(record.URI, "/"+collection+"/") {
		t.Fatalf("record shape %s: uri want to contain collection segment %q", location, "/"+collection+"/")
	}
	if record.CID == "" {
		t.Fatalf("record shape %s: cid is empty", location)
	}
	if record.RKey == "" {
		t.Fatalf("record shape %s: rkey is empty", location)
	}
	if record.Value == nil {
		t.Fatalf("record shape %s: value is null, want JSON object", location)
	}
	if rawType, ok := record.Value["$type"]; ok {
		typeName, ok := rawType.(string)
		if !ok {
			t.Fatalf("record shape %s: value $type = %T(%v), want string %q", location, rawType, rawType, collection)
		}
		if typeName != collection {
			t.Fatalf("record shape %s: value $type = %q, want %q", location, typeName, collection)
		}
	}
}

func assertMatchingRecordMetadata(t testing.TB, typedByURIField string, generic Record, typed typedByURIRecord) {
	t.Helper()

	if typed.URI != generic.URI {
		t.Fatalf("%s uri = %q, want %q", typedByURIField, typed.URI, generic.URI)
	}
	if typed.DID != generic.DID {
		t.Fatalf("%s did = %q, want %q", typedByURIField, typed.DID, generic.DID)
	}
	if typed.CID != generic.CID {
		t.Fatalf("%s cid = %q, want %q", typedByURIField, typed.CID, generic.CID)
	}
	if typed.RKey != generic.RKey {
		t.Fatalf("%s rkey = %q, want %q", typedByURIField, typed.RKey, generic.RKey)
	}
}
