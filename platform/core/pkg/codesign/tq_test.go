package codesign

import "testing"

func TestGoldenTQ(t *testing.T) {
	v, err := Cost(5, 10, 1)
	if err != nil || v != 50 {
		t.Fatalf("golden 5x10=50, got %d err %v", v, err)
	}
	if _, err := Cost(5, 10, 0); err == nil {
		t.Fatal("kappa 0 must refuse")
	}
	if _, err := Cost(5, 10, 4); err == nil {
		t.Fatal("kappa 4 must refuse")
	}
	if _, err := Cost(-1, 10, 1); err == nil {
		t.Fatal("negative T must refuse")
	}
}

func TestFrontierKeepsMinTQ(t *testing.T) {
	f := NewFrontier(2, 2, 1)
	ok, err := f.Offer(4, 4)
	if err != nil || !ok {
		t.Fatalf("first offer must land: %v", err)
	}
	ok, _ = f.Offer(7, 7)
	if ok {
		t.Fatal("same-cell worse TQ must not displace")
	}
	if b := f.Best(); b == nil || b.Toffoli != 4 {
		t.Fatalf("best must be 4T entry, got %+v", b)
	}
}
