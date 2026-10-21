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

func TestParseVerifiedReportPins(t *testing.T) {
	rep, err := ParseVerifiedReport()
	if err != nil {
		t.Fatalf("embedded verified report must parse: %v", err)
	}
	if rep.Total.Toffoli != 3928980 || rep.Total.Qubits != 112168 {
		t.Fatalf("verified total must pin 3928980T x 112168Q, got %dT x %dQ",
			rep.Total.Toffoli, rep.Total.Qubits)
	}
	if rep.Version < 1 || len(rep.Components) == 0 || len(rep.Proofs) == 0 {
		t.Fatal("verified report must carry version, components, and proofs")
	}
	seen := map[string]bool{}
	for _, c := range rep.Components {
		if c.Toffoli <= 0 || c.Pin == "" || c.Scope == "" {
			t.Fatalf("component %q must carry T-count, scope, and pin", c.Name)
		}
		seen[c.Name] = true
	}
	for _, want := range []string{"bm_walk_70r", "eea_36b", "chien_scan", "phase_scan", "omega"} {
		if !seen[want] {
			t.Fatalf("verified report must include component %q", want)
		}
	}
}
