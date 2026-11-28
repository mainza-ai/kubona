package knowledge

import "testing"

func TestNormalizeAndFingerprintAreStable(t *testing.T) {
	a := "The optimizer is trapped in a local minimum for H2."
	b := "optimizer, trapped -- LOCAL minimum H2 the"
	if Normalize(a) != Normalize(b) {
		t.Fatalf("normalize differs: %q vs %q", Normalize(a), Normalize(b))
	}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("equivalent statements must share a fingerprint")
	}
	if Fingerprint(a) == Fingerprint("A completely different probe about shot noise") {
		t.Fatal("different statements collided")
	}
}

func TestSimilarity(t *testing.T) {
	if got := Similarity("The optimizer is trapped in a local minimum", "The optimizer is trapped in a local minimum"); got != 1 {
		t.Fatalf("identical similarity = %v", got)
	}
	if got := Similarity("The optimizer is trapped", "A larger active space would move the residual"); got != 0 {
		t.Fatalf("disjoint similarity = %v", got)
	}
	partial := Similarity("The optimizer is trapped in a local minimum for H2",
		"The optimizer is trapped in a minimum for H2 at this geometry")
	if partial <= 0.5 || partial >= 1 {
		t.Fatalf("partial similarity = %v", partial)
	}
}

func TestFingerprintIsPolarityAware(t *testing.T) {
	pos := "BP-OSD decoding was effective on the gross code."
	neg := "BP-OSD decoding was not effective on the gross code."
	if Fingerprint(pos) == Fingerprint(neg) {
		t.Fatalf("M0 regression: negation collided onto same fingerprint: %s", Fingerprint(pos))
	}
	if !HasNegation(neg) || HasNegation(pos) {
		t.Fatalf("HasNegation mismatch: pos=%v neg=%v", HasNegation(pos), HasNegation(neg))
	}
	noImp := "No improvement observed at p=0.001"
	imp := "Improvement observed at p=0.001"
	if Fingerprint(noImp) == Fingerprint(imp) {
		t.Fatalf("M0 regression: 'no' negation collided onto same fingerprint")
	}
}
