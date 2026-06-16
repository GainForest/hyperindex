package backfill

import (
	"encoding/json"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atdata"
	"github.com/ipfs/go-cid"
)

func TestCBORToJSONPreservesATProtoShapes(t *testing.T) {
	linkCID := mustParseCID(t, "bafyreia2j6ice4knovcubkcqjoycjyvracpa5x4we7hrcdvjvm3ox5tfue")
	blobCID := mustParseCID(t, "bafkreibm6jg4plkzqtmzaeij7fbk4uxjg75g3wjt5t5ysvskwyqlplzoue")

	recordCBOR, err := atdata.MarshalCBOR(map[string]any{
		"$type": "app.certified.actor.profile",
		"name":  "Alice",
		"count": int64(42),
		"link":  atdata.CIDLink(linkCID),
		"bytes": atdata.Bytes([]byte{1, 2, 3}),
		"avatar": atdata.Blob{
			Ref:      atdata.CIDLink(blobCID),
			MimeType: "image/png",
			Size:     123,
		},
	})
	if err != nil {
		t.Fatalf("marshal test record CBOR: %v", err)
	}

	jsonStr, err := CBORToJSON(recordCBOR)
	if err != nil {
		t.Fatalf("CBORToJSON() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &got); err != nil {
		t.Fatalf("CBORToJSON() returned invalid JSON: %v\n%s", err, jsonStr)
	}

	if got["$type"] != "app.certified.actor.profile" {
		t.Fatalf("$type = %v, want app.certified.actor.profile", got["$type"])
	}
	if got["name"] != "Alice" {
		t.Fatalf("name = %v, want Alice", got["name"])
	}
	if got["count"] != float64(42) {
		t.Fatalf("count = %v, want 42", got["count"])
	}

	assertJSONLink(t, got["link"], linkCID.String())
	assertJSONBytes(t, got["bytes"], "AQID")

	avatar, ok := got["avatar"].(map[string]any)
	if !ok {
		t.Fatalf("avatar = %T, want object", got["avatar"])
	}
	if avatar["$type"] != "blob" {
		t.Fatalf("avatar.$type = %v, want blob", avatar["$type"])
	}
	if avatar["mimeType"] != "image/png" {
		t.Fatalf("avatar.mimeType = %v, want image/png", avatar["mimeType"])
	}
	if avatar["size"] != float64(123) {
		t.Fatalf("avatar.size = %v, want 123", avatar["size"])
	}
	assertJSONLink(t, avatar["ref"], blobCID.String())
}

func mustParseCID(t *testing.T, raw string) cid.Cid {
	t.Helper()

	parsed, err := cid.Parse(raw)
	if err != nil {
		t.Fatalf("parse CID %q: %v", raw, err)
	}
	return parsed
}

func assertJSONLink(t *testing.T, value any, want string) {
	t.Helper()

	linkObject, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("link value = %T, want object", value)
	}
	if got := linkObject["$link"]; got != want {
		t.Fatalf("$link = %v, want %s", got, want)
	}
}

func assertJSONBytes(t *testing.T, value any, want string) {
	t.Helper()

	bytesObject, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("bytes value = %T, want object", value)
	}
	if got := bytesObject["$bytes"]; got != want {
		t.Fatalf("$bytes = %v, want %s", got, want)
	}
}
