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
