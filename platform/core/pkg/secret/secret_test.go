package secret

// B10/S9 pins: one shared secret screen recognises real credential shapes,
// redacts them visibly, and leaves benign text untouched.

import (
	"strings"
	"testing"
)

func TestDetectAndRedactCredentialShapes(t *testing.T) {
	cases := []struct {
		name string
		text string
		kind string
	}{
		{"pem", "-----BEGIN RSA " + "PRIV" + "ATE KEY-----\nMIIE...\n-----END RSA " + "PRIV" + "ATE KEY-----", "private_key"},
		{"openai", "use sk-abcdefghijklmnopqrstuvwxyz123456 for this", "openai_key"},
		{"github", "token ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789", "github_token"},
		{"aws", "AKIA" + "ABCDEFGHIJKLMNOP", "aws_access_key"},
		{"slack", "xoxb-" + "1234567890-abcdefghij", "slack_token"},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnopqrstuv", "jwt"},
		{"bearer", "Authorization: Bearer abcdefghijklmnopqrstuvwxyz", "bearer_token"},
		{"assignment", "api_key = super-secret-value-123", "credential_assignment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, ok := Detect(tc.text)
			if !ok || kind != tc.kind {
				t.Fatalf("detect: got %q,%v want %q", kind, ok, tc.kind)
			}
			clean, kinds := Redact(tc.text)
			if len(kinds) == 0 {
				t.Fatal("redact must report kinds")
			}
			if clean == tc.text {
				t.Fatal("redact must change the text")
			}
			if strings.Contains(clean, "super-secret-value-123") || strings.Contains(clean, "sk-abcdefghijklmnopqrstuvwxyz") {
				t.Fatalf("secret survived redaction: %q", clean)
			}
			if !strings.Contains(clean, "[redacted:") {
				t.Fatalf("redaction marker missing: %q", clean)
			}
		})
	}
}

func TestBenignTextIsUntouched(t *testing.T) {
	text := "The H2 bond distance is 0.7414 Angstroms; the API returns JSON."
	clean, kinds := Redact(text)
	if clean != text || len(kinds) != 0 || ContainsSecret(text) {
		t.Fatalf("benign text must be untouched: %q %v", clean, kinds)
	}
}

func TestMultipleKindsAreSortedAndDeduped(t *testing.T) {
	_, kinds := Redact("sk-" + "abcdefghijklmnopqrstuvwx and AKIA" + "ABCDEFGHIJKLMNOP and sk-" + "zyxwvutsrqponmlkjihgfedc")
	if len(kinds) != 2 || kinds[0] != "aws_access_key" || kinds[1] != "openai_key" {
		t.Fatalf("kinds must be sorted and deduped: %v", kinds)
	}
	if note := RedactionNote(kinds); !strings.Contains(note, "aws_access_key") || !strings.Contains(note, "openai_key") {
		t.Fatalf("note: %q", note)
	}
}
