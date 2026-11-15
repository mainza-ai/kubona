package jobs

import "testing"

func TestNormalizeBackendClosedSet(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"local_simulator", "local_simulator", false},
		{"qiskit-aer-simulator", "local_simulator", false},
		{"Local Simulator (qiskit-aer-simulator)", "local_simulator", false},
		{"simulator-first", "local_simulator", false},
		{"ibm_brisbane", "ibm_brisbane", false},
		{"ibm_fez", "ibm_fez", false},
		{"quantinuum_h1", "quantinuum_h1", false},
		{"rigetti_aspen", "rigetti_aspen", false},
		// A provider-prefixed name is metered even when it contains a
		// simulator-sounding word: the hardware profile wins.
		{"ibm_simulator", "ibm_simulator", false},
		{"", "", true},
		{"   ", "", true},
		{"mystery_box", "", true},
		{"vibes", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeBackend(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q must refuse, got %q", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q must normalize, got error %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("%q normalized to %q, want %q", tc.raw, got, tc.want)
		}
	}
}
