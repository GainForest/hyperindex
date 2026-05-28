package labeler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/events"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/gorilla/websocket"

	"github.com/GainForest/hyperindex/internal/testutil"
)

type cborMarshaler interface {
	MarshalCBOR(io.Writer) error
}

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestBuildSubscribeURL(t *testing.T) {
	got, err := buildSubscribeURL("wss://labeler.example/xrpc/com.atproto.label.subscribeLabels?foo=bar&cursor=999", 0)
	if err != nil {
		t.Fatalf("buildSubscribeURL() error = %v", err)
	}

	if !strings.HasPrefix(got, "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels?") {
		t.Fatalf("buildSubscribeURL() = %q", got)
	}
	if !strings.Contains(got, "cursor=0") || !strings.Contains(got, "foo=bar") {
		t.Fatalf("buildSubscribeURL() = %q, want cursor=0 and foo=bar", got)
	}
}

func TestConvertLabels(t *testing.T) {
	cid := "bafyreiabc"
	exp := "2026-01-02T03:04:05Z"
	neg := true
	ver := int64(1)
	labels := []*comatproto.LabelDefs_Label{
		{
			Src: "did:plc:labeler",
			Uri: "at://did:plc:repo/app.example.record/one",
			Cid: &cid,
			Val: "high-quality",
			Neg: &neg,
			Cts: "2025-01-02T03:04:05.123456789Z",
			Exp: &exp,
			Sig: lexutil.LexBytes{1, 2, 3},
			Ver: &ver,
		},
	}

	inputs, err := ConvertLabels(labels)
	if err != nil {
		t.Fatalf("ConvertLabels() error = %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("ConvertLabels() len = %d, want 1", len(inputs))
	}

	input := inputs[0]
	if input.LabelIndex != 0 || input.Src != "did:plc:labeler" || input.URI != "at://did:plc:repo/app.example.record/one" || input.Val != "high-quality" {
		t.Fatalf("unexpected input: %+v", input)
	}
	if input.CID == nil || *input.CID != cid {
		t.Fatalf("CID = %v, want %q", input.CID, cid)
	}
	if !input.Neg {
		t.Fatalf("Neg = false, want true")
	}
	if input.Exp == nil || *input.Exp != exp {
		t.Fatalf("Exp = %v, want %q", input.Exp, exp)
	}
	if input.Sig == nil || *input.Sig != "AQID" {
		t.Fatalf("Sig = %v, want AQID", input.Sig)
	}
	if input.Ver == nil || *input.Ver != ver {
		t.Fatalf("Ver = %v, want %d", input.Ver, ver)
	}
	if !strings.Contains(input.RawJSON, "high-quality") {
		t.Fatalf("RawJSON = %q, want high-quality", input.RawJSON)
	}
}

func TestEventStreamDecodeLabelsFrame(t *testing.T) {
	labelsEvent := &comatproto.LabelSubscribeLabels_Labels{
		Seq: 7,
		Labels: []*comatproto.LabelDefs_Label{
			{Src: "did:plc:labeler", Uri: "at://did:plc:repo/app.example.record/one", Val: "standard", Cts: "2025-01-02T03:04:05Z"},
		},
	}
	frame := encodeMessageFrame(t, "#labels", labelsEvent)

	var decoded events.XRPCStreamEvent
	if err := decoded.Deserialize(bytes.NewReader(frame)); err != nil {
		t.Fatalf("Deserialize() error = %v", err)
	}

	if decoded.LabelLabels == nil {
		t.Fatal("decoded.LabelLabels = nil")
	}
	if decoded.LabelLabels.Seq != 7 {
		t.Fatalf("Seq = %d, want 7", decoded.LabelLabels.Seq)
	}
	if len(decoded.LabelLabels.Labels) != 1 || decoded.LabelLabels.Labels[0].Val != "standard" {
		t.Fatalf("unexpected labels: %+v", decoded.LabelLabels.Labels)
	}
}

func TestSubscriberPersistsWebsocketLabels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cursorCh := make(chan string, 4)
	labelFrame := encodeMessageFrame(t, "#labels", &comatproto.LabelSubscribeLabels_Labels{
		Seq: 1,
		Labels: []*comatproto.LabelDefs_Label{
			{
				Src: "did:plc:labeler",
				Uri: "at://did:plc:repo/app.example.record/one",
				Val: "standard",
				Cts: "2025-01-02T03:04:05Z",
				Sig: lexutil.LexBytes{1, 2, 3},
				Ver: int64Ptr(1),
			},
		},
	})

	srv := newWebsocketServer(t, func(r *http.Request, conn *websocket.Conn) {
		select {
		case cursorCh <- r.URL.Query().Get("cursor"):
		default:
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, labelFrame); err != nil {
			t.Errorf("WriteMessage() error = %v", err)
			return
		}
		time.Sleep(25 * time.Millisecond)
	})
	defer srv.Close()

	wsURL := httpToWS(srv.URL)
	subscriber := NewSubscriber(repo, Config{
		URLs:         []string{wsURL},
		ReconnectMin: 25 * time.Millisecond,
		ReconnectMax: 25 * time.Millisecond,
	})
	subscriber.Start(ctx)

	waitFor(t, 2*time.Second, func() bool {
		labels, err := repo.ListLabelsByEvent(context.Background(), wsURL, 1)
		return err == nil && len(labels) == 1
	})
	cancel()

	select {
	case cursor := <-cursorCh:
		if cursor != "0" {
			t.Fatalf("cursor = %q, want 0", cursor)
		}
	default:
		t.Fatal("server did not observe cursor query parameter")
	}

	labels, err := repo.ListLabelsByEvent(context.Background(), wsURL, 1)
	if err != nil {
		t.Fatalf("ListLabelsByEvent() error = %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("labels = %d, want 1", len(labels))
	}
	label := labels[0]
	if label.Seq != 1 || label.LabelIndex != 0 || label.Src != "did:plc:labeler" || label.URI != "at://did:plc:repo/app.example.record/one" || label.Val != "standard" {
		t.Fatalf("unexpected label: %+v", label)
	}
	if label.Cts != "2025-01-02T03:04:05Z" {
		t.Fatalf("Cts = %q, want source timestamp", label.Cts)
	}
	if label.Neg {
		t.Fatalf("Neg = true, want false")
	}
	if label.Sig == nil || *label.Sig != "AQID" {
		t.Fatalf("Sig = %v, want AQID", label.Sig)
	}
	if label.Ver == nil || *label.Ver != 1 {
		t.Fatalf("Ver = %v, want 1", label.Ver)
	}

	state, err := repo.GetState(context.Background(), wsURL)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.LastSeq != 1 {
		t.Fatalf("LastSeq = %d, want 1", state.LastSeq)
	}
}

func TestSubscriberRecordsOutdatedCursorInfo(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	message := "cursor too old"
	infoFrame := encodeMessageFrame(t, "#info", &comatproto.SyncSubscribeRepos_Info{
		Name:    "OutdatedCursor",
		Message: &message,
	})

	srv := newWebsocketServer(t, func(_ *http.Request, conn *websocket.Conn) {
		if err := conn.WriteMessage(websocket.BinaryMessage, infoFrame); err != nil {
			t.Errorf("WriteMessage() error = %v", err)
			return
		}
		time.Sleep(25 * time.Millisecond)
	})
	defer srv.Close()

	wsURL := httpToWS(srv.URL)
	subscriber := NewSubscriber(repo, Config{
		URLs:         []string{wsURL},
		ReconnectMin: 25 * time.Millisecond,
		ReconnectMax: 25 * time.Millisecond,
	})
	subscriber.Start(ctx)

	waitFor(t, 2*time.Second, func() bool {
		state, err := repo.GetState(context.Background(), wsURL)
		return err == nil && state.LastError != nil && strings.Contains(*state.LastError, "OutdatedCursor")
	})
	cancel()

	state, err := repo.GetState(context.Background(), wsURL)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.LastError == nil || !strings.Contains(*state.LastError, "cursor too old") {
		t.Fatalf("LastError = %v, want OutdatedCursor message", state.LastError)
	}
}

func encodeMessageFrame(t *testing.T, msgType string, payload cborMarshaler) []byte {
	t.Helper()

	var buf bytes.Buffer
	header := &events.EventHeader{Op: events.EvtKindMessage, MsgType: msgType}
	if err := header.MarshalCBOR(&buf); err != nil {
		t.Fatalf("MarshalCBOR(header) error = %v", err)
	}
	if err := payload.MarshalCBOR(&buf); err != nil {
		t.Fatalf("MarshalCBOR(payload) error = %v", err)
	}
	return buf.Bytes()
}

func newWebsocketServer(t *testing.T, handler func(*http.Request, *websocket.Conn)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade() error = %v", err)
			return
		}
		defer conn.Close()
		handler(r, conn)
	}))
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func httpToWS(rawURL string) string {
	return "ws" + strings.TrimPrefix(rawURL, "http")
}

func int64Ptr(v int64) *int64 {
	return &v
}
