package money

import (
	"math"
	"testing"
)

func TestFromUSDExactAndRounding(t *testing.T) {
	cases := []struct {
		in   float64
		want MicroUSD
	}{
		{0, 0},
		{0.000001, 1},
		{0.0000014, 1},
		{0.0000015, 2},
		{1.25, 1_250_000},
		{150, 150_000_000},
		{-0.5, -500_000},
	}
	for _, c := range cases {
		got, err := FromUSD(c.in)
		if err != nil || got != c.want {
			t.Fatalf("FromUSD(%v) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
}

func TestFromUSDRefusesNonFiniteAndHuge(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e12} {
		if _, err := FromUSD(v); err == nil {
			t.Fatalf("FromUSD(%v) must refuse", v)
		}
	}
}

func TestAddSubNoDrift(t *testing.T) {
	start := MustFromUSD(0)
	for i := 0; i < 10_000; i++ {
		next, err := start.Add(MustFromUSD(0.1))
		if err != nil {
			t.Fatal(err)
		}
		start = next
	}
	if start != MustFromUSD(1000) {
		t.Fatalf("10k x $0.1 must equal $1000 exactly, got %v", start.USD())
	}
	back, err := start.Sub(MustFromUSD(1000))
	if err != nil || back != 0 {
		t.Fatalf("sub must return to zero: %v %v", back, err)
	}
}
