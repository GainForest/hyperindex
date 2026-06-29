package tap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAdminClient_AddRepos verifies POST /repos/add sends the correct JSON body.
func TestAdminClient_AddRepos(t *testing.T) {
	tests := []struct {
		name     string
		dids     []string
		password string
		status   int
		wantErr  bool
	}{
		{
			name:    "success with multiple DIDs",
			dids:    []string{"did:plc:abc", "did:plc:def"},
			status:  http.StatusOK,
			wantErr: false,
		},
		{
			name:    "success with single DID",
			dids:    []string{"did:plc:abc"},
			status:  http.StatusOK,
			wantErr: false,
		},
		{
			name:    "server error returns error",
			dids:    []string{"did:plc:abc"},
			status:  http.StatusInternalServerError,
			wantErr: true,
		},
		{
			name:    "bad request returns error",
			dids:    []string{"did:plc:abc"},
			status:  http.StatusBadRequest,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody didsBody
			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					http.Error(w, "bad body", http.StatusBadRequest)
					return
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := NewAdminClient(srv.URL, tt.password)
			err := c.AddRepos(context.Background(), tt.dids)

			if (err != nil) != tt.wantErr {
				t.Errorf("AddRepos() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if gotPath != "/repos/add" {
					t.Errorf("expected path /repos/add, got %s", gotPath)
				}
				if len(gotBody.DIDs) != len(tt.dids) {
					t.Errorf("expected %d DIDs, got %d", len(tt.dids), len(gotBody.DIDs))
				}
				for i, did := range tt.dids {
					if gotBody.DIDs[i] != did {
						t.Errorf("DID[%d]: expected %s, got %s", i, did, gotBody.DIDs[i])
					}
				}
			}
		})
	}
}

// TestAdminClient_RemoveRepos verifies POST /repos/remove sends the correct JSON body.
func TestAdminClient_RemoveRepos(t *testing.T) {
	tests := []struct {
		name    string
		dids    []string
		status  int
		wantErr bool
	}{
		{
			name:    "success",
			dids:    []string{"did:plc:abc"},
			status:  http.StatusOK,
			wantErr: false,
		},
		{
			name:    "server error returns error",
			dids:    []string{"did:plc:abc"},
			status:  http.StatusInternalServerError,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody didsBody
			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					http.Error(w, "bad body", http.StatusBadRequest)
					return
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := NewAdminClient(srv.URL, "")
			err := c.RemoveRepos(context.Background(), tt.dids)

			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveRepos() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if gotPath != "/repos/remove" {
					t.Errorf("expected path /repos/remove, got %s", gotPath)
				}
				if len(gotBody.DIDs) != len(tt.dids) {
					t.Errorf("expected %d DIDs, got %d", len(tt.dids), len(gotBody.DIDs))
				}
			}
		})
	}
}

// TestAdminClient_Health verifies GET /health and status parsing.
func TestAdminClient_Health(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
	}{
		{
			name:       "healthy",
			statusCode: http.StatusOK,
			body:       `{"status":"ok"}`,
			wantErr:    false,
		},
		{
			name:       "unhealthy status value",
			statusCode: http.StatusOK,
			body:       `{"status":"degraded"}`,
			wantErr:    true,
		},
		{
			name:       "non-200 response",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"status":"ok"}`,
			wantErr:    true,
		},
		{
			name:       "invalid JSON body",
			statusCode: http.StatusOK,
			body:       `not-json`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := NewAdminClient(srv.URL, "")
			err := c.Health(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("Health() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAdminClient_RepoInfo verifies GET /info/:did.
func TestAdminClient_RepoInfo(t *testing.T) {
	tests := []struct {
		name       string
		did        string
		statusCode int
		body       string
		wantErr    bool
		wantDID    string
	}{
		{
			name:       "success",
			did:        "did:plc:abc",
			statusCode: http.StatusOK,
			body:       `{"did":"did:plc:abc","status":"active","rev":"abc123"}`,
			wantErr:    false,
			wantDID:    "did:plc:abc",
		},
		{
			name:       "not found",
			did:        "did:plc:unknown",
			statusCode: http.StatusNotFound,
			body:       `{"error":"not found"}`,
			wantErr:    true,
		},
		{
			name:       "invalid JSON",
			did:        "did:plc:abc",
			statusCode: http.StatusOK,
			body:       `not-json`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := NewAdminClient(srv.URL, "")
			info, err := c.RepoInfo(context.Background(), tt.did)

			if (err != nil) != tt.wantErr {
				t.Errorf("RepoInfo() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				expectedPath := "/info/" + tt.did
				if gotPath != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, gotPath)
				}
				if info == nil {
					t.Fatal("expected non-nil RepoInfoResponse")
					return
				}
				if info.DID != tt.wantDID {
					t.Errorf("DID: expected %s, got %s", tt.wantDID, info.DID)
				}
			}
		})
	}
}

// TestAdminClient_BasicAuth verifies auth header behavior.
func TestAdminClient_BasicAuth(t *testing.T) {
	tests := []struct {
		name       string
		password   string
		wantAuth   bool
		wantHeader string
	}{
		{
			name:     "no auth when password is empty",
			password: "",
			wantAuth: false,
		},
		{
			name:       "basic auth header when password is set",
			password:   "secret",
			wantAuth:   true,
			wantHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuthHeader string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuthHeader = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
			defer srv.Close()

			c := NewAdminClient(srv.URL, tt.password)
			_ = c.Health(context.Background())

			if tt.wantAuth {
				if gotAuthHeader == "" {
					t.Error("expected Authorization header, got none")
				}
				if gotAuthHeader != tt.wantHeader {
					t.Errorf("Authorization header: expected %q, got %q", tt.wantHeader, gotAuthHeader)
				}
			} else {
				if gotAuthHeader != "" {
					t.Errorf("expected no Authorization header, got %q", gotAuthHeader)
				}
			}
		})
	}
}

// TestAdminClient_ContentType verifies POST requests set Content-Type: application/json.
func TestAdminClient_ContentType(t *testing.T) {
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL, "")
	_ = c.AddRepos(context.Background(), []string{"did:plc:abc"})

	if !strings.Contains(gotContentType, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", gotContentType)
	}
}
