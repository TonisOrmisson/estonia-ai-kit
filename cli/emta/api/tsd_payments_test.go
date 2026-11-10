package api

import "testing"

func TestCheckTSDHeader(t *testing.T) {
	x := []byte("<tsd_vorm><c108_Aasta>2026</c108_Aasta><c109_Kuu>10</c109_Kuu><regKood>12345678</regKood></tsd_vorm>")
	if err := checkTSDHeader(x, "12345678", 2026, 10); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		reg  string
		y, m int
	}{{"87654321", 2026, 10}, {"12345678", 2025, 10}, {"12345678", 2026, 9}} {
		if checkTSDHeader(x, c.reg, c.y, c.m) == nil {
			t.Fatalf("expected mismatch for %+v", c)
		}
	}
	if checkTSDHeader([]byte("<tsd_vorm/>"), "12345678", 2026, 10) == nil {
		t.Fatal("expected error for missing header")
	}
}
