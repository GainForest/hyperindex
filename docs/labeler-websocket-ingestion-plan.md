# Labeler WebSocket Ingestion Implementation Plan

## Scope

Implement **external ATProto labeler websocket ingestion and storage only**.

This slice will:

- Subscribe to external `com.atproto.label.subscribeLabels` streams.
- Persist incoming label events locally.
- Track cursor/sequence per labeler subscription.
- Resume safely after restart.

This slice will **not**:

- Hydrate GraphQL record results with labels.
- Change public GraphQL query behavior.
- Merge external labels with the existing local/admin `label` table.

## Background

Hyperindex already has a `label` table, but that table came from Quickslice's local moderation system. It is used for admin-created labels, report resolution, label definitions, label preferences, and local takedown logic.

It is not ideal as the first storage target for external labeler events because:

- `val` has a foreign key to `label_definition`, while external labelers emit values like `high-quality`, `standard`, `draft`, and `likely-test`.
- It does not track websocket sequence numbers.
- It does not store the source subscription URL.
- It does not store label signatures or protocol version.
- It was designed for local/admin moderation state, not raw external labeler ingestion.

For this reason, external labeler events should be stored in dedicated tables first. GraphQL exposure is handled separately in `docs/external-labels-graphql-plan.md`; external labels still remain separate from the local/admin moderation label table.

## Initial labeler URL

Activity labeler websocket proxy:

```txt
wss://hyperlabel-proxy-test.up.railway.app/xrpc/com.atproto.label.subscribeLabels
```

This endpoint has been verified to support:

```txt
?cursor=0
```

and replay historical events from `seq: 1`. It also resumes correctly with a non-zero cursor, where `cursor=5` returns the next event at `seq: 6`.

Frames are binary ATProto event-stream frames, not JSON. Each WebSocket frame contains two concatenated DAG-CBOR objects: a header (`op`, `t`) and a payload. Use Indigo's event-stream decoder and generated label types instead of hand-rolling CBOR decoding.

## Configuration

Add environment variables:

```env
LABELER_SUBSCRIBE_ENABLED=true
LABELER_SUBSCRIBE_URLS=wss://hyperlabel-proxy-test.up.railway.app/xrpc/com.atproto.label.subscribeLabels
```

Optional later additions:

```env
LABELER_SUBSCRIBE_RECONNECT_MIN=1s
LABELER_SUBSCRIBE_RECONNECT_MAX=60s
```

Behavior:

- If `LABELER_SUBSCRIBE_ENABLED=false`, do not start subscribers.
- If `LABELER_SUBSCRIBE_URLS` is empty, do not start subscribers.
- `LABELER_SUBSCRIBE_URLS` should be a comma-separated list.

## Database migrations

Add migrations for both SQLite and PostgreSQL under:

- `internal/database/migrations/sqlite/`
- `internal/database/migrations/postgres/`

### `label_subscription_state`

Tracks cursor state for each labeler websocket subscription.

Fields:

- `url TEXT PRIMARY KEY`
- `labeler_did TEXT NULL`
- `last_seq BIGINT NOT NULL DEFAULT 0`
- `last_connected_at` nullable timestamp
- `last_event_at` nullable timestamp
- `last_error TEXT NULL`
- `created_at` timestamp
- `updated_at` timestamp

Notes:

- `url` is the subscription identity for this slice.
- `labeler_did` can be filled later once DID/service validation is added.
- `last_seq` must only advance after labels from that event are stored; events with zero labels should still advance the cursor transactionally.

### `external_label`

Stores one row per label received from a subscription event.

Fields:

- `id` primary key
- `subscription_url TEXT NOT NULL`
- `seq BIGINT NOT NULL`
- `label_index INTEGER NOT NULL` — index of the label inside the event's `labels` array
- `src TEXT NOT NULL`
- `uri TEXT NOT NULL`
- `cid TEXT NULL`
- `val TEXT NOT NULL`
- `neg BOOLEAN/INTEGER NOT NULL DEFAULT false`
- `cts` timestamp/text from the label
- `exp` nullable timestamp/text from the label
- `sig TEXT NULL` — base64-encoded signature bytes
- `ver INTEGER NULL`
- `raw_json TEXT/JSONB NULL`
- `received_at` timestamp

Indexes:

- unique replay/idempotency constraint on `(subscription_url, seq, label_index)`
- index on `uri`
- index on `src`
- index on `(subscription_url, seq)`
- index on `val`

Notes:

- Store `cts` and `exp` as the source RFC3339/RFC3339Nano strings for this raw ingestion slice. Do not replace invalid source timestamps with `now`; reject or record the event error instead.
- `label_index` avoids nullable-`cid` uniqueness differences between SQLite and PostgreSQL and preserves duplicate labels in a single event if they ever occur.

## Repository

Add:

```txt
internal/database/repositories/external_labels.go
```

Suggested types:

```go
type LabelSubscriptionState struct {
    URL             string
    LabelerDID      *string
    LastSeq         int64
    LastConnectedAt *time.Time
    LastEventAt     *time.Time
    LastError       *string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type ExternalLabel struct {
    ID              int64
    SubscriptionURL string
    Seq             int64
    LabelIndex      int64
    Src             string
    URI             string
    CID             *string
    Val             string
    Neg             bool
    Cts             string
    Exp             *string
    Sig             *string
    Ver             *int64
    RawJSON         string
    ReceivedAt      time.Time
}
```

Suggested input type:

```go
type ExternalLabelInput struct {
    LabelIndex int64
    Src        string
    URI        string
    CID        *string
    Val        string
    Neg        bool
    Cts        string
    Exp        *string
    Sig        *string
    Ver        *int64
    RawJSON    string
}
```

The labeler subscriber should convert Indigo `*comatproto.LabelDefs_Label` values into `ExternalLabelInput` values before calling the repository. Keep the repository independent from Indigo if possible.

Suggested methods:

- `EnsureState(ctx, url) (*LabelSubscriptionState, error)`
- `GetState(ctx, url) (*LabelSubscriptionState, error)`
- `UpdateConnected(ctx, url) error`
- `UpdateError(ctx, url, errText) error`
- `PersistEvent(ctx, url string, seq int64, labels []ExternalLabelInput) error`
- `UpdateLastSeq(ctx, url string, seq int64) error` if not folded into `PersistEvent`

`PersistEvent` should perform label inserts and cursor update in a single transaction. It should:

- insert labels with `ON CONFLICT (subscription_url, seq, label_index) DO NOTHING` or dialect equivalent
- advance `last_seq` even when the event contains zero labels
- never move `last_seq` backward on replay or out-of-order events

## Subscriber package

Add:

```txt
internal/labeler/
```

Suggested public API:

```go
type Config struct {
    URLs         []string
    ReconnectMin time.Duration
    ReconnectMax time.Duration
}

type Subscriber struct {
    repo *repositories.ExternalLabelsRepository
    cfg  Config
}

func NewSubscriber(repo *repositories.ExternalLabelsRepository, cfg Config) *Subscriber
func (s *Subscriber) Start(ctx context.Context)
```

Use Indigo for event-stream handling:

```go
import (
    comatproto "github.com/bluesky-social/indigo/api/atproto"
    "github.com/bluesky-social/indigo/events"
    "github.com/bluesky-social/indigo/events/schedulers/sequential"
)
```

Hyperindex already depends on `github.com/bluesky-social/indigo`, and Indigo already has generated CBOR decoders for `com.atproto.label.subscribeLabels` and `com.atproto.label.defs#label`. Do not hand-roll `fxamacker/cbor` decoding for this path.

Behavior:

1. Start one goroutine per configured URL.
2. Read `last_seq` from `label_subscription_state`.
3. Connect to:

   ```txt
   <url>?cursor=<last_seq>
   ```

   Build the URL with `net/url` so existing query parameters are preserved. On first run, explicitly pass `cursor=0`. After dialing, set a WebSocket read limit such as 2-4 MB before handing the connection to Indigo.
4. Create Indigo callbacks and run `events.HandleRepoStream` with a sequential scheduler:

   ```go
   callbacks := &events.RepoStreamCallbacks{
       LabelLabels: func(evt *comatproto.LabelSubscribeLabels_Labels) error {
           // convert evt.Labels to []ExternalLabelInput and persist evt.Seq
           return nil
       },
       RepoInfo: func(evt *comatproto.SyncSubscribeRepos_Info) error {
           // Indigo currently decodes #info as repo info even on label streams.
           // Treat Name == "OutdatedCursor" as labeler cursor state.
           return nil
       },
       Error: func(evt *events.ErrorFrame) error {
           // Handle FutureCursor and other stream errors.
           return nil
       },
   }

   sched := sequential.NewScheduler("labeler:"+subscriptionURL, callbacks.EventHandler)
   err := events.HandleRepoStream(ctx, conn, sched, nil)
   ```

5. For each `#labels` event:
   - read `evt.Seq`
   - convert each `*comatproto.LabelDefs_Label` to repository input with `label_index`
   - treat nil `label.Neg` as `false`
   - base64-encode `label.Sig` into `sig`
   - JSON-marshal the original label into `raw_json`
   - persist labels and update `last_seq` transactionally
6. On disconnect:
   - record error
   - reconnect with exponential backoff
   - use latest stored `last_seq`
7. On `OutdatedCursor` info events:
   - log and record the error
   - do not silently reset to `0` in the first implementation
   - add an explicit reset/backfill policy later if needed
8. On `FutureCursor` error frames:
   - log and record the error
   - reconnect with backoff using the latest stored cursor

## Event-stream decoding with Indigo

Indigo's `events.HandleRepoStream` decodes the ATProto event-stream wire format:

- header DAG-CBOR object: `op`, `t`
- payload DAG-CBOR object: `LabelSubscribeLabels_Labels`, info, or error frame

Useful Indigo types:

```go
comatproto.LabelSubscribeLabels_Labels
comatproto.LabelSubscribeLabels_Info
comatproto.LabelDefs_Label
events.ErrorFrame
```

Caveat: in the current Indigo decoder, `#info` is decoded as `SyncSubscribeRepos_Info` rather than `LabelSubscribeLabels_Info`. The fields are the same (`name`, `message`), so handle labeler info through the `RepoInfo` callback unless Indigo changes this upstream.

## Startup wiring

In `cmd/hyperindex/main.go`:

- Add `externalLabels *repositories.ExternalLabelsRepository` to `services`.
- Add a cancel func to `backgroundServices`, for example `labelerCancel context.CancelFunc`.
- Stop it during graceful shutdown.
- Start subscribers after database and migrations are initialized.
- Start only if config is enabled and URLs are present.

Suggested function:

```go
func startLabelerSubscribers(cfg *config.Config, svc *services, bg *backgroundServices)
```

## Tests

### Repository tests

Add tests for `ExternalLabelsRepository`:

- `EnsureState` creates a state row with `last_seq = 0`.
- `PersistEvent` inserts labels with `label_index` and advances cursor.
- Replaying the same event is idempotent via `(subscription_url, seq, label_index)`.
- Cursor does not advance before label inserts complete.
- `UpdateError` records the latest error.

Use `internal/testutil/db.go` with SQLite in memory.

### Event-stream/subscriber tests

Add tests in `internal/labeler`:

- Build a sample label stream frame using Indigo `events.EventHeader` plus `comatproto.LabelSubscribeLabels_Labels`, then decode it with `events.XRPCStreamEvent.Deserialize` or the subscriber's conversion helper.
- Add an integration-style test with an `httptest` WebSocket server that writes an Indigo-encoded `#labels` frame and assert the subscriber persists it.
- Assert fields:
  - `seq`
  - `label_index`
  - `src`
  - `uri`
  - `val`
  - `neg`
  - `cts`
  - `ver`
  - base64-encoded `sig`
- Add a test for `#info` with `name = "OutdatedCursor"`; because Indigo currently reports this through `RepoInfo`, assert the subscriber still records the error state.

### Config tests

Add tests for:

- comma-separated URL parsing
- whitespace trimming
- disabled config when env is absent or explicitly false

## Verification

For Go changes:

```bash
go build -v ./...
make lint
DATABASE_URL=sqlite::memory: go test -v -race ./...
```

For migration/schema behavior, also run PostgreSQL tests if local Postgres is available:

```bash
DATABASE_URL=postgres://hyperindex:hyperindex@localhost:5432/hyperindex_test?sslmode=disable go test -v -race ./...
```

## Changie

Add an operator-facing Changie fragment.

Affects: `operator`

Suggested summary:

```txt
Add configurable ATProto labeler websocket ingestion that stores external label events locally and resumes from saved cursors.
```

Mention that this ingestion-only slice did not expose external labels in public GraphQL responses by itself.

## Follow-up slices

After ingestion/storage lands:

1. Query active external labels from `external_label`.
2. Expose external labels in public GraphQL subject lookups and record fields without writing remote assertions into the local/admin `label` table.
3. Optionally support `where.externalLabels` record filtering before pagination.
4. Optionally support `atproto-accept-labelers` to let callers choose label sources.
5. Add reconciliation/fallback through `queryLabels` for cases where websocket cursor history is unavailable.
