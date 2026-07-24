package models

import (
	"encoding/json"
	"testing"
)

func TestMagicRulesJSON(t *testing.T) {
	r := MagicRules{Author: "Ada", Format: "epub", Query: "computing"}
	raw := r.JSON()
	var out MagicRules
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Author != "Ada" || out.Format != "epub" || out.Query != "computing" {
		t.Fatalf("unexpected %#v", out)
	}
}

func TestUserCanAdmin(t *testing.T) {
	if !(User{IsAdmin: true}).CanAdmin() {
		t.Fatal("admin should CanAdmin")
	}
	if (User{IsAdmin: false}).CanAdmin() {
		t.Fatal("non-admin should not CanAdmin")
	}
}
