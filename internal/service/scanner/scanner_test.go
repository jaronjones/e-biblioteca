package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsUnderBooks(t *testing.T) {
	root := t.TempDir()
	s := &Scanner{BooksDir: root}

	if !s.IsUnderBooks(root) {
		t.Fatal("root itself should be under books")
	}
	nested := filepath.Join(root, "fiction", "a")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if !s.IsUnderBooks(nested) {
		t.Fatal("nested path should be under books")
	}
	if s.IsUnderBooks(filepath.Join(root, "..", "outside")) {
		t.Fatal("sibling of books dir must be rejected")
	}
	if s.IsUnderBooks("/etc") {
		t.Fatal("absolute system path must be rejected")
	}
}
