package buildinfo

import "testing"

func TestVersionDefault(t *testing.T) {
	if Version != "0.1.0-dev" {
		t.Fatalf("Version = %q, want %q", Version, "0.1.0-dev")
	}
}
