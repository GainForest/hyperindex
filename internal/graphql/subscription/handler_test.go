package subscription

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMakeOriginChecker(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		want           bool
	}{
		{
			name:           "nil origins reject cross-origin",
			allowedOrigins: nil,
			requestOrigin:  "https://example.com",
			want:           false,
		},
		{
			name:           "empty origins reject cross-origin",
			allowedOrigins: []string{},
			requestOrigin:  "https://example.com",
			want:           false,
		},
		{
			name:           "empty origins allow same-origin",
			allowedOrigins: []string{},
			requestOrigin:  "https://api.example",
			want:           true,
		},
		{
			name:           "wildcard allows all",
			allowedOrigins: []string{"*"},
			requestOrigin:  "https://example.com",
			want:           true,
		},
		{
			name:           "no origin header always allowed",
			allowedOrigins: []string{"https://allowed.com"},
			requestOrigin:  "",
			want:           true,
		},
		{
			name:           "matching origin allowed",
			allowedOrigins: []string{"https://allowed.com"},
			requestOrigin:  "https://allowed.com",
			want:           true,
		},
		{
			name:           "non-matching origin rejected",
			allowedOrigins: []string{"https://allowed.com"},
			requestOrigin:  "https://evil.com",
			want:           false,
		},
		{
			name:           "multiple origins one matches",
			allowedOrigins: []string{"https://a.com", "https://b.com"},
			requestOrigin:  "https://b.com",
			want:           true,
		},
		{
			name:           "multiple origins none match",
			allowedOrigins: []string{"https://a.com", "https://b.com"},
			requestOrigin:  "https://c.com",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := makeOriginChecker(tt.allowedOrigins)
			req := httptest.NewRequest(http.MethodGet, "https://api.example/graphql/ws", nil)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
			got := checker(req)
			if got != tt.want {
				t.Errorf("makeOriginChecker(%v) with origin %q = %v, want %v",
					tt.allowedOrigins, tt.requestOrigin, got, tt.want)
			}
		})
	}
}
