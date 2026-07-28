package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/atdata"
	"github.com/ipfs/go-cid"
)

func TestGetRepoClassifiesUnsupportedAndIntegrityFailures(t *testing.T) {
	t.Run("unsupported endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error":"XRPCNotSupported"}`, http.StatusNotImplemented)
		}))
		defer server.Close()
		client := NewClient("", "")
		_, err := client.GetRepo(t.Context(), server.URL, "did:plc:test", nil)
		kind, ok := carFailureKind(err)
		if !ok || kind != CARFailureUnsupported {
			t.Fatalf("GetRepo() error = %v kind=%q, want unsupported CAR failure", err, kind)
		}
	})

	t.Run("malformed CAR", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-a-car"))
		}))
		defer server.Close()
		client := NewClient("", "")
		_, err := client.GetRepo(t.Context(), server.URL, "did:plc:test", nil)
		if !isCARIntegrityFailure(err) {
			t.Fatalf("GetRepo() error = %v, want integrity CAR failure", err)
		}
	})
}

func TestGetRepoStalledBodyUsesRepoTimeoutAsAvailability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient("", "")
	client.httpClient.Timeout = 5 * time.Millisecond
	client.repoTimeout = 40 * time.Millisecond
	started := time.Now()
	_, err := client.GetRepo(t.Context(), server.URL, "did:plc:test", nil)
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond {
		t.Fatalf("GetRepo() returned after %s, want repo-specific timeout rather than shorter shared timeout", elapsed)
	}
	kind, ok := carFailureKind(err)
	if !ok || kind != CARFailureAvailability || !isCARFallbackFailure(err) || isCARIntegrityFailure(err) {
		t.Fatalf("GetRepo() stalled body error = %v kind=%q, want fallback-eligible availability", err, kind)
	}
}

func TestGetRepoStalledBodyPropagatesParentCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient("", "")
	client.repoTimeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.GetRepo(ctx, server.URL, "did:plc:test", nil)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("GetRepo did not begin reading stalled body")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("GetRepo() error = %v, want context.Canceled", err)
		}
		if _, typed := carFailureKind(err); typed {
			t.Fatalf("parent cancellation was wrapped as CAR failure: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("GetRepo did not return after parent cancellation")
	}
}

func TestGetRepoPropagatesParentCancellation(t *testing.T) {
	client := NewClient("", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetRepo(ctx, "https://pds.example", "did:plc:test", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetRepo() error = %v, want context.Canceled", err)
	}
	if _, typed := carFailureKind(err); typed {
		t.Fatalf("parent cancellation was wrapped as CAR failure: %v", err)
	}
}

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
