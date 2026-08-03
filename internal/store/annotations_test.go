package store

import (
	"encoding/json"
	"testing"

	"github.com/jjones/e-biblioteca/internal/models"
)

func TestValidateAnnotationInput_EPUB(t *testing.T) {
	anchor, _ := json.Marshal(map[string]any{"scheme": "epubcfi", "cfi": "epubcfi(/6/4!/4/2)"})
	err := ValidateAnnotationInput("highlight", "yellow", "quote", "note", []string{"tip"}, anchor, "epub")
	if err != nil {
		t.Fatalf("expected ok: %v", err)
	}
}

func TestValidateAnnotationInput_RejectsBadScheme(t *testing.T) {
	anchor, _ := json.Marshal(map[string]any{"scheme": "pdf", "page": 1})
	err := ValidateAnnotationInput("bookmark", "", "", "", nil, anchor, "epub")
	if err == nil {
		t.Fatal("expected error for pdf scheme on epub")
	}
}

func TestValidateAnnotationInput_HighlightNeedsColor(t *testing.T) {
	anchor, _ := json.Marshal(map[string]any{"scheme": "epubcfi", "cfi": "x"})
	err := ValidateAnnotationInput("highlight", "", "q", "", nil, anchor, "epub")
	if err == nil {
		t.Fatal("expected color required")
	}
}

func TestValidateAnnotationInput_PDFAndAudio(t *testing.T) {
	pdf, _ := json.Marshal(map[string]any{"scheme": "pdf", "page": 3.0})
	if err := ValidateAnnotationInput("bookmark", "blue", "", "p", nil, pdf, "pdf"); err != nil {
		t.Fatalf("pdf: %v", err)
	}
	audio, _ := json.Marshal(map[string]any{"scheme": "audio", "seconds": 12.5})
	if err := ValidateAnnotationInput("bookmark", "", "", "mark", nil, audio, "m4b"); err != nil {
		t.Fatalf("audio: %v", err)
	}
}

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" Shell ", "shell", "TIPS", ""})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if got[0] != "Shell" || got[1] != "TIPS" {
		t.Fatalf("got %v", got)
	}
}

func TestAllowedColors(t *testing.T) {
	for _, c := range []string{"yellow", "green", "blue", "pink", "purple"} {
		if !models.AllowedAnnotationColors[c] {
			t.Fatalf("missing color %s", c)
		}
	}
}
