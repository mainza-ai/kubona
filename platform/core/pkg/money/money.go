// Package money provides exact micro-USD arithmetic at the storage boundary.
// Float64 remains the wire/UI type; Go's spend books store integer micro-USD
// so reserve/release/refund cannot drift by float dust.
package money

import (
	"fmt"
	"math"
)

const (
	// MicroPerUSD is the fixed scale (6 decimal places).
	MicroPerUSD int64 = 1_000_000
	// MaxMicroUSD bounds a single row to $1e9 (9e15 micro) — far above any
	// real lab ceiling, safely inside int64.
	MaxMicroUSD int64 = 1_000_000_000 * MicroPerUSD
)

// MicroUSD is an exact integer amount in millionths of a US dollar.
type MicroUSD int64

// FromUSD converts a finite dollar amount to micro-USD, rounding half away
// from zero. NaN/Inf and out-of-range values refuse.
func FromUSD(v float64) (MicroUSD, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("amount must be finite, got %v", v)
	}
	scaled := math.Round(v * float64(MicroPerUSD))
	if scaled > float64(MaxMicroUSD) || scaled < -float64(MaxMicroUSD) {
		return 0, fmt.Errorf("amount %v exceeds the supported range", v)
	}
	return MicroUSD(int64(scaled)), nil
}

// MustFromUSD is for literals/known-valid test values only.
func MustFromUSD(v float64) MicroUSD {
	m, err := FromUSD(v)
	if err != nil {
		panic(err)
	}
	return m
}

// USD converts back to the wire/UI float at 6-decimal precision.
func (m MicroUSD) USD() float64 { return float64(m) / float64(MicroPerUSD) }

// Add returns m+other, refusing int64 overflow.
func (m MicroUSD) Add(other MicroUSD) (MicroUSD, error) {
	sum := int64(m) + int64(other)
	if (other > 0 && sum < int64(m)) || (other < 0 && sum > int64(m)) {
		return 0, fmt.Errorf("micro-USD addition overflow")
	}
	if sum > MaxMicroUSD || sum < -MaxMicroUSD {
		return 0, fmt.Errorf("micro-USD amount exceeds the supported range")
	}
	return MicroUSD(sum), nil
}

// Sub returns m-other, refusing int64 overflow.
func (m MicroUSD) Sub(other MicroUSD) (MicroUSD, error) { return m.Add(-other) }
