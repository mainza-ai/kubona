package api

import (
	"os"
	"testing"
)

func TestValidateDownloadURL(t *testing.T) {
	t.Setenv("KUBONA_ENV", "development")
	for _, raw := range []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"https://user:pass@example.com/x",
		"not a url",
	} {
		if err := validateDownloadURL(raw); err == nil {
			t.Errorf("unsafe URL %q accepted", raw)
		}
	}
	if err := validateDownloadURL("http://127.0.0.1:9/x"); err != nil {
		t.Fatalf("dev loopback fixture must be allowed: %v", err)
	}

	t.Setenv("KUBONA_ENV", "production")
	if err := validateDownloadURL("http://127.0.0.1:9/x"); err == nil {
		t.Fatal("production must refuse http")
	}
	if err := validateDownloadURL("https://127.0.0.1/x"); err == nil {
		t.Fatal("production must refuse loopback resolution")
	}
	os.Unsetenv("KUBONA_ENV")
}
