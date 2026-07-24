package metadata

import "testing"

func TestFormatFromPath(t *testing.T) {
	cases := map[string]string{
		"a.epub": "epub",
		"b.PDF":  "pdf",
		"c.cbz":  "cbz",
		"d.mp3":  "mp3",
		"e.m4b":  "m4b",
		"f.txt":  "",
	}
	for path, want := range cases {
		got, ok := FormatFromPath(path)
		if want == "" {
			if ok {
				t.Fatalf("%s: expected unsupported, got %s", path, got)
			}
			continue
		}
		if !ok || got != want {
			t.Fatalf("%s: got %s ok=%v want %s", path, got, ok, want)
		}
	}
}
