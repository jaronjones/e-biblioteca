package models

import (
	"encoding/json"
	"testing"
)

func TestMagicRulesJSON(t *testing.T) {
	r := MagicRules{Author: "Ada", Format: "epub"}
	raw := r.JSON()
	var out MagicRules
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Author != "Ada" || out.Format != "epub" {
		t.Fatalf("unexpected %#v", out)
	}
}
