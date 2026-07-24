package metadata

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool // a < b
	}{
		{"page1.png", "page2.png", true},
		{"page2.png", "page10.png", true},
		{"page10.png", "page2.png", false},
		{"page9.png", "page10.png", true},
		{"a", "b", true},
	}
	for _, tc := range cases {
		if got := naturalLess(tc.a, tc.b); got != tc.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestListCBZPagesNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "comic.cbz")
	// Store out of natural order: page10, page2, page1
	if err := writeTestCBZ(path, []string{"page10.png", "page2.png", "page1.png"}); err != nil {
		t.Fatal(err)
	}
	pages, err := ListCBZPages(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"page1.png", "page2.png", "page10.png"}
	if len(pages) != len(want) {
		t.Fatalf("got %v, want %v", pages, want)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Fatalf("pages[%d]=%q, want %q (full %v)", i, pages[i], want[i], pages)
		}
	}
}

func TestExtractCBZCoverUsesNaturalFirstPage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "comic.cbz")
	// Distinct 1x1 PNG payloads so we can tell which file became the cover.
	// page10 first in archive, page1 last — cover must still be page1.
	payloads := map[string][]byte{
		"page10.png": tinyPNG(0xFF, 0x00, 0x00), // red-ish marker (not real color, unique bytes)
		"page2.png":  tinyPNG(0x00, 0xFF, 0x00),
		"page1.png":  tinyPNG(0x00, 0x00, 0xFF),
	}
	if err := writeTestCBZPayloads(path, []string{"page10.png", "page2.png", "page1.png"}, payloads); err != nil {
		t.Fatal(err)
	}
	meta, err := extractCBZ(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(meta.CoverData, payloads["page1.png"]) {
		t.Fatalf("cover not from natural first page (page1.png); got %d bytes", len(meta.CoverData))
	}
}

func writeTestCBZ(path string, names []string) error {
	payloads := map[string][]byte{}
	for i, n := range names {
		payloads[n] = tinyPNG(byte(i+1), 0, 0)
	}
	return writeTestCBZPayloads(path, names, payloads)
}

func writeTestCBZPayloads(path string, order []string, payloads map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for _, name := range order {
		fw, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err := fw.Write(payloads[name]); err != nil {
			return err
		}
	}
	return w.Close()
}

// tinyPNG returns a minimal valid 1x1 PNG with a unique IDAT-ish payload marker in the file.
// We only need distinct byte sequences for cover identity checks, not perfect images.
func tinyPNG(r, g, b byte) []byte {
	// Minimal PNG signature + IHDR-ish garbage is enough for cover identity tests;
	// extractCBZ does not decode the image, only reads bytes.
	sig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	return append(sig, r, g, b, 0xAA, 0xBB, 0xCC)
}
