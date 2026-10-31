package api

import (
	"net/url"
	"testing"
)

func TestAllowedAtlasImageURL(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://worldlabs.ai/a.png", true},
		{"https://cdn.worldlabs.ai/a.png", true},
		{"https://storage.googleapis.com/wlt-marble-outputs-prod/us1/a.png?X-Goog-Signature=x", true},
		{"https://storage.googleapis.com/other-bucket/a.png", false},
		{"http://storage.googleapis.com/wlt-marble-outputs-prod/a.png", false},
		{"https://evil.example/wlt-marble-outputs-prod/a.png", false},
		{"https://storage.googleapis.com.evil.example/wlt-marble-outputs-prod/a.png", false},
	}
	for _, tc := range tests {
		u, err := url.Parse(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := allowedAtlasImageURL(u); got != tc.want {
			t.Errorf("allowedAtlasImageURL(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
