package store

import "testing"

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		`plain`: `plain`,
		`100%`:  `100\%`,
		`a_b`:   `a\_b`,
		`a\b`:   `a\\b`,
		`%_\`:   `\%\_\\`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q)=%q want %q", in, got, want)
		}
	}
}
