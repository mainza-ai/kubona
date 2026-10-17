package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/disk"
	"github.com/mainza-ai/kubona/core/pkg/grants"
	"github.com/mainza-ai/kubona/core/pkg/hf"
)

// K6: serve-posture validation. Non-local binds without TLS material are
// refused in strict mode; half material or missing files refuse always.
// W2.3: non-loopback plain HTTP refuses unless the operator explicitly opts
// in with KUBONA_ALLOW_PLAINTEXT=1 (fail-closed — warn-only served
// credentials across plaintext on one forgotten --bind).
func TestValidateServeConfig(t *testing.T) {
	// Loopback plaintext: the dev default, always allowed.
	if err := validateServeConfig("127.0.0.1", "", "", false); err != nil {
		t.Errorf("loopback plaintext must validate: %v", err)
	}
	if err := validateServeConfig("::1", "", "", false); err != nil {
		t.Errorf("ipv6 loopback must validate: %v", err)
	}
	// Half material refuses (serves nothing, must not boot).
	if err := validateServeConfig("127.0.0.1", "cert.pem", "", false); err == nil {
		t.Error("cert without key must refuse")
	}
	if err := validateServeConfig("127.0.0.1", "", "key.pem", false); err == nil {
		t.Error("key without cert must refuse")
	}
	// Strict mode without material refuses.
	if err := validateServeConfig("127.0.0.1", "", "", true); err == nil {
		t.Error("--tls-required without material must refuse")
	}
	// Missing files refuse with the path named.
	if err := validateServeConfig("0.0.0.0", "/nonexistent/c.pem", "/nonexistent/k.pem", false); err == nil {
		t.Error("missing material files must refuse")
	}
	// Real material validates.
	dir := t.TempDir()
	certFile, keyFile := writeSelfSigned(t, dir)
	if err := validateServeConfig("0.0.0.0", certFile, keyFile, true); err != nil {
		t.Errorf("real material in strict mode must validate: %v", err)
	}
	// W2.3: non-loopback plaintext without TLS refuses by default ...
	t.Setenv("KUBONA_ALLOW_PLAINTEXT", "")
	if err := validateServeConfig("0.0.0.0", "", "", false); err == nil {
		t.Error("non-loopback plain HTTP must refuse without KUBONA_ALLOW_PLAINTEXT=1")
	}
	// ... and passes with the explicit opt-in (warned at boot, not silent).
	t.Setenv("KUBONA_ALLOW_PLAINTEXT", "1")
	if err := validateServeConfig("0.0.0.0", "", "", false); err != nil {
		t.Errorf("explicit KUBONA_ALLOW_PLAINTEXT=1 must validate: %v", err)
	}
	// Opt-in values other than exactly "1" do not count.
	t.Setenv("KUBONA_ALLOW_PLAINTEXT", "true")
	if err := validateServeConfig("0.0.0.0", "", "", false); err == nil {
		t.Error("KUBONA_ALLOW_PLAINTEXT=true must not count (explicit \"1\" only)")
	}
}

func TestIsLoopback(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "::1", "localhost", "LOCALHOST", "[::1]", " 127.0.0.1 "} {
		if !isLoopback(h) {
			t.Errorf("%q must count as loopback", h)
		}
	}
	for _, h := range []string{"0.0.0.0", "", "192.168.1.5", "[::]", "example.com"} {
		if isLoopback(h) {
			t.Errorf("%q must not count as loopback", h)
		}
	}
}

// TestTLSServesWithMaterial proves end-to-end HTTPS serving with
// operator-owned material (stdlib only, no test dependency on prod creds).
func TestTLSServesWithMaterial(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeSelfSigned(t, dir)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("load material: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.0 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	}()
	addr := ln.Addr().String()
	if host, _, err := net.SplitHostPort(addr); err != nil || host == "" {
		t.Fatalf("bad listener addr %q: %v", addr, err)
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) // test-only: self-signed
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET / HTTP/1.0\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 64)
	n, _ := conn.Read(resp)
	if string(resp[:n]) == "" {
		t.Error("expected TLS response bytes")
	}
}

func writeSelfSigned(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "kubona-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, "t.pem")
	keyFile := filepath.Join(dir, "k.pem")
	certOut, _ := os.Create(certFile)
	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	certOut.Close()
	keyOut, _ := os.Create(keyFile)
	derKey, _ := x509.MarshalECPrivateKey(key)
	_ = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: derKey})
	keyOut.Close()
	return certFile, keyFile
}

// W0.20: operator CLI guards — key-file permissions, required scopes, and
// multi-envelope attenuation parents.
func TestGrantsCLIKeyFilePerms(t *testing.T) {
	dir := t.TempDir()
	// World-readable key file refuses.
	openPath := dir + "/open.hex"
	if err := os.WriteFile(openPath, []byte("abcd"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readKeyFile(openPath); err == nil {
		t.Fatalf("0644 key file must refuse")
	}
	// 0600 loads (content validation still applies).
	okPath := dir + "/ok.hex"
	if err := os.WriteFile(okPath, []byte("zz-not-hex"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readKeyFile(okPath); err == nil {
		t.Fatalf("non-hex content must refuse")
	}
}

func TestGrantsCLIFlagValidation(t *testing.T) {
	// Missing --scopes refuses at flag validation (usage error, exit 2).
	if code := grantsMint([]string{"--key-file", "k", "--aud", "a", "--granting", "g",
		"--target", "t", "--scope", "s", "--budget", "1", "--ttl", "1h"}); code != 2 {
		t.Fatalf("mint without --scopes must exit 2, got %d", code)
	}
	if code := grantsAttenuate([]string{"--key-file", "k", "--aud", "a", "--parent", "p",
		"--scope", "s", "--budget", "1", "--ttl", "1h"}); code != 2 {
		t.Fatalf("attenuate without --scopes must exit 2, got %d", code)
	}
}

// W0.20: attenuate accepts a multi-envelope parent (attenuates its leaf).
func TestGrantsCLIAttenuateChainParent(t *testing.T) {
	dir := t.TempDir()
	mkkey := func(name string) string {
		t.Helper()
		priv := make([]byte, 32)
		if _, err := rand.Read(priv); err != nil {
			t.Fatal(err)
		}
		hexKey := hex.EncodeToString(priv)
		if err := os.WriteFile(dir+"/"+name, []byte(hexKey), 0600); err != nil {
			t.Fatal(err)
		}
		return hexKey
	}
	privA, privB := mkkey("a.hex"), mkkey("b.hex")
	didB, err := grants.DIDForPrivkey(privB)
	if err != nil {
		t.Fatal(err)
	}
	rootSealed, _, err := grants.MintGrant(privA, didB, "lab-a", "lab-b", "qpu/submit",
		[]string{"qpu:submit", "read"}, 100.0, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	leafSealed, _, err := grants.AttenuateGrant(rootSealed, privB, didB,
		"qpu/submit", []string{"qpu:submit"}, 25.0, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	chainFile := dir + "/chain.b64"
	chainB64 := grants.EncodeChainB64([][]byte{rootSealed, leafSealed})
	if err := os.WriteFile(chainFile, []byte(chainB64[0]+"\n"+chainB64[1]+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Attenuating the 2-chain file (from its leaf) succeeds — previously
	// only single-envelope parents were accepted.
	if code := grantsAttenuate([]string{"--key-file", dir + "/b.hex", "--aud", didB,
		"--parent", chainFile, "--scope", "qpu/submit", "--scopes", "qpu:submit",
		"--budget", "10", "--ttl", "5m"}); code != 0 {
		t.Fatalf("attenuate from chain leaf must succeed, got %d", code)
	}
}

// D1: fresh state roots must become measurable. The disk guard statfs's
// hf.DefaultModelsDir(); on a fresh HOME that dir is missing, so /api/ready
// latched not-ready forever until the daemon ensures it at boot. This pins
// the contract: unmeasurable before ensure, measurable after.
func TestEnsureStateDirsMakesGuardPathMeasurable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := hf.DefaultModelsDir()
	if _, err := disk.MeasureAvail(dir); err == nil {
		t.Fatalf("fresh state root %q must NOT be measurable before ensure (else this test proves nothing)", dir)
	}
	if err := ensureStateDirs(); err != nil {
		t.Fatalf("ensure must succeed on a writable HOME: %v", err)
	}
	if _, err := disk.MeasureAvail(dir); err != nil {
		t.Fatalf("models dir must be measurable after ensure: %v", err)
	}
}
