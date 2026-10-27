package api

import "testing"

func TestOptionalFeaturesAreOptInInProduction(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_ATLAS_ENABLED", "")
	t.Setenv("KUBONA_A2A_ENABLED", "")
	t.Setenv("KUBONA_SINGLE_LAB", "")

	if atlasLensEnabled() {
		t.Fatal("Atlas must be disabled in production without an explicit opt-in")
	}
	if a2aEnabled() {
		t.Fatal("A2A must be disabled in production without an explicit opt-in")
	}
	if singleLabDeclared() {
		t.Fatal("single-lab mode must be declared explicitly in production")
	}

	t.Setenv("KUBONA_ATLAS_ENABLED", "1")
	t.Setenv("KUBONA_A2A_ENABLED", "1")
	t.Setenv("KUBONA_SINGLE_LAB", "1")
	if !atlasLensEnabled() || !a2aEnabled() || !singleLabDeclared() {
		t.Fatal("explicit opt-ins must enable the optional features")
	}
}

func TestOptionalFeaturesStayOnInDev(t *testing.T) {
	t.Setenv("KUBONA_ENV", "")
	t.Setenv("KUBONA_ATLAS_ENABLED", "")
	t.Setenv("KUBONA_A2A_ENABLED", "")
	if !atlasLensEnabled() || !a2aEnabled() || !singleLabDeclared() {
		t.Fatal("dev/test keeps the optional rails available")
	}
}
