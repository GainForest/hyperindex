package repositories

import (
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

func TestBuildRequestedSubjectSQLUsesSQLSafeSubjectIndexes(t *testing.T) {
	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })

	subjects := []LabelSubject{
		{URI: "at://did:plc:repo/app.example.record/one", CID: "bafyrecordone"},
		{URI: "at://did:plc:repo/app.example.record/two", CID: "bafyrecordtwo"},
	}

	var params []database.Value
	sqlStr := buildRequestedSubjectSQL(exec, subjects, &params)

	if !strings.Contains(sqlStr, "subject_index") {
		t.Fatalf("requested subject SQL = %q, want subject_index column", sqlStr)
	}
	if strings.Contains(sqlStr, "subject_key") {
		t.Fatalf("requested subject SQL = %q, should not expose Go map keys to SQL", sqlStr)
	}
	if len(params) != 6 {
		t.Fatalf("params = %d, want 6: %#v", len(params), params)
	}
	for i, param := range params {
		if text, ok := param.(database.TextValue); ok && strings.ContainsRune(string(text), '\x00') {
			t.Fatalf("params[%d] contains a NUL byte: %#v", i, param)
		}
	}

	firstIndex, ok := params[0].(database.IntValue)
	if !ok || int64(firstIndex) != 0 {
		t.Fatalf("params[0] = %#v, want subject index 0", params[0])
	}
	secondIndex, ok := params[3].(database.IntValue)
	if !ok || int64(secondIndex) != 1 {
		t.Fatalf("params[3] = %#v, want subject index 1", params[3])
	}
}
