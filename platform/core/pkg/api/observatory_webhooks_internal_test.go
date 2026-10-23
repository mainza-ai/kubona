package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// P6: Ed25519 v1a webhook verification pins. Real keypairs, no network.

func testJWKS(t *testing.T, pubs ...ed25519.PublicKey) map[string]any {
	t.Helper()
	keys := make([]any, 0, len(pubs))
	for _, p := range pubs {
		keys = append(keys, map[string]any{
			"kty": "OKP",
			"crv": "Ed25519",
			"x":   base64.RawURLEncoding.EncodeToString(p),
		})
	}
	return map[string]any{"keys": keys}
}

func testSignedHeaders(t *testing.T, priv ed25519.PrivateKey, id string, ts int64, body []byte) http.Header {
	t.Helper()
	signed := append([]byte(fmt.Sprintf("%s.%d.", id, ts)), body...)
	sig := ed25519.Sign(priv, signed)
	h := http.Header{}
	h.Set("webhook-id", id)
	h.Set("webhook-timestamp", fmt.Sprintf("%d", ts))
	h.Set("webhook-signature", "v1a,"+base64.StdEncoding.EncodeToString(sig))
	return h
}

func TestVerifyAtlasWebhookSignatureValid(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	doc := testJWKS(t, other, pub) // rotation: valid key need not be first
	body := []byte(`{"id":"evt_1","type":"task.succeeded"}`)
	h := testSignedHeaders(t, priv, "del_1", time.Now().Unix(), body)
	if err := verifyAtlasWebhookSignature(doc, h, body); err != nil {
		t.Fatalf("valid delivery refused: %v", err)
	}
}

func TestVerifyAtlasWebhookSignatureTampered(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	doc := testJWKS(t, pub)
	h := testSignedHeaders(t, priv, "del_1", time.Now().Unix(), []byte(`{"id":"evt_1"}`))
	if err := verifyAtlasWebhookSignature(doc, h, []byte(`{"id":"evt_2"}`)); err == nil {
		t.Fatalf("tampered body accepted")
	}
}

func TestVerifyAtlasWebhookSignatureExpired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	doc := testJWKS(t, pub)
	body := []byte(`{"id":"evt_1"}`)
	h := testSignedHeaders(t, priv, "del_1", time.Now().Unix()-301, body)
	if err := verifyAtlasWebhookSignature(doc, h, body); err == nil {
		t.Fatalf("stale delivery accepted (replay risk)")
	}
}

func TestVerifyAtlasWebhookSignatureWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	doc := testJWKS(t, otherPub)
	body := []byte(`{"id":"evt_1"}`)
	h := testSignedHeaders(t, priv, "del_1", time.Now().Unix(), body)
	if err := verifyAtlasWebhookSignature(doc, h, body); err == nil {
		t.Fatalf("foreign key accepted")
	}
}

func TestVerifyAtlasWebhookSignatureMissingHeaders(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	doc := testJWKS(t, pub)
	if err := verifyAtlasWebhookSignature(doc, http.Header{}, []byte(`{}`)); err == nil {
		t.Fatalf("headerless delivery accepted")
	}
}
