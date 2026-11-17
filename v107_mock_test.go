//go:build !windows

package main

import "testing"

func TestAutoBoost(t *testing.T) {
	t.Setenv("LARIK_DATA", t.TempDir())
	cat, _ := LoadCatalog(catalogJSON)
	sys := newMockSys(cat)
	if err := startAutoBoost(sys, "CS2.exe"); err != nil {
		t.Fatal(err)
	}
	if b := loadBoost(); b == nil || b.Auto != "cs2.exe" {
		t.Fatalf("%+v", b)
	}
	if err := endAutoBoost(sys); err != nil || loadBoost() != nil {
		t.Fatal("auto boost not ended")
	}
	// Boost started by hand is left alone.
	if _, err := setBoost(sys, true); err != nil {
		t.Fatal(err)
	}
	_ = startAutoBoost(sys, "cs2.exe")
	if b := loadBoost(); b == nil || b.Auto != "" {
		t.Fatalf("manual boost taken over: %+v", b)
	}
	_ = endAutoBoost(sys)
	if loadBoost() == nil {
		t.Fatal("manual boost switched off")
	}
	_, _ = setBoost(sys, false)
}
