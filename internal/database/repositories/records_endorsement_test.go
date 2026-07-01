package repositories_test

import (
	"context"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestEndorsementAdjacencyForSQLite(t *testing.T) {
	db := testutil.SetupTestDB(t)
	runEndorsementAdjacencyCore(t, db.Records, testSuffix(t))
}

func TestEndorsementAdjacencyForPostgres(t *testing.T) {
	repo, ok := setupPostgresRecordsRepository(t)
	if !ok {
		t.Skip("PostgreSQL endorsement adjacency test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	runEndorsementAdjacencyCore(t, repo, testSuffix(t))
}

func TestEndorsementAdjacencyForRequiresDIDSubjectSQLite(t *testing.T) {
	db := testutil.SetupTestDB(t)
	runEndorsementAdjacencyRequiresDIDSubject(t, db.Records, testSuffix(t))
}

func TestEndorsementAdjacencyForRequiresDIDSubjectPostgres(t *testing.T) {
	repo, ok := setupPostgresRecordsRepository(t)
	if !ok {
		t.Skip("PostgreSQL endorsement adjacency subject test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	runEndorsementAdjacencyRequiresDIDSubject(t, repo, testSuffix(t))
}

func TestEndorsementAdjacencyForAllowedIssuersSQLite(t *testing.T) {
	db := testutil.SetupTestDB(t)
	runEndorsementAdjacencyAllowedIssuers(t, db.Records, testSuffix(t))
}

func TestEndorsementAdjacencyForAllowedIssuersPostgres(t *testing.T) {
	repo, ok := setupPostgresRecordsRepository(t)
	if !ok {
		t.Skip("PostgreSQL endorsement adjacency allowed issuer test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	runEndorsementAdjacencyAllowedIssuers(t, repo, testSuffix(t))
}

func runEndorsementAdjacencyCore(t *testing.T, repo *repositories.RecordsRepository, suffix string) {
	t.Helper()
	ctx := context.Background()

	endorsementBadgeURI := "at://did:plc:issuer/app.certified.badge.definition/endorsement-" + suffix
	verificationBadgeURI := "at://did:plc:issuer/app.certified.badge.definition/verification-" + suffix
	insertRecord(t, repo, endorsementBadgeURI, "cid-endorsement-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Endorsement","badgeType":"endorsement"}`)
	insertRecord(t, repo, verificationBadgeURI, "cid-verification-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Verification","badgeType":"verification"}`)

	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/alice-"+suffix, "cid-award-alice-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`","cid":"cid-endorsement"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:alice"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/bob-"+suffix, "cid-award-bob-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+verificationBadgeURI+`","cid":"cid-verification"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:bob"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/self-"+suffix, "cid-award-self-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`","cid":"cid-endorsement"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:issuer"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/rejected-"+suffix, "cid-award-rejected-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`","cid":"cid-endorsement"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:rejected"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/accepted-"+suffix, "cid-award-accepted-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`","cid":"cid-endorsement"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:zara"}}`)
	insertRecord(t, repo, "at://did:plc:rejected/app.certified.badge.response/reject-"+suffix, "cid-response-reject-"+suffix, "did:plc:rejected", "app.certified.badge.response", `{"badgeAward":{"uri":"at://did:plc:issuer/app.certified.badge.award/rejected-`+suffix+`","cid":"cid-award-rejected"},"response":"rejected"}`)
	insertRecord(t, repo, "at://did:plc:zara/app.certified.badge.response/accept-"+suffix, "cid-response-accept-"+suffix, "did:plc:zara", "app.certified.badge.response", `{"badgeAward":{"uri":"at://did:plc:issuer/app.certified.badge.award/accepted-`+suffix+`","cid":"cid-award-accepted"},"response":"accepted"}`)
	insertRecord(t, repo, "at://did:plc:stranger/app.certified.badge.response/reject-"+suffix, "cid-response-stranger-"+suffix, "did:plc:stranger", "app.certified.badge.response", `{"badgeAward":{"uri":"at://did:plc:issuer/app.certified.badge.award/alice-`+suffix+`","cid":"cid-award-alice"},"response":"rejected"}`)

	insertRecord(t, repo, "at://did:plc:alice/app.certified.badge.award/carol-"+suffix, "cid-award-carol-"+suffix, "did:plc:alice", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`","cid":"cid-endorsement"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:carol"}}`)

	got, err := repo.EndorsementAdjacencyFor(ctx, []string{"did:plc:issuer", "did:plc:alice"})
	if err != nil {
		t.Fatalf("EndorsementAdjacencyFor() error = %v", err)
	}

	want := map[string][]string{
		"did:plc:issuer": {"did:plc:alice", "did:plc:zara"},
		"did:plc:alice":  {"did:plc:carol"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EndorsementAdjacencyFor() = %#v, want %#v", got, want)
	}
}

func runEndorsementAdjacencyRequiresDIDSubject(t *testing.T, repo *repositories.RecordsRepository, suffix string) {
	t.Helper()
	ctx := context.Background()

	endorsementBadgeURI := "at://did:plc:issuer/app.certified.badge.definition/subject-shapes-" + suffix
	insertRecord(t, repo, endorsementBadgeURI, "cid-endorsement-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Endorsement","badgeType":"endorsement"}`)

	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/object-"+suffix, "cid-award-object-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:object"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/web-"+suffix, "cid-award-web-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:web:example.com"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/bare-"+suffix, "cid-award-bare-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":"did:plc:bare"}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/strongref-"+suffix, "cid-award-strongref-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":{"uri":"at://did:plc:record-author/app.certified.actor.profile/self","cid":"cid-profile"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/invalid-did-"+suffix, "cid-award-invalid-did-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"not-a-did"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/bad-space-"+suffix, "cid-award-bad-space-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:bad space"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/empty-web-"+suffix, "cid-award-empty-web-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:web:"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/missing-type-"+suffix, "cid-award-missing-type-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`"},"subject":{"did":"did:plc:missingtype"}}`)

	got, err := repo.EndorsementAdjacencyFor(ctx, []string{"did:plc:issuer"})
	if err != nil {
		t.Fatalf("EndorsementAdjacencyFor() error = %v", err)
	}

	want := map[string][]string{
		"did:plc:issuer": {"did:plc:object", "did:web:example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EndorsementAdjacencyFor() = %#v, want %#v", got, want)
	}
}

func runEndorsementAdjacencyAllowedIssuers(t *testing.T, repo *repositories.RecordsRepository, suffix string) {
	t.Helper()
	ctx := context.Background()

	openBadgeURI := "at://did:plc:issuer/app.certified.badge.definition/open-" + suffix
	restrictedBadgeURI := "at://did:plc:issuer/app.certified.badge.definition/restricted-" + suffix
	emptyAllowedBadgeURI := "at://did:plc:issuer/app.certified.badge.definition/empty-" + suffix
	nullAllowedBadgeURI := "at://did:plc:issuer/app.certified.badge.definition/null-" + suffix
	insertRecord(t, repo, openBadgeURI, "cid-open-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Open","badgeType":"endorsement"}`)
	insertRecord(t, repo, restrictedBadgeURI, "cid-restricted-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Restricted","badgeType":"endorsement","allowedIssuers":[{"$type":"app.certified.defs#did","did":"did:plc:allowed"}]}`)
	insertRecord(t, repo, emptyAllowedBadgeURI, "cid-empty-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Empty","badgeType":"endorsement","allowedIssuers":[]}`)
	insertRecord(t, repo, nullAllowedBadgeURI, "cid-null-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Null","badgeType":"endorsement","allowedIssuers":null}`)

	insertRecord(t, repo, "at://did:plc:anyone/app.certified.badge.award/open-"+suffix, "cid-award-open-"+suffix, "did:plc:anyone", "app.certified.badge.award", `{"badge":{"uri":"`+openBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:open"}}`)
	insertRecord(t, repo, "at://did:plc:allowed/app.certified.badge.award/allowed-"+suffix, "cid-award-allowed-"+suffix, "did:plc:allowed", "app.certified.badge.award", `{"badge":{"uri":"`+restrictedBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:allowedsubject"}}`)
	insertRecord(t, repo, "at://did:plc:blocked/app.certified.badge.award/blocked-"+suffix, "cid-award-blocked-"+suffix, "did:plc:blocked", "app.certified.badge.award", `{"badge":{"uri":"`+restrictedBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:blockedsubject"}}`)
	insertRecord(t, repo, "at://did:plc:empty/app.certified.badge.award/empty-"+suffix, "cid-award-empty-"+suffix, "did:plc:empty", "app.certified.badge.award", `{"badge":{"uri":"`+emptyAllowedBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:emptysubject"}}`)
	insertRecord(t, repo, "at://did:plc:null/app.certified.badge.award/null-"+suffix, "cid-award-null-"+suffix, "did:plc:null", "app.certified.badge.award", `{"badge":{"uri":"`+nullAllowedBadgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:nullsubject"}}`)

	got, err := repo.EndorsementAdjacencyFor(ctx, []string{"did:plc:anyone", "did:plc:allowed", "did:plc:blocked", "did:plc:empty", "did:plc:null"})
	if err != nil {
		t.Fatalf("EndorsementAdjacencyFor() error = %v", err)
	}

	want := map[string][]string{
		"did:plc:anyone":  {"did:plc:open"},
		"did:plc:allowed": {"did:plc:allowedsubject"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EndorsementAdjacencyFor() = %#v, want %#v", got, want)
	}
}

func TestEndorsementAdjacencyForLimitSQLite(t *testing.T) {
	db := testutil.SetupTestDB(t)
	runEndorsementAdjacencyLimit(t, db.Records, testSuffix(t))
}

func TestEndorsementAdjacencyForLimitPostgres(t *testing.T) {
	repo, ok := setupPostgresRecordsRepository(t)
	if !ok {
		t.Skip("PostgreSQL endorsement adjacency limit test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	runEndorsementAdjacencyLimit(t, repo, testSuffix(t))
}

func runEndorsementAdjacencyLimit(t *testing.T, repo *repositories.RecordsRepository, suffix string) {
	t.Helper()
	ctx := context.Background()

	badgeURI := "at://did:plc:issuer/app.certified.badge.definition/limit-" + suffix
	insertRecord(t, repo, badgeURI, "cid-limit-"+suffix, "did:plc:issuer", "app.certified.badge.definition", `{"title":"Limit","badgeType":"endorsement"}`)
	for _, subject := range []string{"did:plc:!", "did:plc:a", "did:plc:b", "did:plc:c"} {
		rkey := strings.TrimPrefix(subject, "did:plc:")
		insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/"+rkey+"-"+suffix, "cid-award-"+rkey+"-"+suffix, "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+badgeURI+`"},"subject":{"$type":"app.certified.defs#did","did":"`+subject+`"}}`)
	}

	got, truncated, err := repo.EndorsementAdjacencyForLimit(ctx, []string{"did:plc:issuer"}, 2)
	if err != nil {
		t.Fatalf("EndorsementAdjacencyForLimit() error = %v", err)
	}
	want := map[string][]string{"did:plc:issuer": {"did:plc:a", "did:plc:b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EndorsementAdjacencyForLimit() = %#v, want %#v", got, want)
	}
	if !truncated {
		t.Fatal("EndorsementAdjacencyForLimit() truncated = false, want true")
	}
}

func setupPostgresRecordsRepository(t *testing.T) (*repositories.RecordsRepository, bool) {
	t.Helper()

	databaseURL, ok := safePostgresTestDatabaseURL(t)
	if !ok {
		return nil, false
	}

	exec, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("failed to run postgres migrations: %v", err)
	}
	if _, err := exec.Exec(ctx, "DELETE FROM record", nil); err != nil {
		t.Fatalf("failed to clear postgres test records: %v", err)
	}

	return repositories.NewRecordsRepository(exec), true
}

func safePostgresTestDatabaseURL(t *testing.T) (string, bool) {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return "", false
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("DATABASE_URL is not a valid URL: %v", err)
	}
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if !isSafePostgresTestDatabaseName(databaseName) {
		return "", false
	}

	return databaseURL, true
}

func isSafePostgresTestDatabaseName(databaseName string) bool {
	name := strings.ToLower(strings.TrimSpace(databaseName))
	return name == "test" || strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "-test")
}

func testSuffix(t *testing.T) string {
	replacer := strings.NewReplacer("/", "-", "_", "-", " ", "-")
	return strings.ToLower(replacer.Replace(t.Name()))
}
