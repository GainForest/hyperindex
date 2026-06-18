package repositories_test

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestGetByCollectionSortedWithKeysetCursor_BadgeAwardBadgeTypeFilter(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.Records
	ctx := context.Background()

	const endorsementBadgeURI = "at://did:plc:issuer/app.certified.badge.definition/endorsement"
	const trustedBadgeURI = "at://did:plc:issuer/app.certified.badge.definition/trusted"
	const missingTypeBadgeURI = "at://did:plc:issuer/app.certified.badge.definition/missing-type"
	insertRecord(t, repo, endorsementBadgeURI, "cid-endorsement", "did:plc:issuer", "app.certified.badge.definition", `{"title":"Endorsement","badgeType":"endorsement"}`)
	insertRecord(t, repo, trustedBadgeURI, "cid-trusted", "did:plc:issuer", "app.certified.badge.definition", `{"title":"Trusted","badgeType":"trusted-evaluator"}`)
	insertRecord(t, repo, missingTypeBadgeURI, "cid-missing-type", "did:plc:issuer", "app.certified.badge.definition", `{"title":"Missing type"}`)

	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/endorsement", "cid-award-endorsement", "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+endorsementBadgeURI+`","cid":"cid-endorsement"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:alice"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/trusted", "cid-award-trusted", "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+trustedBadgeURI+`","cid":"cid-trusted"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:bob"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/missing-type", "cid-award-missing-type", "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"`+missingTypeBadgeURI+`","cid":"cid-missing-type"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:carol"}}`)
	insertRecord(t, repo, "at://did:plc:issuer/app.certified.badge.award/missing-definition", "cid-award-missing-definition", "did:plc:issuer", "app.certified.badge.award", `{"badge":{"uri":"at://did:plc:issuer/app.certified.badge.definition/missing","cid":"cid-missing"},"subject":{"$type":"app.certified.defs#did","did":"did:plc:dana"}}`)

	tests := []struct {
		name     string
		operator string
		value    interface{}
		wantURIs []string
	}{
		{
			name:     "eq",
			operator: "eq",
			value:    "endorsement",
			wantURIs: []string{"at://did:plc:issuer/app.certified.badge.award/endorsement"},
		},
		{
			name:     "neq excludes null derived values",
			operator: "neq",
			value:    "endorsement",
			wantURIs: []string{"at://did:plc:issuer/app.certified.badge.award/trusted"},
		},
		{
			name:     "in",
			operator: "in",
			value:    []interface{}{"endorsement", "trusted-evaluator"},
			wantURIs: []string{"at://did:plc:issuer/app.certified.badge.award/endorsement", "at://did:plc:issuer/app.certified.badge.award/trusted"},
		},
		{
			name:     "contains",
			operator: "contains",
			value:    "endorse",
			wantURIs: []string{"at://did:plc:issuer/app.certified.badge.award/endorsement"},
		},
		{
			name:     "startsWith",
			operator: "startsWith",
			value:    "trusted",
			wantURIs: []string{"at://did:plc:issuer/app.certified.badge.award/trusted"},
		},
		{
			name:     "isNull true includes missing type and missing definition",
			operator: "isNull",
			value:    true,
			wantURIs: []string{"at://did:plc:issuer/app.certified.badge.award/missing-definition", "at://did:plc:issuer/app.certified.badge.award/missing-type"},
		},
		{
			name:     "isNull false requires resolved badge type",
			operator: "isNull",
			value:    false,
			wantURIs: []string{"at://did:plc:issuer/app.certified.badge.award/endorsement", "at://did:plc:issuer/app.certified.badge.award/trusted"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := []repositories.FieldFilter{
				{Field: "badgeType", Operator: tt.operator, Value: tt.value, FieldType: "string", Target: repositories.FieldFilterTargetBadgeAwardBadgeType},
			}

			records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "app.certified.badge.award", filters, repositories.DIDFilter{}, nil, 10, nil)
			if err != nil {
				t.Fatalf("GetByCollectionSortedWithKeysetCursor() error = %v", err)
			}
			assertRecordURISet(t, records, tt.wantURIs)

			count, err := repo.GetCollectionCountFiltered(ctx, "app.certified.badge.award", filters, repositories.DIDFilter{})
			if err != nil {
				t.Fatalf("GetCollectionCountFiltered() error = %v", err)
			}
			if count != int64(len(tt.wantURIs)) {
				t.Fatalf("count = %d, want %d", count, len(tt.wantURIs))
			}
		})
	}
}

func insertRecord(t *testing.T, repo *repositories.RecordsRepository, uri, cid, did, collection, jsonData string) {
	t.Helper()
	_, err := repo.Insert(context.Background(), uri, cid, did, collection, jsonData)
	if err != nil {
		t.Fatalf("failed to insert record %s: %v", uri, err)
	}
}
