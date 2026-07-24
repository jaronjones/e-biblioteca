package theme

import "testing"

func TestNormalize(t *testing.T) {
	if Normalize("dracula") != "dracula" {
		t.Fatal("expected dracula")
	}
	if Normalize("nope") != Default {
		t.Fatalf("expected default %s", Default)
	}
}
