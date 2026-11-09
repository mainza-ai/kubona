package main

import "testing"

func TestOffhostDrillRefusesIncompleteInputs(t *testing.T) {
	cases := [][]string{
		{},
		{"--db", "relative.db"},
		{"--db", "/tmp/live.db"},
		{"--db", "/tmp/live.db", "--offhost-dir", "/tmp/offhost"},
	}
	for _, args := range cases {
		if code := runOffhostDrill(args); code == 0 {
			t.Fatalf("incomplete drill args %v must refuse", args)
		}
	}
}
