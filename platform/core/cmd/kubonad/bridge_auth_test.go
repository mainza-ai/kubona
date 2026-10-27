package main

import "testing"

func TestSessionAssertionVector(t *testing.T) {
	// Shared vector with platform/bridge/test_quantum_rpc.py.
	got := sessionAssertionMAC("k", "s1", 25.0, 0.25, "lab")
	want := "4e0b142452c9b1d33d7d916a409ef2434b24a97d1c6a9d4acdc842178530c7ea"
	if got != want {
		t.Fatalf("session assertion drifted from the Python vector: got %s want %s", got, want)
	}
}
