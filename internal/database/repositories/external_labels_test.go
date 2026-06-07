package repositories_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestExternalLabelsRepositoryEnsureState(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()

	state, err := repo.EnsureState(ctx, "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels")
	if err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	if state.URL != "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels" {
		t.Fatalf("URL = %q", state.URL)
	}
	if state.LastSeq != 0 {
		t.Fatalf("LastSeq = %d, want 0", state.LastSeq)
	}
	if state.Status() != repositories.LabelSubscriptionStatusPending {
		t.Fatalf("Status() = %q, want %q", state.Status(), repositories.LabelSubscriptionStatusPending)
	}
	if state.LabelerDID != nil {
		t.Fatalf("LabelerDID = %q, want nil", *state.LabelerDID)
	}
	if state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		t.Fatalf("created/updated timestamps should be set: %+v", state)
	}
}

func TestExternalLabelsRepositoryPersistEvent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	cid := "bafyreiabc"
	exp := "2026-01-02T03:04:05Z"
	sig := "AQID"
	ver := int64(1)

	labels := []repositories.ExternalLabelInput{
		{
			LabelIndex: 0,
			Src:        "did:plc:labeler",
			URI:        "at://did:plc:repo/app.example.record/one",
			CID:        &cid,
			Val:        "high-quality",
			Neg:        true,
			Cts:        "2025-01-02T03:04:05.123456789Z",
			Exp:        &exp,
			Sig:        &sig,
			Ver:        &ver,
			RawJSON:    `{"src":"did:plc:labeler","uri":"at://did:plc:repo/app.example.record/one","val":"high-quality","cts":"2025-01-02T03:04:05.123456789Z"}`,
		},
		{
			LabelIndex: 1,
			Src:        "did:plc:labeler",
			URI:        "at://did:plc:repo/app.example.record/two",
			Val:        "standard",
			Cts:        "2025-01-02T03:05:05Z",
			RawJSON:    `{"src":"did:plc:labeler","uri":"at://did:plc:repo/app.example.record/two","val":"standard","cts":"2025-01-02T03:05:05Z"}`,
		},
	}

	if err := repo.PersistEvent(ctx, url, 42, labels); err != nil {
		t.Fatalf("PersistEvent() error = %v", err)
	}

	state, err := repo.GetState(ctx, url)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.LastSeq != 42 {
		t.Fatalf("LastSeq = %d, want 42", state.LastSeq)
	}
	if state.Status() != repositories.LabelSubscriptionStatusConnected {
		t.Fatalf("Status() = %q, want %q", state.Status(), repositories.LabelSubscriptionStatusConnected)
	}
	if state.LastError != nil {
		t.Fatalf("successful event should clear error state: %+v", state)
	}
	if state.LastEventAt == nil || state.LastEventAt.IsZero() {
		t.Fatalf("LastEventAt should be set: %+v", state)
	}

	stored, err := repo.ListLabelsByEvent(ctx, url, 42)
	if err != nil {
		t.Fatalf("ListLabelsByEvent() error = %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored labels = %d, want 2", len(stored))
	}

	first := stored[0]
	if first.Seq != 42 || first.LabelIndex != 0 || first.Src != "did:plc:labeler" || first.Val != "high-quality" {
		t.Fatalf("unexpected first label: %+v", first)
	}
	if first.CID == nil || *first.CID != cid {
		t.Fatalf("CID = %v, want %q", first.CID, cid)
	}
	if !first.Neg {
		t.Fatalf("Neg = false, want true")
	}
	if first.Exp == nil || *first.Exp != exp {
		t.Fatalf("Exp = %v, want %q", first.Exp, exp)
	}
	if first.Sig == nil || *first.Sig != sig {
		t.Fatalf("Sig = %v, want %q", first.Sig, sig)
	}
	if first.Ver == nil || *first.Ver != ver {
		t.Fatalf("Ver = %v, want %d", first.Ver, ver)
	}
	if !strings.Contains(first.RawJSON, "high-quality") {
		t.Fatalf("RawJSON = %q, want high-quality", first.RawJSON)
	}
}

func TestExternalLabelsRepositoryReplayIsIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	labels := []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: "at://did:plc:repo/app.example.record/one", Val: "standard", Cts: "2025-01-02T03:04:05Z", RawJSON: `{}`},
	}

	if err := repo.PersistEvent(ctx, url, 10, labels); err != nil {
		t.Fatalf("first PersistEvent() error = %v", err)
	}
	if err := repo.PersistEvent(ctx, url, 10, labels); err != nil {
		t.Fatalf("replay PersistEvent() error = %v", err)
	}

	count, err := repo.CountLabels(ctx, url)
	if err != nil {
		t.Fatalf("CountLabels() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountLabels() = %d, want 1", count)
	}

	if err := repo.PersistEvent(ctx, url, 9, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: "at://did:plc:repo/app.example.record/old", Val: "draft", Cts: "2025-01-02T03:03:05Z", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("out-of-order PersistEvent() error = %v", err)
	}

	state, err := repo.GetState(ctx, url)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.LastSeq != 10 {
		t.Fatalf("LastSeq = %d, want 10", state.LastSeq)
	}
}

func TestExternalLabelsRepositoryZeroLabelEventAdvancesCursor(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"

	if err := repo.PersistEvent(ctx, url, 5, nil); err != nil {
		t.Fatalf("PersistEvent() error = %v", err)
	}

	state, err := repo.GetState(ctx, url)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.LastSeq != 5 {
		t.Fatalf("LastSeq = %d, want 5", state.LastSeq)
	}

	count, err := repo.CountLabels(ctx, url)
	if err != nil {
		t.Fatalf("CountLabels() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountLabels() = %d, want 0", count)
	}
}

func TestExternalLabelsRepositoryDoesNotAdvanceCursorOnInvalidLabel(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"

	if _, err := repo.EnsureState(ctx, url); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	err := repo.PersistEvent(ctx, url, 99, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: "at://did:plc:repo/app.example.record/one", Val: "standard", Cts: "not-a-timestamp", RawJSON: `{}`},
	})
	if err == nil {
		t.Fatal("PersistEvent() error = nil, want invalid timestamp error")
	}

	state, err := repo.GetState(ctx, url)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.LastSeq != 0 {
		t.Fatalf("LastSeq = %d, want 0", state.LastSeq)
	}

	count, err := repo.CountLabels(ctx, url)
	if err != nil {
		t.Fatalf("CountLabels() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountLabels() = %d, want 0", count)
	}
}

func TestExternalLabelsRepositoryUpdateError(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"

	if err := repo.UpdateError(ctx, url, "temporary websocket failure"); err != nil {
		t.Fatalf("UpdateError() error = %v", err)
	}

	state, err := repo.GetState(ctx, url)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.Status() != repositories.LabelSubscriptionStatusError {
		t.Fatalf("Status() = %q, want %q", state.Status(), repositories.LabelSubscriptionStatusError)
	}
	if state.LastError == nil || *state.LastError != "temporary websocket failure" {
		t.Fatalf("LastError = %v, want temporary websocket failure", state.LastError)
	}
	if state.IsFatal() {
		t.Fatalf("retryable error should not be fatal: %+v", state)
	}
}

func TestExternalLabelsRepositoryMarkFatalCursor(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"

	if err := repo.UpdateLastSeq(ctx, url, 56428); err != nil {
		t.Fatalf("UpdateLastSeq() error = %v", err)
	}
	if err := repo.MarkFatalCursor(ctx, url, "FutureCursor", "Cursor is in the future. Reset subscription cursor and replay labels."); err != nil {
		t.Fatalf("MarkFatalCursor() error = %v", err)
	}

	state, err := repo.GetState(ctx, url)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !state.IsFatal() {
		t.Fatalf("IsFatal() = false, want true: %+v", state)
	}
	if state.Status() != repositories.LabelSubscriptionStatusFatal {
		t.Fatalf("Status() = %q, want %q", state.Status(), repositories.LabelSubscriptionStatusFatal)
	}
	if state.LastSeq != 56428 {
		t.Fatalf("LastSeq = %d, want 56428", state.LastSeq)
	}
	if got := repositories.FatalCursorCode(state.LastError); got != "FutureCursor" {
		t.Fatalf("FatalCursorCode() = %q, want FutureCursor", got)
	}
	if state.LastError == nil || !strings.HasPrefix(*state.LastError, "FATAL_CURSOR FutureCursor:") || !strings.Contains(*state.LastError, "Reset subscription cursor and replay labels") {
		t.Fatalf("LastError = %v, want fatal reset marker", state.LastError)
	}
}

func TestExternalLabelsRepositoryListStatesIncludesConfiguredMissingURLs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	storedURL := "wss://stored.example/xrpc/com.atproto.label.subscribeLabels"
	missingURL := "wss://missing.example/xrpc/com.atproto.label.subscribeLabels"

	if err := repo.MarkFatalCursor(ctx, storedURL, "OutdatedCursor", "Cursor is too old. Reset subscription cursor and replay labels."); err != nil {
		t.Fatalf("MarkFatalCursor() error = %v", err)
	}

	states, err := repo.ListStates(ctx, []string{missingURL, storedURL})
	if err != nil {
		t.Fatalf("ListStates() error = %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("ListStates() len = %d, want 2", len(states))
	}
	if states[0].URL != missingURL || states[0].Status() != repositories.LabelSubscriptionStatusPending {
		t.Fatalf("missing URL state = %+v, want pending %q", states[0], missingURL)
	}
	if states[1].URL != storedURL || !states[1].IsFatal() {
		t.Fatalf("stored URL state = %+v, want fatal %q", states[1], storedURL)
	}
}

func TestExternalLabelsRepositoryFatalStateIsStickyUntilExplicitReset(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"

	if err := repo.UpdateLastSeq(ctx, url, 56428); err != nil {
		t.Fatalf("UpdateLastSeq() error = %v", err)
	}
	if err := repo.MarkFatalCursor(ctx, url, "FutureCursor", "Cursor is in the future. Reset subscription cursor and replay labels."); err != nil {
		t.Fatalf("MarkFatalCursor() error = %v", err)
	}

	if err := repo.UpdateConnected(ctx, url); err != nil {
		t.Fatalf("UpdateConnected() error = %v", err)
	}
	if err := repo.UpdateError(ctx, url, "temporary websocket timeout"); err != nil {
		t.Fatalf("UpdateError() error = %v", err)
	}
	if err := repo.UpdateLastSeq(ctx, url, 56429); err != nil {
		t.Fatalf("UpdateLastSeq() error = %v", err)
	}

	state, err := repo.GetState(ctx, url)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !state.IsFatal() {
		t.Fatalf("IsFatal() = false, want true after non-fatal updates: %+v", state)
	}
	if state.LastSeq != 56428 {
		t.Fatalf("LastSeq = %d, want fatal cursor preserved at 56428", state.LastSeq)
	}
	if got := repositories.FatalCursorCode(state.LastError); got != "FutureCursor" {
		t.Fatalf("FatalCursorCode() = %q, want FutureCursor", got)
	}
}

func TestExternalLabelsRepositoryGetBySubjectsReturnsBatchedActiveLabels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()

	recordOneURI := "at://did:plc:repo/app.example.record/one"
	recordOneCID := "bafyrecordone"
	recordTwoURI := "at://did:plc:repo/app.example.record/two"
	recordTwoCID := "bafyrecordtwo"
	accountDID := "did:plc:account"

	persistExternalLabelEvent(t, repo, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler-a", URI: recordOneURI, CID: &recordOneCID, Val: "high-quality", Cts: "2025-01-02T03:04:05Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:labeler-a", URI: accountDID, Val: "high-quality", Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:labeler-b", URI: recordTwoURI, CID: &recordTwoCID, Val: "high-quality", Cts: "2025-01-02T03:06:05Z", RawJSON: `{}`},
		{LabelIndex: 3, Src: "did:plc:labeler-a", URI: recordOneURI, CID: &recordOneCID, Val: "spam", Cts: "2025-01-02T03:07:05Z", RawJSON: `{}`},
	})

	recordOneSubject := repositories.LabelSubject{URI: recordOneURI, CID: recordOneCID}
	recordTwoSubject := repositories.LabelSubject{URI: recordTwoURI, CID: recordTwoCID}
	accountSubject := repositories.LabelSubject{URI: accountDID}
	labelsBySubject, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{
		recordOneSubject,
		recordTwoSubject,
		accountSubject,
	}, repositories.ExternalLabelFilter{
		Sources:    []string{"did:plc:labeler-a"},
		Values:     []string{"high-quality"},
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("GetBySubjects() error = %v", err)
	}

	assertExternalLabelVals(t, labelsBySubject[recordOneSubject.Key()], []string{"high-quality"})
	assertExternalLabelVals(t, labelsBySubject[accountSubject.Key()], []string{"high-quality"})
	assertExternalLabelVals(t, labelsBySubject[recordTwoSubject.Key()], nil)
}

func TestExternalLabelsRepositoryGetBySubjectsBatchesLargeSubjectSets(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()

	const subjectCount = 1000
	subjects := make([]repositories.LabelSubject, 0, subjectCount)
	labelInputs := []repositories.ExternalLabelInput{}
	filterValues := make([]string, 0, 5)
	labeledIndexes := map[int]string{
		0:   "first-batch-start",
		249: "first-batch-end",
		250: "second-batch-start",
		501: "third-batch",
		999: "last-subject",
	}

	for i := range subjectCount {
		uri := fmt.Sprintf("at://did:plc:repo/app.example.record/%04d", i)
		subjects = append(subjects, repositories.LabelSubject{URI: uri})
		if val, ok := labeledIndexes[i]; ok {
			filterValues = append(filterValues, val)
			labelInputs = append(labelInputs, repositories.ExternalLabelInput{
				LabelIndex: int64(len(labelInputs)),
				Src:        "did:plc:labeler",
				URI:        uri,
				Val:        val,
				Cts:        fmt.Sprintf("2025-01-02T03:04:%02dZ", len(labelInputs)),
				RawJSON:    `{}`,
			})
			labelInputs = append(labelInputs, repositories.ExternalLabelInput{
				LabelIndex: int64(len(labelInputs)),
				Src:        "did:plc:other-labeler",
				URI:        uri,
				Val:        "excluded",
				Cts:        fmt.Sprintf("2025-01-02T03:04:%02dZ", len(labelInputs)),
				RawJSON:    `{}`,
			})
		}
	}

	persistExternalLabelEvent(t, repo, 1, labelInputs)

	labelsBySubject, err := repo.GetBySubjects(ctx, subjects, repositories.ExternalLabelFilter{
		Sources:    []string{"did:plc:labeler"},
		Values:     filterValues,
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("GetBySubjects() error = %v", err)
	}
	if len(labelsBySubject) != subjectCount {
		t.Fatalf("labelsBySubject len = %d, want %d", len(labelsBySubject), subjectCount)
	}

	labelledSubjectCount := 0
	for _, labels := range labelsBySubject {
		if len(labels) > 0 {
			labelledSubjectCount++
		}
	}
	if labelledSubjectCount != len(labeledIndexes) {
		t.Fatalf("labelled subject count = %d, want %d", labelledSubjectCount, len(labeledIndexes))
	}
	for index, val := range labeledIndexes {
		assertExternalLabelVals(t, labelsBySubject[subjects[index].Key()], []string{val})
	}
	assertExternalLabelVals(t, labelsBySubject[subjects[123].Key()], nil)
}

func TestExternalLabelsRepositoryGetBySubjectsActiveExcludesLatestNegation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	uri := "at://did:plc:repo/app.example.record/negated"
	subject := repositories.LabelSubject{URI: uri}

	persistExternalLabelEvent(t, repo, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, Val: "high-quality", Cts: "2025-01-02T03:04:05Z", RawJSON: `{}`},
	})
	persistExternalLabelEvent(t, repo, 2, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, Val: "high-quality", Neg: true, Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
	})

	active, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{subject}, repositories.ExternalLabelFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("GetBySubjects(active) error = %v", err)
	}
	assertExternalLabelVals(t, active[subject.Key()], nil)

	history, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{subject}, repositories.ExternalLabelFilter{ActiveOnly: false})
	if err != nil {
		t.Fatalf("GetBySubjects(history) error = %v", err)
	}
	labels := history[subject.Key()]
	if len(labels) != 2 {
		t.Fatalf("history labels = %d, want 2: %+v", len(labels), labels)
	}
	if !labels[0].Neg || labels[1].Neg {
		t.Fatalf("history order/negation = [%v, %v], want latest negation first then positive", labels[0].Neg, labels[1].Neg)
	}
}

func TestExternalLabelsRepositoryGetBySubjectsActiveExcludesExpiredLabels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	uri := "at://did:plc:repo/app.example.record/expired"
	subject := repositories.LabelSubject{URI: uri}
	expiredAt := "2000-01-02T03:04:05Z"

	persistExternalLabelEvent(t, repo, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, Val: "temporary", Cts: "2025-01-02T03:04:05Z", Exp: &expiredAt, RawJSON: `{}`},
	})

	active, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{subject}, repositories.ExternalLabelFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("GetBySubjects(active) error = %v", err)
	}
	assertExternalLabelVals(t, active[subject.Key()], nil)

	history, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{subject}, repositories.ExternalLabelFilter{ActiveOnly: false})
	if err != nil {
		t.Fatalf("GetBySubjects(history) error = %v", err)
	}
	assertExternalLabelVals(t, history[subject.Key()], []string{"temporary"})
}

func TestExternalLabelsRepositoryGetBySubjectsCIDApplicability(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	uri := "at://did:plc:repo/app.example.record/cid-specific"
	matchingCID := "bafymatching"
	otherCID := "bafyother"

	persistExternalLabelEvent(t, repo, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, CID: &matchingCID, Val: "high-quality", Cts: "2025-01-02T03:04:05Z", RawJSON: `{}`},
	})

	matchingSubject := repositories.LabelSubject{URI: uri, CID: matchingCID}
	otherSubject := repositories.LabelSubject{URI: uri, CID: otherCID}
	uriOnlySubject := repositories.LabelSubject{URI: uri}
	labelsBySubject, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{
		matchingSubject,
		otherSubject,
		uriOnlySubject,
	}, repositories.ExternalLabelFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("GetBySubjects() error = %v", err)
	}

	assertExternalLabelVals(t, labelsBySubject[matchingSubject.Key()], []string{"high-quality"})
	assertExternalLabelVals(t, labelsBySubject[otherSubject.Key()], nil)
	assertExternalLabelVals(t, labelsBySubject[uriOnlySubject.Key()], []string{"high-quality"})
}

func TestExternalLabelsRepositoryGetBySubjectsActiveSupersessionScopedByCID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	uri := "at://did:plc:repo/app.example.record/cid-supersession"
	cidOne := "bafyone"
	cidTwo := "bafytwo"

	persistExternalLabelEvent(t, repo, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, CID: &cidOne, Val: "high-quality", Cts: "2025-01-02T03:04:05Z", RawJSON: `{}`},
	})
	persistExternalLabelEvent(t, repo, 2, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, CID: &cidTwo, Val: "high-quality", Neg: true, Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
	})

	cidOneSubject := repositories.LabelSubject{URI: uri, CID: cidOne}
	cidTwoSubject := repositories.LabelSubject{URI: uri, CID: cidTwo}
	active, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{cidOneSubject, cidTwoSubject}, repositories.ExternalLabelFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("GetBySubjects(active) error = %v", err)
	}

	assertExternalLabelVals(t, active[cidOneSubject.Key()], []string{"high-quality"})
	assertExternalLabelVals(t, active[cidTwoSubject.Key()], nil)
}

func TestExternalLabelsRepositoryGetBySubjectsActiveTieBreaksByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.ExternalLabels
	ctx := context.Background()
	uri := "at://did:plc:repo/app.example.record/tie"
	subject := repositories.LabelSubject{URI: uri}
	cts := "2025-01-02T03:04:05Z"

	persistExternalLabelEvent(t, repo, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, Val: "high-quality", Neg: true, Cts: cts, RawJSON: `{}`},
	})
	persistExternalLabelEvent(t, repo, 2, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: uri, Val: "high-quality", Cts: cts, RawJSON: `{}`},
	})

	active, err := repo.GetBySubjects(ctx, []repositories.LabelSubject{subject}, repositories.ExternalLabelFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("GetBySubjects() error = %v", err)
	}
	labels := active[subject.Key()]
	if len(labels) != 1 {
		t.Fatalf("active labels = %d, want 1: %+v", len(labels), labels)
	}
	if labels[0].Neg || labels[0].Val != "high-quality" {
		t.Fatalf("active label = %+v, want non-negated high-quality", labels[0])
	}
}

const testExternalLabelSubscriptionURL = "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"

func persistExternalLabelEvent(t *testing.T, repo *repositories.ExternalLabelsRepository, seq int64, labels []repositories.ExternalLabelInput) {
	t.Helper()
	if err := repo.PersistEvent(context.Background(), testExternalLabelSubscriptionURL, seq, labels); err != nil {
		t.Fatalf("PersistEvent(seq=%d) error = %v", seq, err)
	}
}

func assertExternalLabelVals(t *testing.T, labels []repositories.ExternalLabel, want []string) {
	t.Helper()
	if len(labels) != len(want) {
		t.Fatalf("labels = %d, want %d: %+v", len(labels), len(want), labels)
	}
	for i := range want {
		if labels[i].Val != want[i] {
			t.Fatalf("label[%d].Val = %q, want %q; labels=%+v", i, labels[i].Val, want[i], labels)
		}
	}
}
