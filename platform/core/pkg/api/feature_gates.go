package api

import "os"

// Optional/external features are opt-in in production. Local dev keeps them
// on so tests and the zero-cost workflow are unchanged. Atlas/World Labs is
// explicitly optional: a production deployment that never sets a WLT key and
// never sets KUBONA_ATLAS_ENABLED=1 has no Atlas route at all.
func atlasLensEnabled() bool {
	if os.Getenv("KUBONA_ATLAS_ENABLED") == "1" {
		return true
	}
	return os.Getenv("KUBONA_ENV") != "production"
}

// A2A/x402 stays dormant (Q-23/Q-53). The self-signed local challenge is not
// a settlement rail, so production requires an explicit opt-in.
func a2aEnabled() bool {
	if os.Getenv("KUBONA_A2A_ENABLED") == "1" {
		return true
	}
	return os.Getenv("KUBONA_ENV") != "production"
}

// A production deployment must declare single-lab mode before anonymous
// session reads are allowed. Multi-lab (the default) requires auth for every
// session-scoped read (Q-57).
func singleLabDeclared() bool {
	if os.Getenv("KUBONA_ENV") != "production" {
		return true
	}
	return os.Getenv("KUBONA_SINGLE_LAB") == "1"
}
