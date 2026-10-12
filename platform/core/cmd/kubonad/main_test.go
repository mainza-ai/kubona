package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// K6: serve-posture validation. Non-local binds without TLS material are
// refused in strict mode; half material or missing files refuse always.
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
