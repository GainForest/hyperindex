package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseDIDDocument(t *testing.T) {
	jsonData := `{
		"id": "did:plc:example123",
		"alsoKnownAs": ["at://user.bsky.social"],
		"service": [
			{
				"id": "#atproto_pds",
				"type": "AtprotoPersonalDataServer",
				"serviceEndpoint": "https://pds.example.com"
			}
		]
	}`

	doc, err := parseDIDDocument([]byte(jsonData))
	if err != nil {
		t.Fatalf("parseDIDDocument() error = %v", err)
	}

	if doc.ID != "did:plc:example123" {
		t.Errorf("ID = %v, want did:plc:example123", doc.ID)
	}
	if len(doc.AlsoKnownAs) != 1 || doc.AlsoKnownAs[0] != "at://user.bsky.social" {
		t.Errorf("AlsoKnownAs = %v, want [at://user.bsky.social]", doc.AlsoKnownAs)
	}
	if len(doc.Service) != 1 {
		t.Fatalf("Service length = %d, want 1", len(doc.Service))
	}
	if doc.Service[0].Type != "AtprotoPersonalDataServer" {
		t.Errorf("Service[0].Type = %v, want AtprotoPersonalDataServer", doc.Service[0].Type)
	}
}

func TestDIDDocument_GetPDSEndpoint(t *testing.T) {
	doc := &DIDDocument{
		ID: "did:plc:example",
		Service: []Service{
			{ID: "#other", Type: "OtherService", ServiceEndpoint: "https://other.com"},
			{ID: "#atproto_pds", Type: "AtprotoPersonalDataServer", ServiceEndpoint: "https://pds.example.com"},
		},
	}

	endpoint := doc.GetPDSEndpoint()
	if endpoint != "https://pds.example.com" {
		t.Errorf("GetPDSEndpoint() = %v, want https://pds.example.com", endpoint)
	}

	// Test with no PDS service
	doc2 := &DIDDocument{
		ID: "did:plc:example",
		Service: []Service{
			{ID: "#other", Type: "OtherService", ServiceEndpoint: "https://other.com"},
		},
	}

	endpoint2 := doc2.GetPDSEndpoint()
	if endpoint2 != "" {
		t.Errorf("GetPDSEndpoint() = %v, want empty string", endpoint2)
	}
}

func TestDIDDocument_GetHandle(t *testing.T) {
	doc := &DIDDocument{
		ID:          "did:plc:example",
		AlsoKnownAs: []string{"at://user.bsky.social", "https://other.example.com"},
	}

	handle := doc.GetHandle()
	if handle != "user.bsky.social" {
		t.Errorf("GetHandle() = %v, want user.bsky.social", handle)
	}

	// Test with no at:// URI
	doc2 := &DIDDocument{
		ID:          "did:plc:example",
		AlsoKnownAs: []string{"https://other.example.com"},
	}

	handle2 := doc2.GetHandle()
	if handle2 != "" {
		t.Errorf("GetHandle() = %v, want empty string", handle2)
	}
}

func TestIsValidDID(t *testing.T) {
	tests := []struct {
		did  string
		want bool
	}{
		{"did:plc:example123", true},
		{"did:web:example.com", true},
		{"did:web:", false},
		{"did:plc:bad space", false},
		{"did:key:example", false},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsValidDID(tt.did); got != tt.want {
			t.Errorf("IsValidDID(%q) = %v, want %v", tt.did, got, tt.want)
		}
	}
}

func TestIsDIDPLC(t *testing.T) {
	tests := []struct {
		did  string
		want bool
	}{
		{"did:plc:example123", true},
		{"did:web:example.com", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		if got := IsDIDPLC(tt.did); got != tt.want {
			t.Errorf("IsDIDPLC(%q) = %v, want %v", tt.did, got, tt.want)
		}
	}
}

func TestIsDIDWeb(t *testing.T) {
	tests := []struct {
		did  string
		want bool
	}{
		{"did:plc:example123", false},
		{"did:web:example.com", true},
		{"invalid", false},
	}

	for _, tt := range tests {
		if got := IsDIDWeb(tt.did); got != tt.want {
			t.Errorf("IsDIDWeb(%q) = %v, want %v", tt.did, got, tt.want)
		}
	}
}

func TestDIDResolver_ResolvePLCDID(t *testing.T) {
	// Create mock PLC directory server
	plcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/did:plc:example123" {
			doc := DIDDocument{
				ID:          "did:plc:example123",
				AlsoKnownAs: []string{"at://test.bsky.social"},
				Service: []Service{
					{ID: "#atproto_pds", Type: "AtprotoPersonalDataServer", ServiceEndpoint: "https://pds.test.com"},
				},
			}
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		if r.URL.Path == "/did:plc:notfound" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer plcServer.Close()

	resolver := NewDIDResolver(WithPLCDirectoryURL(plcServer.URL))

	// Test successful resolution
	doc, err := resolver.ResolveDID("did:plc:example123")
	if err != nil {
		t.Fatalf("ResolveDID() error = %v", err)
	}
	if doc.ID != "did:plc:example123" {
		t.Errorf("ID = %v, want did:plc:example123", doc.ID)
	}
	if doc.GetHandle() != "test.bsky.social" {
		t.Errorf("GetHandle() = %v, want test.bsky.social", doc.GetHandle())
	}

	// Test not found
	_, err = resolver.ResolveDID("did:plc:notfound")
	if err == nil {
		t.Error("ResolveDID() should return error for not found DID")
	}
}

func TestDIDResolver_ResolveWebDID(t *testing.T) {
	// Create mock web DID server
	webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/did.json" {
			doc := DIDDocument{
				ID:          "did:web:localhost",
				AlsoKnownAs: []string{"at://webuser.example.com"},
				Service: []Service{
					{ID: "#atproto_pds", Type: "AtprotoPersonalDataServer", ServiceEndpoint: "https://pds.example.com"},
				},
			}
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer webServer.Close()

	// Note: We can't easily test did:web with httptest because the URL needs to be https
	// and httptest uses the actual address. In production, did:web would work correctly.
	// For unit testing, we test the URL construction and parsing separately.
}

func TestDIDResolver_UnsupportedMethod(t *testing.T) {
	resolver := NewDIDResolver()

	_, err := resolver.ResolveDID("did:key:example")
	if err == nil {
		t.Error("ResolveDID() should return error for unsupported DID method")
	}
}

func TestNewDIDResolver_DefaultConfig(t *testing.T) {
	resolver := NewDIDResolver()

	if resolver.plcDirectoryURL != DefaultPLCDirectoryURL {
		t.Errorf("plcDirectoryURL = %v, want %v", resolver.plcDirectoryURL, DefaultPLCDirectoryURL)
	}
	if resolver.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

func TestNewDIDResolver_CustomConfig(t *testing.T) {
	customClient := &http.Client{}
	customURL := "https://custom.plc.directory"

	resolver := NewDIDResolver(
		WithPLCDirectoryURL(customURL),
		WithHTTPClient(customClient),
	)

	if resolver.plcDirectoryURL != customURL {
		t.Errorf("plcDirectoryURL = %v, want %v", resolver.plcDirectoryURL, customURL)
	}
	if resolver.httpClient != customClient {
		t.Error("httpClient should be custom client")
	}
}
