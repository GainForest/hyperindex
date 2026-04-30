package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/buildinfo"
	"github.com/GainForest/hyperindex/internal/config"
)

func TestRootEndpointReturnsBuildInfoVersion(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "v9.9.9-test"
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
	})

	r := setupRouter(&config.Config{ExternalBaseURL: "https://example.com"}, &services{}, &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET / response: %v", err)
	}
	if body["version"] != buildinfo.Version {
		t.Fatalf("version = %q, want buildinfo.Version %q", body["version"], buildinfo.Version)
	}
}

func TestApplyTapSidecarHealth(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		healthFn  func(context.Context) error
		wantState string
	}{
		{
			name:    "sidecar healthy",
			timeout: 50 * time.Millisecond,
			healthFn: func(context.Context) error {
				return nil
			},
			wantState: "ok",
		},
		{
			name:    "sidecar returns error",
			timeout: 50 * time.Millisecond,
			healthFn: func(context.Context) error {
				return errors.New("sidecar unavailable")
			},
			wantState: "unreachable",
		},
		{
			name:    "sidecar health times out",
			timeout: 10 * time.Millisecond,
			healthFn: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			wantState: "unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tapInfo := map[string]any{}
			applyTapSidecarHealth(context.Background(), tapInfo, tt.timeout, tt.healthFn)

			gotState, ok := tapInfo["sidecar"].(string)
			if !ok {
				t.Fatalf("sidecar state missing or non-string: %#v", tapInfo["sidecar"])
			}
			if gotState != tt.wantState {
				t.Fatalf("sidecar state = %q, want %q", gotState, tt.wantState)
			}

			if _, hasErr := tapInfo["sidecar_error"]; hasErr {
				t.Fatalf("unexpected sidecar_error in stats payload")
			}
		})
	}
}
