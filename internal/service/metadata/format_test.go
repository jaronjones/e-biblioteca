package metadata

import "testing"

func TestFormatFromPath(t *testing.T) {
	cases := map[string]string{
		"a.epub": "epub",
		"b.PDF":  "pdf",
		"c.cbz":  "cbz",
		"d.mp3":  "mp3",
		"e.m4b":  "m4b",
		"g.m4a":  "m4a",
		"h.opus": "opus",
		"f.txt":  "",
	}
	for path, want := range cases {
		got, ok := FormatFromPath(path)
		if want == "" {
			if ok {
				t.Errorf("%s: expected unsupported, got %s", path, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("%s: got %s ok=%v want %s", path, got, ok, want)
		}
	}
}
