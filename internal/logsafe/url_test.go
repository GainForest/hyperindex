package logsafe

import "testing"

func TestURLRedactsSensitiveParts(t *testing.T) {
	got := URL("wss://user:pass@labeler.example:443/labels?token=secret#fragment")
	want := "wss://labeler.example:443/labels"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestURLRejectsInvalidOrHostlessValues(t *testing.T) {
	for _, raw := range []string{"%", "not-a-url", "/relative/path"} {
		if got := URL(raw); got != "<invalid-url>" {
			t.Fatalf("URL(%q) = %q, want <invalid-url>", raw, got)
		}
	}
}

func TestURLsReturnsRedactedCopy(t *testing.T) {
	raw := []string{"wss://user:pass@one.example/labels?token=secret", "wss://two.example/labels"}
	got := URLs(raw)
	want := []string{"wss://one.example/labels", "wss://two.example/labels"}

	if len(got) != len(want) {
		t.Fatalf("URLs() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("URLs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if raw[0] != "wss://user:pass@one.example/labels?token=secret" {
		t.Fatalf("URLs() mutated input: %#v", raw)
	}
}
