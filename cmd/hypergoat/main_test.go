package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestApplyTapSidecarHealth(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		healthFn  func(context.Context) error
		wantState string
		wantErr   string
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
			wantErr:   "sidecar unavailable",
		},
		{
			name:    "sidecar health times out",
			timeout: 10 * time.Millisecond,
			healthFn: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			wantState: "unreachable",
			wantErr:   "context deadline exceeded",
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

			gotErr, hasErr := tapInfo["sidecar_error"].(string)
			if tt.wantErr == "" {
				if hasErr {
					t.Fatalf("unexpected sidecar_error = %q", gotErr)
				}
				return
			}

			if !hasErr {
				t.Fatalf("expected sidecar_error containing %q, got none", tt.wantErr)
			}
			if !strings.Contains(gotErr, tt.wantErr) {
				t.Fatalf("sidecar_error = %q, want substring %q", gotErr, tt.wantErr)
			}
		})
	}
}
