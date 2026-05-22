package repositories_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

type auditTestEnv struct {
	audit   *repositories.AuditRepository
	records *repositories.RecordsRepository
	actors  *repositories.ActorsRepository
	exec    database.Executor
}

func setupAuditTest(t *testing.T) *auditTestEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return &auditTestEnv{
		audit:   repositories.NewAuditRepository(db.Executor),
		records: db.Records,
		actors:  db.Actors,
		exec:    db.Executor,
	}
}

func TestAuditRepository_IngestRecordCreateDuplicateUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	create := auditRecordEvent(101, "create", "rev1", "cid1", `{"text":"first"}`)
	createRaw := []byte(`{"id":101,"type":"record","record":{"live":true,"rev":"rev1","did":"did:plc:alice","collection":"app.example.record","rkey":"one","action":"create","cid":"cid1","record":{"text":"first"}}}`)
	firstResult, err := env.audit.IngestTapEvent(ctx, createRaw, create)
	if err != nil {
		t.Fatalf("IngestTapEvent(create) error = %v", err)
	}
	if !firstResult.Inserted {
		t.Fatal("first create Inserted = false, want true")
	}
	if firstResult.RawEventID == 0 || firstResult.EventID == nil || *firstResult.EventID == 0 {
		t.Fatalf("first create ids not populated: %+v", firstResult)
	}
	if firstResult.EventKey == nil || *firstResult.EventKey != "record:did:plc:alice:rev1:app.example.record:one:create:cid1" {
		t.Fatalf("first create event key = %v", firstResult.EventKey)
	}

	duplicateResult, err := env.audit.IngestTapEvent(ctx, createRaw, create)
	if err != nil {
		t.Fatalf("IngestTapEvent(duplicate create) error = %v", err)
	}
	if duplicateResult.Inserted {
		t.Fatal("duplicate create Inserted = true, want false")
	}
	if duplicateResult.EventID == nil || firstResult.EventID == nil || *duplicateResult.EventID != *firstResult.EventID {
		t.Fatalf("duplicate EventID = %v, want existing %v", duplicateResult.EventID, firstResult.EventID)
	}
	assertRowCount(t, env.exec, "raw_tap_events", 2)
	assertRowCount(t, env.exec, "record_events", 1)

	uri := create.Record.URI()
	record, err := env.records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI(create) error = %v", err)
	}
	if record.CID != "cid1" || record.JSON != `{"text":"first"}` {
		t.Fatalf("current record after create = CID %q JSON %q", record.CID, record.JSON)
	}

	update := auditRecordEvent(102, "update", "rev2", "cid2", `{"text":"second"}`)
	if _, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":102,"type":"record"}`), update); err != nil {
		t.Fatalf("IngestTapEvent(update) error = %v", err)
	}
	record, err = env.records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI(update) error = %v", err)
	}
	if record.CID != "cid2" || record.JSON != `{"text":"second"}` {
		t.Fatalf("current record after update = CID %q JSON %q", record.CID, record.JSON)
	}

	deleteEvent := auditRecordEvent(103, "delete", "rev3", "", "")
	if _, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":103,"type":"record"}`), deleteEvent); err != nil {
		t.Fatalf("IngestTapEvent(delete) error = %v", err)
	}
	if _, err := env.records.GetByURI(ctx, uri); err == nil {
		t.Fatal("GetByURI(delete) error = nil, want missing current record")
	} else if !strings.Contains(err.Error(), sql.ErrNoRows.Error()) {
		t.Fatalf("GetByURI(delete) error = %v, want sql.ErrNoRows", err)
	}
	assertRowCount(t, env.exec, "raw_tap_events", 4)
	assertRowCount(t, env.exec, "record_events", 3)
}

func TestAuditRepository_IngestRecordMissingBodyCIDAndRev(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	event := auditRecordEvent(201, "create", "", "", "")
	result, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":201,"type":"record","record":{"did":"did:plc:alice"}}`), event)
	if err != nil {
		t.Fatalf("IngestTapEvent(missing fields tolerated) error = %v", err)
	}
	if !result.Inserted {
		t.Fatal("missing-body create Inserted = false, want true")
	}
	if result.EventKey == nil || !strings.HasPrefix(*result.EventKey, "record:fallback:201:") {
		t.Fatalf("fallback event key = %v, want record:fallback prefix", result.EventKey)
	}
	assertRowCount(t, env.exec, "raw_tap_events", 1)
	assertRowCount(t, env.exec, "record_events", 1)

	if _, err := env.records.GetByURI(ctx, event.Record.URI()); err == nil {
		t.Fatal("missing body created current record, want audit-only row")
	}

	emptyCID := ""
	page, err := env.audit.FindRecordEvents(ctx, repositories.RecordEventFindOptions{
		First: 10,
		Where: repositories.AuditRecordEventFilters{CID: &emptyCID},
	})
	if err != nil {
		t.Fatalf("FindRecordEvents(CID empty) error = %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].CID != nil || page.Events[0].Record != nil || page.Events[0].Rev != "" {
		t.Fatalf("empty CID/rev query page = %+v", page.Events)
	}
}

func TestAuditRepository_RollsBackRawEventWhenDecodedRecordInsertFails(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	invalid := auditRecordEvent(250, "invalid", "rev-bad", "cid-bad", `{"text":"bad"}`)
	if _, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":250,"type":"record"}`), invalid); err == nil {
		t.Fatal("IngestTapEvent(invalid action) error = nil, want failure")
	}

	assertRowCount(t, env.exec, "raw_tap_events", 0)
	assertRowCount(t, env.exec, "record_events", 0)
	if _, err := env.records.GetByURI(ctx, invalid.Record.URI()); err == nil {
		t.Fatal("invalid record event updated current state, want rollback")
	}
}

func TestAuditRepository_DuplicateRecordEventDoesNotOverwriteCurrentState(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	first := auditRecordEvent(260, "create", "rev-immutable", "cid-immutable", `{"text":"first"}`)
	if _, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":260,"type":"record"}`), first); err != nil {
		t.Fatalf("IngestTapEvent(first) error = %v", err)
	}

	duplicate := auditRecordEvent(261, "create", "rev-immutable", "cid-immutable", `{"text":"duplicate should not win"}`)
	result, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":261,"type":"record"}`), duplicate)
	if err != nil {
		t.Fatalf("IngestTapEvent(duplicate with changed body) error = %v", err)
	}
	if result.Inserted {
		t.Fatal("duplicate Inserted = true, want false")
	}

	record, err := env.records.GetByURI(ctx, first.Record.URI())
	if err != nil {
		t.Fatalf("GetByURI(after duplicate) error = %v", err)
	}
	if record.CID != "cid-immutable" || record.JSON != `{"text":"first"}` {
		t.Fatalf("current record after duplicate = CID %q JSON %q, want original", record.CID, record.JSON)
	}
	assertRowCount(t, env.exec, "raw_tap_events", 2)
	assertRowCount(t, env.exec, "record_events", 1)
}

func TestAuditRepository_IdentityIsActiveMissingAndExplicitFalseRemainDistinct(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	missing := &repositories.AuditTapEvent{
		ID:   270,
		Type: "identity",
		Identity: &repositories.AuditTapIdentityEvent{
			DID:    "did:plc:alice",
			Handle: "alice.example.com",
			Status: "active",
		},
	}
	missingResult, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":270,"type":"identity"}`), missing)
	if err != nil {
		t.Fatalf("IngestTapEvent(identity missing is_active) error = %v", err)
	}
	if missingResult.EventKey == nil || *missingResult.EventKey != "identity:270:did:plc:alice:alice.example.com::active" {
		t.Fatalf("missing is_active event key = %v", missingResult.EventKey)
	}

	explicitFalse := &repositories.AuditTapEvent{
		ID:   271,
		Type: "identity",
		Identity: &repositories.AuditTapIdentityEvent{
			DID:             "did:plc:alice",
			Handle:          "alice2.example.com",
			IsActive:        false,
			IsActivePresent: true,
			Status:          "active",
		},
	}
	falseResult, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":271,"type":"identity"}`), explicitFalse)
	if err != nil {
		t.Fatalf("IngestTapEvent(identity explicit false) error = %v", err)
	}
	if falseResult.EventKey == nil || *falseResult.EventKey != "identity:271:did:plc:alice:alice2.example.com:false:active" {
		t.Fatalf("explicit false event key = %v", falseResult.EventKey)
	}

	var missingNullCount int64
	if err := env.exec.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM identity_events WHERE tap_delivery_id = %s AND is_active IS NULL", env.exec.Placeholder(1)),
		[]database.Value{database.Int(270)},
		&missingNullCount,
	); err != nil {
		t.Fatalf("count missing is_active row: %v", err)
	}
	if missingNullCount != 1 {
		t.Fatalf("missing is_active NULL rows = %d, want 1", missingNullCount)
	}

	var explicitFalseCount int64
	if err := env.exec.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM identity_events WHERE tap_delivery_id = %s AND is_active = %s", env.exec.Placeholder(1), env.exec.Placeholder(2)),
		[]database.Value{database.Int(271), database.Bool(false)},
		&explicitFalseCount,
	); err != nil {
		t.Fatalf("count explicit false is_active row: %v", err)
	}
	if explicitFalseCount != 1 {
		t.Fatalf("explicit false is_active rows = %d, want 1", explicitFalseCount)
	}

	actor, err := env.actors.GetByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("GetByDID(after explicit false active identity) error = %v", err)
	}
	if actor.Handle != "alice2.example.com" {
		t.Fatalf("actor handle = %q, want explicit false active identity to update actor", actor.Handle)
	}
}

func TestAuditRepository_IngestIdentityUpdatesAndPurgesCurrentState(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	active := &repositories.AuditTapEvent{
		ID:   301,
		Type: "identity",
		Identity: &repositories.AuditTapIdentityEvent{
			DID:             "did:plc:alice",
			Handle:          "alice.example.com",
			IsActive:        true,
			IsActivePresent: true,
			Status:          "active",
		},
	}
	result, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":301,"type":"identity"}`), active)
	if err != nil {
		t.Fatalf("IngestTapEvent(active identity) error = %v", err)
	}
	if !result.Inserted || result.EventKey == nil || *result.EventKey != "identity:301:did:plc:alice:alice.example.com:true:active" {
		t.Fatalf("active identity result = %+v", result)
	}
	actor, err := env.actors.GetByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("GetByDID(active identity) error = %v", err)
	}
	if actor.Handle != "alice.example.com" {
		t.Fatalf("actor handle = %q, want alice.example.com", actor.Handle)
	}

	create := auditRecordEvent(302, "create", "rev1", "cid1", `{"text":"kept until purge"}`)
	if _, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":302,"type":"record"}`), create); err != nil {
		t.Fatalf("IngestTapEvent(record before purge) error = %v", err)
	}

	purge := &repositories.AuditTapEvent{
		ID:   303,
		Type: "identity",
		Identity: &repositories.AuditTapIdentityEvent{
			DID:             "did:plc:alice",
			Handle:          "alice.example.com",
			IsActive:        false,
			IsActivePresent: true,
			Status:          "deleted",
		},
	}
	if _, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":303,"type":"identity"}`), purge); err != nil {
		t.Fatalf("IngestTapEvent(purge identity) error = %v", err)
	}
	if _, err := env.records.GetByURI(ctx, create.Record.URI()); err == nil {
		t.Fatal("purged identity left current record behind")
	}
	if _, err := env.actors.GetByDID(ctx, "did:plc:alice"); err == nil {
		t.Fatal("purged identity left current actor behind")
	}
	assertRowCount(t, env.exec, "identity_events", 2)
	assertRowCount(t, env.exec, "record_events", 1)
}

func TestAuditRepository_FindRecordEventsFiltersAndCursorPagination(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	events := []*repositories.AuditTapEvent{
		auditRecordEvent(401, "create", "rev1", "cid1", `{"text":"one"}`),
		auditRecordEvent(402, "update", "rev2", "cid2", `{"text":"two"}`),
		auditRecordEvent(403, "delete", "rev3", "", ""),
	}
	for _, event := range events {
		if _, err := env.audit.IngestTapEvent(ctx, []byte(fmt.Sprintf(`{"id":%d,"type":"record"}`, event.ID)), event); err != nil {
			t.Fatalf("IngestTapEvent(%d) error = %v", event.ID, err)
		}
	}

	did := "did:plc:alice"
	collection := "app.example.record"
	total, err := env.audit.CountRecordEvents(ctx, repositories.AuditRecordEventFilters{
		DID:        &did,
		Collection: &collection,
	})
	if err != nil {
		t.Fatalf("CountRecordEvents(did+collection) error = %v", err)
	}
	if total != 3 {
		t.Fatalf("CountRecordEvents(did+collection) = %d, want 3", total)
	}

	firstPage, err := env.audit.FindRecordEvents(ctx, repositories.RecordEventFindOptions{
		First: 1,
		Where: repositories.AuditRecordEventFilters{
			DID:        &did,
			Collection: &collection,
		},
		OrderBy: repositories.AuditRecordEventOrder{Direction: "ASC"},
	})
	if err != nil {
		t.Fatalf("FindRecordEvents(first page) error = %v", err)
	}
	if len(firstPage.Events) != 1 || firstPage.Events[0].Action != "create" || !firstPage.HasNextPage || firstPage.EndCursor == nil {
		t.Fatalf("first page = %+v", firstPage)
	}

	secondPage, err := env.audit.FindRecordEvents(ctx, repositories.RecordEventFindOptions{
		First: 1,
		After: *firstPage.EndCursor,
		Where: repositories.AuditRecordEventFilters{
			DID:        &did,
			Collection: &collection,
		},
		OrderBy: repositories.AuditRecordEventOrder{Direction: "ASC"},
	})
	if err != nil {
		t.Fatalf("FindRecordEvents(second page) error = %v", err)
	}
	if len(secondPage.Events) != 1 || secondPage.Events[0].Action != "update" || !secondPage.HasPreviousPage {
		t.Fatalf("second page = %+v", secondPage)
	}

	action := "delete"
	deletePage, err := env.audit.FindRecordEvents(ctx, repositories.RecordEventFindOptions{
		First: 10,
		Where: repositories.AuditRecordEventFilters{Action: &action},
	})
	if err != nil {
		t.Fatalf("FindRecordEvents(action delete) error = %v", err)
	}
	if len(deletePage.Events) != 1 || deletePage.Events[0].Action != "delete" {
		t.Fatalf("delete filter page = %+v", deletePage.Events)
	}
}

func TestAuditRepository_FindRecordEventsReceivedAtFiltersNormalizeSQLiteTimestamps(t *testing.T) {
	ctx := context.Background()
	env := setupAuditTest(t)

	event := auditRecordEvent(501, "create", "rev-time", "cid-time", `{"text":"time"}`)
	if _, err := env.audit.IngestTapEvent(ctx, []byte(`{"id":501,"type":"record"}`), event); err != nil {
		t.Fatalf("IngestTapEvent(time filter fixture) error = %v", err)
	}

	var stored string
	if err := env.exec.QueryRow(ctx,
		fmt.Sprintf("SELECT received_at FROM record_events WHERE tap_delivery_id = %s", env.exec.Placeholder(1)),
		[]database.Value{database.Int(501)},
		&stored,
	); err != nil {
		t.Fatalf("read stored received_at: %v", err)
	}

	storedTime, err := time.Parse("2006-01-02 15:04:05", stored)
	if err != nil {
		t.Fatalf("parse SQLite received_at %q: %v", stored, err)
	}
	lower := storedTime.Add(-time.Second).UTC().Format(time.RFC3339)
	upper := storedTime.Add(time.Second).UTC().Format(time.RFC3339)

	page, err := env.audit.FindRecordEvents(ctx, repositories.RecordEventFindOptions{
		First: 10,
		Where: repositories.AuditRecordEventFilters{
			ReceivedAtAfter:  &lower,
			ReceivedAtBefore: &upper,
		},
	})
	if err != nil {
		t.Fatalf("FindRecordEvents(receivedAt range) error = %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].TapDeliveryID != 501 {
		t.Fatalf("receivedAt range page = %+v, want delivery 501", page.Events)
	}
}

func auditRecordEvent(id int64, action, rev, cid, recordBody string) *repositories.AuditTapEvent {
	var record []byte
	if recordBody != "" {
		record = []byte(recordBody)
	}
	return &repositories.AuditTapEvent{
		ID:   id,
		Type: "record",
		Record: &repositories.AuditTapRecordEvent{
			Live:       true,
			Rev:        rev,
			DID:        "did:plc:alice",
			Collection: "app.example.record",
			RKey:       "one",
			Action:     action,
			CID:        cid,
			Record:     record,
		},
	}
}

func assertRowCount(t *testing.T, exec database.Executor, table string, want int64) {
	t.Helper()
	var got int64
	if err := exec.QueryRow(context.Background(), fmt.Sprintf("SELECT COUNT(*) FROM %s", table), nil, &got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("count %s = %d, want %d", table, got, want)
	}
}
