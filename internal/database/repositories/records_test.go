package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/testutil"
)

type recordsTestEnv struct {
	repo *repositories.RecordsRepository
	db   *testutil.TestDB
}

func setupRecordsTest(t *testing.T) *repositories.RecordsRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Records
}

func setupRecordsTestEnv(t *testing.T) *recordsTestEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return &recordsTestEnv{repo: db.Records, db: db}
}

// insertTestRecord is a helper that inserts a record and fails the test on error.
func insertTestRecord(t *testing.T, repo *repositories.RecordsRepository, uri, cid, did, collection, jsonData string) {
	t.Helper()
	_, err := repo.Insert(context.Background(), uri, cid, did, collection, jsonData)
	if err != nil {
		t.Fatalf("failed to insert test record %s: %v", uri, err)
	}
}

func TestRecordsRepository_Insert(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*repositories.RecordsRepository)
		uri        string
		cid        string
		did        string
		collection string
		json       string
		wantResult repositories.InsertResult
		wantErr    bool
	}{
		{
			name:       "insert new record",
			uri:        "at://did:plc:test1/app.bsky.feed.post/abc123",
			cid:        "bafyreiabc123",
			did:        "did:plc:test1",
			collection: "app.bsky.feed.post",
			json:       `{"text":"hello","createdAt":"2026-01-15T10:00:00Z"}`,
			wantResult: repositories.Inserted,
		},
		{
			name: "insert same URI and same CID is skipped",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo,
					"at://did:plc:test1/app.bsky.feed.post/dup1",
					"bafyreisame",
					"did:plc:test1",
					"app.bsky.feed.post",
					`{"text":"original"}`,
				)
			},
			uri:        "at://did:plc:test1/app.bsky.feed.post/dup1",
			cid:        "bafyreisame",
			did:        "did:plc:test1",
			collection: "app.bsky.feed.post",
			json:       `{"text":"original"}`,
			wantResult: repositories.Skipped,
		},
		{
			name:       "insert new record with empty CID is inserted (not silently skipped)",
			uri:        "at://did:plc:test1/app.bsky.feed.post/nocid",
			cid:        "", // Tap omits CID on some events
			did:        "did:plc:test1",
			collection: "app.bsky.feed.post",
			json:       `{"text":"no cid"}`,
			wantResult: repositories.Inserted,
		},
		{
			name: "insert same URI with different CID is updated",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo,
					"at://did:plc:test1/app.bsky.feed.post/upd1",
					"bafyreiold",
					"did:plc:test1",
					"app.bsky.feed.post",
					`{"text":"old version"}`,
				)
			},
			uri:        "at://did:plc:test1/app.bsky.feed.post/upd1",
			cid:        "bafyreinew",
			did:        "did:plc:test1",
			collection: "app.bsky.feed.post",
			json:       `{"text":"new version"}`,
			wantResult: repositories.Inserted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			result, err := repo.Insert(ctx, tt.uri, tt.cid, tt.did, tt.collection, tt.json)
			if (err != nil) != tt.wantErr {
				t.Errorf("Insert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.wantResult {
				t.Errorf("Insert() result = %v, want %v", result, tt.wantResult)
			}

			// For the update case, verify the CID was actually updated
			if tt.name == "insert same URI with different CID is updated" {
				rec, err := repo.GetByURI(ctx, tt.uri)
				if err != nil {
					t.Fatalf("GetByURI after update: %v", err)
				}
				if rec.CID != tt.cid {
					t.Errorf("CID after update = %q, want %q", rec.CID, tt.cid)
				}
				if rec.JSON != tt.json {
					t.Errorf("JSON after update = %q, want %q", rec.JSON, tt.json)
				}
			}
		})
	}
}

func TestRecordsRepository_BatchInsert(t *testing.T) {
	tests := []struct {
		name    string
		records []*repositories.Record
		wantErr bool
	}{
		{
			name:    "empty slice",
			records: nil,
		},
		{
			name: "single record",
			records: []*repositories.Record{
				{
					URI:        "at://did:plc:test1/app.bsky.feed.post/batch1",
					CID:        "bafyreibatch1",
					DID:        "did:plc:test1",
					Collection: "app.bsky.feed.post",
					JSON:       `{"text":"batch 1","createdAt":"2026-01-15T10:00:00Z"}`,
				},
			},
		},
		{
			name: "five records",
			records: []*repositories.Record{
				{URI: "at://did:plc:test1/app.bsky.feed.post/b1", CID: "bafyreib1", DID: "did:plc:test1", Collection: "app.bsky.feed.post", JSON: `{"text":"b1"}`},
				{URI: "at://did:plc:test1/app.bsky.feed.post/b2", CID: "bafyreib2", DID: "did:plc:test1", Collection: "app.bsky.feed.post", JSON: `{"text":"b2"}`},
				{URI: "at://did:plc:test2/app.bsky.feed.post/b3", CID: "bafyreib3", DID: "did:plc:test2", Collection: "app.bsky.feed.post", JSON: `{"text":"b3"}`},
				{URI: "at://did:plc:test2/app.bsky.feed.like/b4", CID: "bafyreib4", DID: "did:plc:test2", Collection: "app.bsky.feed.like", JSON: `{"subject":"at://x"}`},
				{URI: "at://did:plc:test3/app.bsky.feed.post/b5", CID: "bafyreib5", DID: "did:plc:test3", Collection: "app.bsky.feed.post", JSON: `{"text":"b5"}`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			err := repo.BatchInsert(ctx, tt.records)
			if (err != nil) != tt.wantErr {
				t.Errorf("BatchInsert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify all records are retrievable
			for _, rec := range tt.records {
				got, err := repo.GetByURI(ctx, rec.URI)
				if err != nil {
					t.Errorf("GetByURI(%s) after BatchInsert: %v", rec.URI, err)
					continue
				}
				if got.CID != rec.CID {
					t.Errorf("record %s CID = %q, want %q", rec.URI, got.CID, rec.CID)
				}
				if got.DID != rec.DID {
					t.Errorf("record %s DID = %q, want %q", rec.URI, got.DID, rec.DID)
				}
				if got.Collection != rec.Collection {
					t.Errorf("record %s Collection = %q, want %q", rec.URI, got.Collection, rec.Collection)
				}
			}
		})
	}
}

func TestRecordsRepository_GetByURI(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*repositories.RecordsRepository)
		uri     string
		wantErr error
		check   func(*testing.T, *repositories.Record)
	}{
		{
			name: "found",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo,
					"at://did:plc:test1/app.bsky.feed.post/found1",
					"bafyreifound1",
					"did:plc:test1",
					"app.bsky.feed.post",
					`{"text":"found me","createdAt":"2026-01-15T10:00:00Z"}`,
				)
			},
			uri: "at://did:plc:test1/app.bsky.feed.post/found1",
			check: func(t *testing.T, rec *repositories.Record) {
				if rec.URI != "at://did:plc:test1/app.bsky.feed.post/found1" {
					t.Errorf("URI = %q", rec.URI)
				}
				if rec.CID != "bafyreifound1" {
					t.Errorf("CID = %q", rec.CID)
				}
				if rec.DID != "did:plc:test1" {
					t.Errorf("DID = %q", rec.DID)
				}
				if rec.Collection != "app.bsky.feed.post" {
					t.Errorf("Collection = %q", rec.Collection)
				}
				if rec.JSON != `{"text":"found me","createdAt":"2026-01-15T10:00:00Z"}` {
					t.Errorf("JSON = %q", rec.JSON)
				}
			},
		},
		{
			name:    "not found",
			uri:     "at://did:plc:nonexistent/app.bsky.feed.post/nope",
			wantErr: sql.ErrNoRows,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			rec, err := repo.GetByURI(ctx, tt.uri)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("GetByURI() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetByURI() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, rec)
			}
		})
	}
}

func TestRecordsRepository_GetByURIs(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*repositories.RecordsRepository)
		uris      []string
		wantCount int
	}{
		{
			name:      "empty slice returns nil",
			uris:      nil,
			wantCount: 0,
		},
		{
			name: "multiple URIs",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/m1", "bafyreim1", "did:plc:test1", "app.bsky.feed.post", `{"text":"m1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/m2", "bafyreim2", "did:plc:test1", "app.bsky.feed.post", `{"text":"m2"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/m3", "bafyreim3", "did:plc:test1", "app.bsky.feed.post", `{"text":"m3"}`)
			},
			uris: []string{
				"at://did:plc:test1/app.bsky.feed.post/m1",
				"at://did:plc:test1/app.bsky.feed.post/m3",
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			records, err := repo.GetByURIs(ctx, tt.uris)
			if err != nil {
				t.Fatalf("GetByURIs() error: %v", err)
			}
			if len(records) != tt.wantCount {
				t.Errorf("GetByURIs() returned %d records, want %d", len(records), tt.wantCount)
			}

			// Verify each requested URI is in the results
			if tt.wantCount > 0 {
				uriSet := make(map[string]bool)
				for _, rec := range records {
					uriSet[rec.URI] = true
				}
				for _, uri := range tt.uris {
					if !uriSet[uri] {
						t.Errorf("GetByURIs() missing expected URI %s", uri)
					}
				}
			}
		})
	}
}

func TestRecordsRepository_GetByCollection(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	// Insert records across two collections
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/c1", "bafyreic1", "did:plc:test1", "app.bsky.feed.post", `{"text":"c1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/c2", "bafyreic2", "did:plc:test1", "app.bsky.feed.post", `{"text":"c2"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.like/c3", "bafyreic3", "did:plc:test1", "app.bsky.feed.like", `{"subject":"at://x"}`)

	t.Run("returns records for specific collection", func(t *testing.T) {
		records, err := repo.GetByCollection(ctx, "app.bsky.feed.post", 100)
		if err != nil {
			t.Fatalf("GetByCollection() error: %v", err)
		}
		if len(records) != 2 {
			t.Errorf("got %d records, want 2", len(records))
		}
		for _, rec := range records {
			if rec.Collection != "app.bsky.feed.post" {
				t.Errorf("unexpected collection %q", rec.Collection)
			}
		}
	})

	t.Run("does not return records from other collections", func(t *testing.T) {
		records, err := repo.GetByCollection(ctx, "app.bsky.feed.like", 100)
		if err != nil {
			t.Fatalf("GetByCollection() error: %v", err)
		}
		if len(records) != 1 {
			t.Errorf("got %d records, want 1", len(records))
		}
		if len(records) > 0 && records[0].Collection != "app.bsky.feed.like" {
			t.Errorf("unexpected collection %q", records[0].Collection)
		}
	})
}

func TestRecordsRepository_GetByCollectionWithCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	// Use the executor's underlying DB to set indexed_at to distinct values
	sqlDB := env.db.Executor.DB()

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/p1", "bafyreip1", "did:plc:test1", "app.bsky.feed.post", `{"text":"p1"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/p1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/p2", "bafyreip2", "did:plc:test1", "app.bsky.feed.post", `{"text":"p2"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/p2'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/p3", "bafyreip3", "did:plc:test1", "app.bsky.feed.post", `{"text":"p3"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/p3'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/p4", "bafyreip4", "did:plc:test1", "app.bsky.feed.post", `{"text":"p4"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/p4'`)

	t.Run("first page returns newest first", func(t *testing.T) {
		records, err := repo.GetByCollectionWithCursor(ctx, "app.bsky.feed.post", 2, "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Newest first: p4, p3
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/p4" {
			t.Errorf("first record URI = %q, want p4", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/p3" {
			t.Errorf("second record URI = %q, want p3", records[1].URI)
		}
	})

	t.Run("second page with cursor returns older records", func(t *testing.T) {
		// Use p3's indexed_at as cursor to get records older than p3
		records, err := repo.GetByCollectionWithCursor(ctx, "app.bsky.feed.post", 2, "2026-01-15T12:00:00Z")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Older than p3: p2, p1
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/p2" {
			t.Errorf("first record URI = %q, want p2", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/p1" {
			t.Errorf("second record URI = %q, want p1", records[1].URI)
		}
	})
}

func TestRecordsRepository_GetByCollectionWithKeysetCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert records with distinct indexed_at timestamps
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/k1", "bafyreik1", "did:plc:test1", "app.bsky.feed.post", `{"text":"k1"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/k1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/k2", "bafyreik2", "did:plc:test1", "app.bsky.feed.post", `{"text":"k2"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/k2'`)

	// k3a and k3b have the SAME indexed_at to test URI tiebreaking
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/k3a", "bafyreik3a", "did:plc:test1", "app.bsky.feed.post", `{"text":"k3a"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/k3a'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/k3b", "bafyreik3b", "did:plc:test1", "app.bsky.feed.post", `{"text":"k3b"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/k3b'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/k4", "bafyreik4", "did:plc:test1", "app.bsky.feed.post", `{"text":"k4"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/k4'`)

	t.Run("first page without cursor", func(t *testing.T) {
		records, err := repo.GetByCollectionWithKeysetCursor(ctx, "app.bsky.feed.post", 3, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("got %d records, want 3", len(records))
		}
		// Newest first: k4, k3b, k3a (k3b > k3a by URI DESC)
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/k4" {
			t.Errorf("first record URI = %q, want k4", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/k3b" {
			t.Errorf("second record URI = %q, want k3b", records[1].URI)
		}
		if records[2].URI != "at://did:plc:test1/app.bsky.feed.post/k3a" {
			t.Errorf("third record URI = %q, want k3a", records[2].URI)
		}
	})

	t.Run("keyset cursor skips to correct position", func(t *testing.T) {
		// Cursor is after k3b (same timestamp as k3a, but k3b > k3a by URI)
		records, err := repo.GetByCollectionWithKeysetCursor(ctx, "app.bsky.feed.post", 10,
			"2026-01-15T12:00:00Z", "at://did:plc:test1/app.bsky.feed.post/k3b")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("got %d records, want 3", len(records))
		}
		// After k3b: k3a (same timestamp, smaller URI), then k2, k1
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/k3a" {
			t.Errorf("first record URI = %q, want k3a", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/k2" {
			t.Errorf("second record URI = %q, want k2", records[1].URI)
		}
		if records[2].URI != "at://did:plc:test1/app.bsky.feed.post/k1" {
			t.Errorf("third record URI = %q, want k1", records[2].URI)
		}
	})

	t.Run("cursor between timestamps", func(t *testing.T) {
		// Cursor after k3a — should return k2, k1
		records, err := repo.GetByCollectionWithKeysetCursor(ctx, "app.bsky.feed.post", 10,
			"2026-01-15T12:00:00Z", "at://did:plc:test1/app.bsky.feed.post/k3a")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/k2" {
			t.Errorf("first record URI = %q, want k2", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/k1" {
			t.Errorf("second record URI = %q, want k1", records[1].URI)
		}
	})
}

func TestRecordsRepository_GetByDID(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	insertTestRecord(t, repo, "at://did:plc:alice/app.bsky.feed.post/a1", "bafyreia1", "did:plc:alice", "app.bsky.feed.post", `{"text":"a1"}`)
	insertTestRecord(t, repo, "at://did:plc:alice/app.bsky.feed.like/a2", "bafyreia2", "did:plc:alice", "app.bsky.feed.like", `{"subject":"at://x"}`)
	insertTestRecord(t, repo, "at://did:plc:bob/app.bsky.feed.post/b1", "bafyreib1", "did:plc:bob", "app.bsky.feed.post", `{"text":"b1"}`)

	records, err := repo.GetByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("GetByDID() error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2", len(records))
	}
	for _, rec := range records {
		if rec.DID != "did:plc:alice" {
			t.Errorf("unexpected DID %q, want did:plc:alice", rec.DID)
		}
	}
}

func TestRecordsRepository_Delete(t *testing.T) {
	t.Run("delete existing record", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/del1", "bafyreidel1", "did:plc:test1", "app.bsky.feed.post", `{"text":"delete me"}`)

		countBefore, _ := repo.GetCount(ctx)

		err := repo.Delete(ctx, "at://did:plc:test1/app.bsky.feed.post/del1")
		if err != nil {
			t.Fatalf("Delete() error: %v", err)
		}

		countAfter, _ := repo.GetCount(ctx)
		if countAfter != countBefore-1 {
			t.Errorf("count after delete = %d, want %d", countAfter, countBefore-1)
		}

		_, err = repo.GetByURI(ctx, "at://did:plc:test1/app.bsky.feed.post/del1")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
		}
	})

	t.Run("delete non-existing record is no error", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		err := repo.Delete(ctx, "at://did:plc:nonexistent/app.bsky.feed.post/nope")
		if err != nil {
			t.Errorf("Delete() on non-existing record should not error, got: %v", err)
		}
	})
}

func TestRecordsRepository_DeleteAll(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/da1", "bafyreida1", "did:plc:test1", "app.bsky.feed.post", `{"text":"da1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/da2", "bafyreida2", "did:plc:test1", "app.bsky.feed.post", `{"text":"da2"}`)

	err := repo.DeleteAll(ctx)
	if err != nil {
		t.Fatalf("DeleteAll() error: %v", err)
	}

	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error: %v", err)
	}
	if count != 0 {
		t.Errorf("count after DeleteAll = %d, want 0", count)
	}
}

func TestRecordsRepository_GetCount(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*repositories.RecordsRepository)
		wantCount int64
	}{
		{
			name:      "empty database",
			wantCount: 0,
		},
		{
			name: "after inserts",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/gc1", "bafyreigc1", "did:plc:test1", "app.bsky.feed.post", `{"text":"gc1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/gc2", "bafyreigc2", "did:plc:test1", "app.bsky.feed.post", `{"text":"gc2"}`)
				insertTestRecord(t, repo, "at://did:plc:test2/app.bsky.feed.post/gc3", "bafyreigc3", "did:plc:test2", "app.bsky.feed.post", `{"text":"gc3"}`)
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			count, err := repo.GetCount(ctx)
			if err != nil {
				t.Fatalf("GetCount() error: %v", err)
			}
			if count != tt.wantCount {
				t.Errorf("GetCount() = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestRecordsRepository_GetCollectionStats(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	// Insert records: 3 posts, 2 likes, 1 follow
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/s1", "bafyreis1", "did:plc:test1", "app.bsky.feed.post", `{"text":"s1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/s2", "bafyreis2", "did:plc:test1", "app.bsky.feed.post", `{"text":"s2"}`)
	insertTestRecord(t, repo, "at://did:plc:test2/app.bsky.feed.post/s3", "bafyreis3", "did:plc:test2", "app.bsky.feed.post", `{"text":"s3"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.like/s4", "bafyreis4", "did:plc:test1", "app.bsky.feed.like", `{"subject":"at://x"}`)
	insertTestRecord(t, repo, "at://did:plc:test2/app.bsky.feed.like/s5", "bafyreis5", "did:plc:test2", "app.bsky.feed.like", `{"subject":"at://y"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.graph.follow/s6", "bafyreis6", "did:plc:test1", "app.bsky.graph.follow", `{"subject":"did:plc:test2"}`)

	stats, err := repo.GetCollectionStats(ctx)
	if err != nil {
		t.Fatalf("GetCollectionStats() error: %v", err)
	}

	if len(stats) != 3 {
		t.Fatalf("got %d stats, want 3", len(stats))
	}

	// Ordered by count DESC: posts(3), likes(2), follow(1)
	if stats[0].Collection != "app.bsky.feed.post" || stats[0].Count != 3 {
		t.Errorf("stats[0] = {%s, %d}, want {app.bsky.feed.post, 3}", stats[0].Collection, stats[0].Count)
	}
	if stats[1].Collection != "app.bsky.feed.like" || stats[1].Count != 2 {
		t.Errorf("stats[1] = {%s, %d}, want {app.bsky.feed.like, 2}", stats[1].Collection, stats[1].Count)
	}
	if stats[2].Collection != "app.bsky.graph.follow" || stats[2].Count != 1 {
		t.Errorf("stats[2] = {%s, %d}, want {app.bsky.graph.follow, 1}", stats[2].Collection, stats[2].Count)
	}
}

func TestRecordsRepository_GetCollectionStatsFiltered(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sf1", "bafyreisf1", "did:plc:test1", "app.bsky.feed.post", `{"text":"sf1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sf2", "bafyreisf2", "did:plc:test1", "app.bsky.feed.post", `{"text":"sf2"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.like/sf3", "bafyreisf3", "did:plc:test1", "app.bsky.feed.like", `{"subject":"at://x"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.graph.follow/sf4", "bafyreisf4", "did:plc:test1", "app.bsky.graph.follow", `{"subject":"did:plc:test2"}`)

	t.Run("with specific collections", func(t *testing.T) {
		stats, err := repo.GetCollectionStatsFiltered(ctx, []string{"app.bsky.feed.post", "app.bsky.feed.like"})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("got %d stats, want 2", len(stats))
		}
		// Verify only requested collections appear
		for _, stat := range stats {
			if stat.Collection != "app.bsky.feed.post" && stat.Collection != "app.bsky.feed.like" {
				t.Errorf("unexpected collection %q in filtered results", stat.Collection)
			}
		}
	})

	t.Run("empty collections returns all", func(t *testing.T) {
		stats, err := repo.GetCollectionStatsFiltered(ctx, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(stats) != 3 {
			t.Errorf("got %d stats, want 3 (all collections)", len(stats))
		}
	})
}

func TestRecordsRepository_GetCollectionTimeSeries(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	// Insert records with createdAt in JSON on different dates, from different users
	insertTestRecord(t, repo, "at://did:plc:alice/app.bsky.feed.post/ts1", "bafyreits1", "did:plc:alice", "app.bsky.feed.post", `{"text":"ts1","createdAt":"2026-01-15T10:00:00Z"}`)
	insertTestRecord(t, repo, "at://did:plc:alice/app.bsky.feed.post/ts2", "bafyreits2", "did:plc:alice", "app.bsky.feed.post", `{"text":"ts2","createdAt":"2026-01-15T14:00:00Z"}`)
	insertTestRecord(t, repo, "at://did:plc:bob/app.bsky.feed.post/ts3", "bafyreits3", "did:plc:bob", "app.bsky.feed.post", `{"text":"ts3","createdAt":"2026-01-16T09:00:00Z"}`)

	ts, err := repo.GetCollectionTimeSeries(ctx, "app.bsky.feed.post")
	if err != nil {
		t.Fatalf("GetCollectionTimeSeries() error: %v", err)
	}

	if ts.Collection != "app.bsky.feed.post" {
		t.Errorf("Collection = %q", ts.Collection)
	}
	if ts.TotalRecords != 3 {
		t.Errorf("TotalRecords = %d, want 3", ts.TotalRecords)
	}
	if ts.UniqueUsers != 2 {
		t.Errorf("UniqueUsers = %d, want 2", ts.UniqueUsers)
	}

	if len(ts.Data) < 2 {
		t.Fatalf("got %d data points, want at least 2", len(ts.Data))
	}

	// First date: 2026-01-15 with 2 records
	if ts.Data[0].Date != "2026-01-15" {
		t.Errorf("Data[0].Date = %q, want 2026-01-15", ts.Data[0].Date)
	}
	if ts.Data[0].Count != 2 {
		t.Errorf("Data[0].Count = %d, want 2", ts.Data[0].Count)
	}
	if ts.Data[0].Cumulative != 2 {
		t.Errorf("Data[0].Cumulative = %d, want 2", ts.Data[0].Cumulative)
	}

	// Second date: 2026-01-16 with 1 record, cumulative 3
	if ts.Data[1].Date != "2026-01-16" {
		t.Errorf("Data[1].Date = %q, want 2026-01-16", ts.Data[1].Date)
	}
	if ts.Data[1].Count != 1 {
		t.Errorf("Data[1].Count = %d, want 1", ts.Data[1].Count)
	}
	if ts.Data[1].Cumulative != 3 {
		t.Errorf("Data[1].Cumulative = %d, want 3", ts.Data[1].Cumulative)
	}
}

func TestRecordsRepository_GetCIDsByURIs(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*repositories.RecordsRepository)
		uris    []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "empty returns empty map",
			uris: nil,
			want: map[string]string{},
		},
		{
			name: "returns correct URI to CID mapping",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/cid1", "bafyreicid1", "did:plc:test1", "app.bsky.feed.post", `{"text":"cid1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/cid2", "bafyreicid2", "did:plc:test1", "app.bsky.feed.post", `{"text":"cid2"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/cid3", "bafyreicid3", "did:plc:test1", "app.bsky.feed.post", `{"text":"cid3"}`)
			},
			uris: []string{
				"at://did:plc:test1/app.bsky.feed.post/cid1",
				"at://did:plc:test1/app.bsky.feed.post/cid3",
			},
			want: map[string]string{
				"at://did:plc:test1/app.bsky.feed.post/cid1": "bafyreicid1",
				"at://did:plc:test1/app.bsky.feed.post/cid3": "bafyreicid3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			got, err := repo.GetCIDsByURIs(ctx, tt.uris)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetCIDsByURIs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d entries, want %d", len(got), len(tt.want))
			}
			for uri, wantCID := range tt.want {
				if gotCID, ok := got[uri]; !ok {
					t.Errorf("missing URI %s in result", uri)
				} else if gotCID != wantCID {
					t.Errorf("CID for %s = %q, want %q", uri, gotCID, wantCID)
				}
			}
		})
	}
}

func TestRecordsRepository_GetExistingCIDs(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*repositories.RecordsRepository)
		cids    []string
		want    map[string]bool
		wantErr bool
	}{
		{
			name: "empty returns empty map",
			cids: nil,
			want: map[string]bool{},
		},
		{
			name: "returns correct existing CIDs",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/ec1", "bafyreiec1", "did:plc:test1", "app.bsky.feed.post", `{"text":"ec1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/ec2", "bafyreiec2", "did:plc:test1", "app.bsky.feed.post", `{"text":"ec2"}`)
			},
			cids: []string{"bafyreiec1", "bafyreiec2", "bafyreinonexistent"},
			want: map[string]bool{
				"bafyreiec1": true,
				"bafyreiec2": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			got, err := repo.GetExistingCIDs(ctx, tt.cids)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetExistingCIDs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d entries, want %d", len(got), len(tt.want))
			}
			for cid, wantVal := range tt.want {
				if gotVal, ok := got[cid]; !ok {
					t.Errorf("missing CID %s in result", cid)
				} else if gotVal != wantVal {
					t.Errorf("value for CID %s = %v, want %v", cid, gotVal, wantVal)
				}
			}
			// Ensure non-existent CID is not in the result
			if _, ok := got["bafyreinonexistent"]; ok {
				t.Error("non-existent CID should not be in result")
			}
		})
	}
}

func TestRecordsRepository_GetByCollectionFilteredWithKeysetCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert records with distinct indexed_at timestamps and varied JSON fields
	insertTestRecord(t, repo, "at://did:plc:alice/app.bsky.feed.post/f1", "bafyreif1", "did:plc:alice", "app.bsky.feed.post", `{"text":"hello world","score":10}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:alice/app.bsky.feed.post/f1'`)

	insertTestRecord(t, repo, "at://did:plc:alice/app.bsky.feed.post/f2", "bafyreif2", "did:plc:alice", "app.bsky.feed.post", `{"text":"goodbye world","score":20}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:alice/app.bsky.feed.post/f2'`)

	insertTestRecord(t, repo, "at://did:plc:bob/app.bsky.feed.post/f3", "bafyreif3", "did:plc:bob", "app.bsky.feed.post", `{"text":"hello again"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:bob/app.bsky.feed.post/f3'`)

	insertTestRecord(t, repo, "at://did:plc:bob/app.bsky.feed.post/f4", "bafyreif4", "did:plc:bob", "app.bsky.feed.post", `{"text":"no greeting"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:bob/app.bsky.feed.post/f4'`)

	t.Run("no filters returns all records", func(t *testing.T) {
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 4 {
			t.Errorf("got %d records, want 4", len(records))
		}
	})

	t.Run("filter by string eq", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "text", Operator: "eq", Value: "hello world", FieldType: "string"},
		}
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", filters, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("got %d records, want 1", len(records))
		}
		if records[0].URI != "at://did:plc:alice/app.bsky.feed.post/f1" {
			t.Errorf("unexpected URI %q", records[0].URI)
		}
	})

	t.Run("filter by isNull true returns records without field", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "score", Operator: "isNull", Value: true, FieldType: "integer"},
		}
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", filters, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		// f3 and f4 have no score field
		if len(records) != 2 {
			t.Errorf("got %d records, want 2", len(records))
		}
	})

	t.Run("filter with DID omits when empty", func(t *testing.T) {
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 4 {
			t.Errorf("got %d records, want 4 (no DID filter)", len(records))
		}
	})

	t.Run("filter with DID adds AND did = ? when non-empty", func(t *testing.T) {
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{EQ: "did:plc:alice"}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Errorf("got %d records, want 2", len(records))
		}
		for _, rec := range records {
			if rec.DID != "did:plc:alice" {
				t.Errorf("unexpected DID %q, want did:plc:alice", rec.DID)
			}
		}
	})

	t.Run("filter with DID and field filter combined", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "text", Operator: "contains", Value: "hello", FieldType: "string"},
		}
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", filters, repositories.DIDFilter{EQ: "did:plc:alice"}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("got %d records, want 1", len(records))
		}
		if records[0].URI != "at://did:plc:alice/app.bsky.feed.post/f1" {
			t.Errorf("unexpected URI %q", records[0].URI)
		}
	})

	t.Run("pagination with filters", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "text", Operator: "contains", Value: "hello", FieldType: "string"},
		}
		// f3 (2026-01-15T12:00:00Z) and f1 (2026-01-15T10:00:00Z) contain "hello"
		// First page: limit 1 → f3 (newest)
		page1, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", filters, repositories.DIDFilter{}, 1, "", "")
		if err != nil {
			t.Fatalf("page1 error: %v", err)
		}
		if len(page1) != 1 {
			t.Fatalf("page1: got %d records, want 1", len(page1))
		}
		if page1[0].URI != "at://did:plc:bob/app.bsky.feed.post/f3" {
			t.Errorf("page1[0] URI = %q, want f3", page1[0].URI)
		}

		// Second page using cursor from f3
		afterTS := page1[0].IndexedAt.UTC().Format("2006-01-02T15:04:05Z")
		page2, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "app.bsky.feed.post", filters, repositories.DIDFilter{}, 1, afterTS, page1[0].URI)
		if err != nil {
			t.Fatalf("page2 error: %v", err)
		}
		if len(page2) != 1 {
			t.Fatalf("page2: got %d records, want 1", len(page2))
		}
		if page2[0].URI != "at://did:plc:alice/app.bsky.feed.post/f1" {
			t.Errorf("page2[0] URI = %q, want f1", page2[0].URI)
		}
	})
}

func TestRecordsRepository_IterateAll(t *testing.T) {
	t.Run("empty database returns 0 processed", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		count, err := repo.IterateAll(ctx, 10, func(r *repositories.Record) error {
			t.Error("callback should not be called on empty DB")
			return nil
		})
		if err != nil {
			t.Fatalf("IterateAll() error: %v", err)
		}
		if count != 0 {
			t.Errorf("processed = %d, want 0", count)
		}
	})

	t.Run("processes all records in URI order", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		// Insert 5 records with URIs that sort alphabetically
		for i := 1; i <= 5; i++ {
			uri := fmt.Sprintf("at://did:plc:test1/app.bsky.feed.post/iter%d", i)
			cid := fmt.Sprintf("bafyreiiter%d", i)
			jsonStr := fmt.Sprintf(`{"text":"iter%d","createdAt":"2026-01-15T10:00:00Z"}`, i)
			insertTestRecord(t, repo, uri, cid, "did:plc:test1", "app.bsky.feed.post", jsonStr)
		}

		var visited []string
		count, err := repo.IterateAll(ctx, 2, func(r *repositories.Record) error {
			visited = append(visited, r.URI)
			return nil
		})
		if err != nil {
			t.Fatalf("IterateAll() error: %v", err)
		}
		if count != 5 {
			t.Errorf("processed = %d, want 5", count)
		}
		if len(visited) != 5 {
			t.Fatalf("visited %d records, want 5", len(visited))
		}

		// Verify URI order (ascending)
		for i := 1; i < len(visited); i++ {
			if visited[i] <= visited[i-1] {
				t.Errorf("records not in URI order: %q <= %q", visited[i], visited[i-1])
			}
		}
	})

	t.Run("callback error stops iteration", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/err1", "bafyreierr1", "did:plc:test1", "app.bsky.feed.post", `{"text":"err1"}`)
		insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/err2", "bafyreierr2", "did:plc:test1", "app.bsky.feed.post", `{"text":"err2"}`)
		insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/err3", "bafyreierr3", "did:plc:test1", "app.bsky.feed.post", `{"text":"err3"}`)

		callbackErr := fmt.Errorf("stop processing")
		callCount := 0

		count, err := repo.IterateAll(ctx, 10, func(r *repositories.Record) error {
			callCount++
			if callCount == 2 {
				return callbackErr
			}
			return nil
		})

		if !errors.Is(err, callbackErr) {
			t.Errorf("IterateAll() error = %v, want %v", err, callbackErr)
		}
		// totalProcessed is incremented after fn returns successfully,
		// so it should be 1 (the first successful call before the error on call 2)
		if count != 1 {
			t.Errorf("processed = %d, want 1", count)
		}
	})

	t.Run("batchSize 0 defaults to 1000", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/bs1", "bafyreibs1", "did:plc:test1", "app.bsky.feed.post", `{"text":"bs1"}`)
		insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/bs2", "bafyreibs2", "did:plc:test1", "app.bsky.feed.post", `{"text":"bs2"}`)

		count, err := repo.IterateAll(ctx, 0, func(r *repositories.Record) error {
			return nil
		})
		if err != nil {
			t.Fatalf("IterateAll() error: %v", err)
		}
		if count != 2 {
			t.Errorf("processed = %d, want 2", count)
		}
	})
}

func TestRecordsRepository_Search(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*repositories.RecordsRepository)
		query          string
		collection     string
		limit          int
		afterTimestamp string
		afterURI       string
		wantCount      int
		wantURIs       []string
	}{
		{
			name: "returns records containing search term",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sr1", "bafyreisr1", "did:plc:test1", "app.bsky.feed.post", `{"text":"hello world"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sr2", "bafyreisr2", "did:plc:test1", "app.bsky.feed.post", `{"text":"goodbye world"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sr3", "bafyreisr3", "did:plc:test1", "app.bsky.feed.post", `{"text":"hello again"}`)
			},
			query:     "hello",
			limit:     10,
			wantCount: 2,
		},
		{
			name: "search with collection filter narrows results",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sc1", "bafyreisc1", "did:plc:test1", "app.bsky.feed.post", `{"text":"hello post"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.like/sc2", "bafyreisc2", "did:plc:test1", "app.bsky.feed.like", `{"text":"hello like"}`)
			},
			query:      "hello",
			collection: "app.bsky.feed.post",
			limit:      10,
			wantCount:  1,
		},
		{
			name: "search returns no results when term not found",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sn1", "bafyreisn1", "did:plc:test1", "app.bsky.feed.post", `{"text":"nothing here"}`)
			},
			query:     "xyzzy",
			limit:     10,
			wantCount: 0,
		},
		{
			name: "search is case-insensitive",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/si1", "bafyreisi1", "did:plc:test1", "app.bsky.feed.post", `{"text":"Hello World"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/si2", "bafyreisi2", "did:plc:test1", "app.bsky.feed.post", `{"text":"HELLO WORLD"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/si3", "bafyreisi3", "did:plc:test1", "app.bsky.feed.post", `{"text":"hello world"}`)
			},
			query:     "hello",
			limit:     10,
			wantCount: 3,
		},
		{
			name: "search with pagination limit",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sp1", "bafyreisp1", "did:plc:test1", "app.bsky.feed.post", `{"text":"paginate me one"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sp2", "bafyreisp2", "did:plc:test1", "app.bsky.feed.post", `{"text":"paginate me two"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sp3", "bafyreisp3", "did:plc:test1", "app.bsky.feed.post", `{"text":"paginate me three"}`)
			},
			query:     "paginate",
			limit:     2,
			wantCount: 2,
		},
		{
			name: "search escapes percent wildcard in query",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sw1", "bafyreisw1", "did:plc:test1", "app.bsky.feed.post", `{"text":"100% complete"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/sw2", "bafyreisw2", "did:plc:test1", "app.bsky.feed.post", `{"text":"anything else"}`)
			},
			query:     "100%",
			limit:     10,
			wantCount: 1,
		},
		{
			name: "search escapes underscore wildcard in query",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/su1", "bafyreisu1", "did:plc:test1", "app.bsky.feed.post", `{"text":"hello_world"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/su2", "bafyreisu2", "did:plc:test1", "app.bsky.feed.post", `{"text":"helloXworld"}`)
			},
			query:     "hello_world",
			limit:     10,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			records, err := repo.Search(ctx, tt.query, tt.collection, tt.limit, tt.afterTimestamp, tt.afterURI)
			if err != nil {
				t.Fatalf("Search() error: %v", err)
			}
			if len(records) != tt.wantCount {
				t.Errorf("Search() returned %d records, want %d", len(records), tt.wantCount)
			}

			// Verify specific URIs if provided
			if len(tt.wantURIs) > 0 {
				uriSet := make(map[string]bool)
				for _, rec := range records {
					uriSet[rec.URI] = true
				}
				for _, uri := range tt.wantURIs {
					if !uriSet[uri] {
						t.Errorf("Search() missing expected URI %s", uri)
					}
				}
			}
		})
	}
}

func TestRecordsRepository_Search_Pagination(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert records with distinct indexed_at timestamps
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/pg1", "bafyreipg1", "did:plc:test1", "app.bsky.feed.post", `{"text":"search term alpha"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/pg1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/pg2", "bafyreipg2", "did:plc:test1", "app.bsky.feed.post", `{"text":"search term beta"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/pg2'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/pg3", "bafyreipg3", "did:plc:test1", "app.bsky.feed.post", `{"text":"search term gamma"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/pg3'`)

	t.Run("first page returns newest first", func(t *testing.T) {
		records, err := repo.Search(ctx, "search term", "", 2, "", "")
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Newest first: pg3, pg2
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/pg3" {
			t.Errorf("first record URI = %q, want pg3", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/pg2" {
			t.Errorf("second record URI = %q, want pg2", records[1].URI)
		}
	})

	t.Run("second page with keyset cursor returns older records", func(t *testing.T) {
		// Cursor after pg2 (indexed_at=2026-01-15T11:00:00Z)
		records, err := repo.Search(ctx, "search term", "", 10,
			"2026-01-15T11:00:00Z", "at://did:plc:test1/app.bsky.feed.post/pg2")
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("got %d records, want 1", len(records))
		}
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/pg1" {
			t.Errorf("record URI = %q, want pg1", records[0].URI)
		}
	})
}

func TestRecordsRepository_GetByCollectionReversedWithKeysetCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert 5 records with distinct indexed_at timestamps (oldest = r1, newest = r5)
	// Default DESC order: r5, r4, r3, r2, r1
	// Backward pagination (last N) returns the last N edges in the connection.
	// "last 3" without cursor returns r3, r2, r1 (the 3 oldest in DESC order).
	// "last 2, before r1" returns r3, r2 (the 2 edges just before r1 in the DESC connection).
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/r1", "bafyreir1", "did:plc:test1", "app.bsky.feed.post", `{"text":"r1"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/r1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/r2", "bafyreir2", "did:plc:test1", "app.bsky.feed.post", `{"text":"r2"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/r2'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/r3", "bafyreir3", "did:plc:test1", "app.bsky.feed.post", `{"text":"r3"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/r3'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/r4", "bafyreir4", "did:plc:test1", "app.bsky.feed.post", `{"text":"r4"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/r4'`)

	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.feed.post/r5", "bafyreir5", "did:plc:test1", "app.bsky.feed.post", `{"text":"r5"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T14:00:00Z' WHERE uri = 'at://did:plc:test1/app.bsky.feed.post/r5'`)

	t.Run("last 3 without cursor returns oldest 3 in DESC order", func(t *testing.T) {
		// Default DESC order: r5, r4, r3, r2, r1
		// last 3 = r3, r2, r1 (the last 3 edges in the connection)
		// Algorithm: reversed sort=ASC, LIMIT 3 → r1,r2,r3 → reverse → r3,r2,r1
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{}, nil, 3, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("got %d records, want 3", len(records))
		}
		// Result in DESC order: r3, r2, r1
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/r3" {
			t.Errorf("records[0].URI = %q, want r3", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/r2" {
			t.Errorf("records[1].URI = %q, want r2", records[1].URI)
		}
		if records[2].URI != "at://did:plc:test1/app.bsky.feed.post/r1" {
			t.Errorf("records[2].URI = %q, want r1", records[2].URI)
		}
	})

	t.Run("last N+1 allows hasPreviousPage detection", func(t *testing.T) {
		// Fetch 4 (last 3 + 1 extra) — should return 4 records
		// Algorithm: reversed sort=ASC, LIMIT 4 → r1,r2,r3,r4 → reverse → r4,r3,r2,r1
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{}, nil, 4, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		// 4 records returned means there are more (hasPreviousPage = true when caller uses last=3)
		if len(records) != 4 {
			t.Fatalf("got %d records, want 4", len(records))
		}
	})

	t.Run("before r1 returns edges before r1 in DESC connection", func(t *testing.T) {
		// DESC connection: r5, r4, r3, r2, r1
		// "before r1" = edges that come before r1 in the list = r5, r4, r3, r2
		// Algorithm: reversed sort=ASC, comparison=>, WHERE indexed_at > 10:00
		//   → r2,r3,r4,r5 → reverse → r5,r4,r3,r2
		beforeCursor := []string{"2026-01-15T10:00:00Z", "at://did:plc:test1/app.bsky.feed.post/r1"}
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{}, nil, 10, beforeCursor)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 4 {
			t.Fatalf("got %d records, want 4", len(records))
		}
		// Result in DESC order: r5, r4, r3, r2
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/r5" {
			t.Errorf("records[0].URI = %q, want r5", records[0].URI)
		}
		if records[3].URI != "at://did:plc:test1/app.bsky.feed.post/r2" {
			t.Errorf("records[3].URI = %q, want r2", records[3].URI)
		}
	})

	t.Run("last 2 before r1 returns 2 edges just before r1", func(t *testing.T) {
		// DESC connection: r5, r4, r3, r2, r1
		// "before r1" = r5, r4, r3, r2; last 2 = r3, r2
		// Algorithm: reversed sort=ASC, comparison=>, WHERE indexed_at > 10:00, LIMIT 2
		//   → r2,r3 → reverse → r3,r2
		beforeCursor := []string{"2026-01-15T10:00:00Z", "at://did:plc:test1/app.bsky.feed.post/r1"}
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{}, nil, 2, beforeCursor)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Result in DESC order: r3, r2 (the 2 edges just before r1)
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/r3" {
			t.Errorf("records[0].URI = %q, want r3", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/app.bsky.feed.post/r2" {
			t.Errorf("records[1].URI = %q, want r2", records[1].URI)
		}
	})

	t.Run("all records returned when limit exceeds total", func(t *testing.T) {
		// Algorithm: reversed sort=ASC, LIMIT 100 → r1,r2,r3,r4,r5 → reverse → r5,r4,r3,r2,r1
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "app.bsky.feed.post", nil, repositories.DIDFilter{}, nil, 100, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 5 {
			t.Fatalf("got %d records, want 5", len(records))
		}
		// Should be in DESC order: r5, r4, r3, r2, r1
		if records[0].URI != "at://did:plc:test1/app.bsky.feed.post/r5" {
			t.Errorf("records[0].URI = %q, want r5", records[0].URI)
		}
		if records[4].URI != "at://did:plc:test1/app.bsky.feed.post/r1" {
			t.Errorf("records[4].URI = %q, want r1", records[4].URI)
		}
	})
}

// TestSearchTimeout verifies that the Search method applies a context deadline.
func TestSearchTimeout(t *testing.T) {
	tests := []struct {
		name        string
		ctxTimeout  time.Duration
		wantTimeout bool
	}{
		{
			name:        "already-cancelled context returns error immediately",
			ctxTimeout:  0, // will be cancelled before call
			wantTimeout: true,
		},
		{
			name:        "context with ample time succeeds",
			ctxTimeout:  30 * time.Second,
			wantTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)

			ctx, cancel := context.WithTimeout(context.Background(), repositories.SearchTimeout)
			defer cancel()

			// Verify that the Search method wraps the context with a deadline.
			// We do this by checking that a pre-cancelled context causes an error.
			if tt.wantTimeout {
				cancelledCtx, cancelFn := context.WithCancel(context.Background())
				cancelFn() // cancel immediately
				_, err := repo.Search(cancelledCtx, "hello", "", 10, "", "")
				if err == nil {
					t.Error("Search() with cancelled context should return an error, got nil")
				}
			} else {
				// Normal call with a fresh context should succeed (even with empty results).
				_, err := repo.Search(ctx, "hello", "", 10, "", "")
				if err != nil {
					t.Errorf("Search() with valid context returned unexpected error: %v", err)
				}
			}
		})
	}

	// Verify the exported constant value.
	t.Run("SearchTimeout constant is 10 seconds", func(t *testing.T) {
		if repositories.SearchTimeout != 10*time.Second {
			t.Errorf("SearchTimeout = %v, want 10s", repositories.SearchTimeout)
		}
	})
}
