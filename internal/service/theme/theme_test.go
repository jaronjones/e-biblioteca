package theme

import "testing"

func TestNormalize(t *testing.T) {
	if got := Normalize("dracula"); got != "dracula" {
		t.Fatalf("got %q", got)
	}
	if got := Normalize("nope"); got != Default {
		t.Fatalf("expected default %s, got %q", Default, got)
	}
	if got := Normalize(""); got != Default {
		t.Fatalf("empty should default to %s, got %q", Default, got)
	}
}

func TestNamesNonEmpty(t *testing.T) {
	if len(Names) == 0 {
		t.Fatal("expected theme names")
	}
	if !Valid(Default) {
		t.Fatalf("default theme %q must be valid", Default)
	}
}
