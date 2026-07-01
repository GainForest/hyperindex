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
query SmokeRecords($collection: String!, $first: Int!, $after: String) {
  records(collection: $collection, first: $first, after: $after) {
    edges {
      cursor
      node {
        uri
        cid
        did
        collection
        rkey
        value
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}`

type recordsQueryResponse struct {
	Records recordConnection `json:"records"`
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type recordConnection struct {
	Edges    []recordEdge `json:"edges"`
	PageInfo pageInfo     `json:"pageInfo"`
}

type recordEdge struct {
	Cursor string `json:"cursor"`
	Node   Record `json:"node"`
}

type typedByURIRecord struct {
	URI  string `json:"uri"`
	DID  string `json:"did"`
	CID  string `json:"cid"`
	RKey string `json:"rkey"`
}

type typedRecordConnection struct {
	Edges []typedRecordEdge `json:"edges"`
}

type typedRecordEdge struct {
	Node typedByURIRecord `json:"node"`
}

func TestRequiredCollectionsAreQueryable(t *testing.T) {
	config := loadSmokeConfig(t)

	for collection := range config.expectations.TypedQueryFields {
		collection := collection
		t.Run(collection, func(t *testing.T) {
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

func TestTypedURIWhereFilterRoundTrip(t *testing.T) {
	config := loadSmokeConfig(t)

	for _, collection := range config.expectations.DataBearingCollections {
		collection := collection
		t.Run(collection.NSID, func(t *testing.T) {
			typedField := config.expectations.TypedQueryFields[collection.NSID]
			genericResponse := fetchGenericRecords(t, config, collection.NSID, 1)
			if len(genericResponse.Records.Edges) != 1 {
				t.Fatalf("records(%q, first: 1) returned %d edges, want exactly 1", collection.NSID, len(genericResponse.Records.Edges))
			}

			genericRecord := genericResponse.Records.Edges[0].Node
			assertGenericRecordShape(t, collection.NSID, 0, genericRecord)

			eqRecords := fetchTypedRecordsByURIWhereEQ(t, config, typedField, genericRecord.URI)
			assertSingleURIWhereMatch(t, typedField+" where.uri.eq", genericRecord, eqRecords)

			inRecords := fetchTypedRecordsByURIWhereIn(t, config, typedField, genericRecord.URI)
			assertSingleURIWhereMatch(t, typedField+" where.uri.in", genericRecord, inRecords)
		})
	}

	smokeLog("✓ Typed uri where filters work for eq and in")
}

func fetchGenericRecords(t testing.TB, config smokeConfig, collection string, first int) recordsQueryResponse {
	t.Helper()
	return fetchGenericRecordsPage(t, config, collection, first, "")
}

func fetchAllGenericRecords(t testing.TB, config smokeConfig, collection string) []recordEdge {
	t.Helper()

	var edges []recordEdge
	after := ""
	for {
		page := fetchGenericRecordsPage(t, config, collection, 1000, after)
		edges = append(edges, page.Records.Edges...)
		if !page.Records.PageInfo.HasNextPage {
			return edges
		}
		if page.Records.PageInfo.EndCursor == "" {
			t.Fatalf("records(%q) has next page without an end cursor", collection)
		}
		after = page.Records.PageInfo.EndCursor
	}
}

func fetchGenericRecordsPage(t testing.TB, config smokeConfig, collection string, first int, after string) recordsQueryResponse {
	t.Helper()

	var afterValue any
	if after != "" {
		afterValue = after
	}
	response := postGraphQL(t, context.Background(), config, "SmokeRecords", smokeRecordsQuery, map[string]any{
		"collection": collection,
		"first":      first,
		"after":      afterValue,
	})

	var decoded recordsQueryResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeRecords data for collection %q: %v", collection, err)
	}

	return decoded
}

func fetchTypedRecordsByURIWhereEQ(t testing.TB, config smokeConfig, typedField string, uri string) []typedByURIRecord {
	t.Helper()

	query := fmt.Sprintf(`
query SmokeTypedURIWhereEQ($uri: String!) {
  %s(first: 2, where: { uri: { eq: $uri } }) {
    edges {
      node {
        uri
        did
        cid
        rkey
      }
    }
  }
}`, typedField)

	response := postGraphQL(t, context.Background(), config, "SmokeTypedURIWhereEQ", query, map[string]any{
		"uri": uri,
	})

	return decodeTypedRecordConnection(t, response, "SmokeTypedURIWhereEQ", typedField, uri)
}

func fetchTypedRecordsByURIWhereIn(t testing.TB, config smokeConfig, typedField string, uri string) []typedByURIRecord {
	t.Helper()

	query := fmt.Sprintf(`
query SmokeTypedURIWhereIn($uris: [String!]) {
  %s(first: 2, where: { uri: { in: $uris } }) {
    edges {
      node {
        uri
        did
        cid
        rkey
      }
    }
  }
}`, typedField)

	response := postGraphQL(t, context.Background(), config, "SmokeTypedURIWhereIn", query, map[string]any{
		"uris": []string{uri},
	})

	return decodeTypedRecordConnection(t, response, "SmokeTypedURIWhereIn", typedField, uri)
}

func decodeTypedRecordConnection(t testing.TB, response GraphQLResponse, operationName string, typedField string, uri string) []typedByURIRecord {
	t.Helper()

	var decoded map[string]typedRecordConnection
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode %s data for %s uri %q: %v", operationName, typedField, uri, err)
	}

	connection := decoded[typedField]
	records := make([]typedByURIRecord, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		records = append(records, edge.Node)
	}
	return records
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

func assertSingleURIWhereMatch(t testing.TB, label string, generic Record, records []typedByURIRecord) {
	t.Helper()

	if len(records) != 1 {
		t.Fatalf("%s returned %d records for uri %q, want exactly 1", label, len(records), generic.URI)
	}
	assertMatchingRecordMetadata(t, label, generic, records[0])
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
