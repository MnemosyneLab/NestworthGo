package domain

import "testing"

func TestExplicitTrendRange(t *testing.T) {
	for _, value := range []string{"2026-09-21:2026-09-27", "2024-02-29:2024-02-29"} {
		r, err := ParseTrendRange(value)
		if err != nil {
			t.Fatal(err)
		}
		from, to, ok := r.DateBounds()
		if !ok || from+":"+to != value {
			t.Fatalf("invalid bounds: %s %s", from, to)
		}
	}
	for _, value := range []string{"2026-02-29:2026-03-01", "2026-09-27:2026-09-21", "garbage", "2026-09-21:", "0000-01-01:2026-01-01"} {
		if _, err := ParseTrendRange(value); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
