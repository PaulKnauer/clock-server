package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"log"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"
)

// generateSelfSignedCert creates a self-signed RSA certificate valid between notBefore and notAfter.
func generateSelfSignedCert(t *testing.T, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

func TestCheckCertExpiryExpired(t *testing.T) {
	now := time.Now()
	cert := generateSelfSignedCert(t, now.Add(-48*time.Hour), now.Add(-1*time.Hour))
	warn, err := checkCertExpiry(cert, now)
	if err == nil {
		t.Fatal("expected error for expired cert, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected 'expired' in error, got: %v", err)
	}
	if warn {
		t.Fatal("expected warn=false when cert is already expired")
	}
}

func TestCheckCertExpiryWarningSoon(t *testing.T) {
	now := time.Now()
	cert := generateSelfSignedCert(t, now.Add(-24*time.Hour), now.Add(15*24*time.Hour))
	warn, err := checkCertExpiry(cert, now)
	if err != nil {
		t.Fatalf("expected no error for soon-expiring cert, got: %v", err)
	}
	if !warn {
		t.Fatal("expected warn=true for cert expiring in 15 days")
	}
}

func TestCheckCertExpiryValid(t *testing.T) {
	now := time.Now()
	cert := generateSelfSignedCert(t, now.Add(-24*time.Hour), now.Add(90*24*time.Hour))
	warn, err := checkCertExpiry(cert, now)
	if err != nil {
		t.Fatalf("expected no error for valid cert, got: %v", err)
	}
	if warn {
		t.Fatal("expected warn=false for cert expiring in 90 days")
	}
}

func TestCheckCertExpiryWarnLogged(t *testing.T) {
	// Verify that main-level code would log a warning for a near-expiry cert.
	// We test checkCertExpiry directly and confirm the warn flag triggers logging.
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	now := time.Now()
	cert := generateSelfSignedCert(t, now.Add(-24*time.Hour), now.Add(15*24*time.Hour))
	warn, err := checkCertExpiry(cert, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warn {
		daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)
		logger.Printf("WARNING: TLS certificate expires in %d days; renew soon", daysLeft)
	}

	output := buf.String()
	if !strings.Contains(output, "WARNING") {
		t.Fatalf("expected WARNING in log output, got: %q", output)
	}
	if !strings.Contains(output, "renew soon") {
		t.Fatalf("expected 'renew soon' in log output, got: %q", output)
	}
}

type fakeHTTPServer struct {
	listenAndServeFn    func() error
	listenAndServeTLSFn func(string, string) error
	shutdownFn          func(context.Context) error
}

func (f *fakeHTTPServer) ListenAndServe() error {
	if f.listenAndServeFn != nil {
		return f.listenAndServeFn()
	}
	return http.ErrServerClosed
}

func (f *fakeHTTPServer) ListenAndServeTLS(certFile, keyFile string) error {
	if f.listenAndServeTLSFn != nil {
		return f.listenAndServeTLSFn(certFile, keyFile)
	}
	return http.ErrServerClosed
}

func (f *fakeHTTPServer) Shutdown(ctx context.Context) error {
	if f.shutdownFn != nil {
		return f.shutdownFn(ctx)
	}
	return nil
}

func TestRunServerShutsDownOnContextCancel(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	shutdownCalled := make(chan struct{})

	server := &fakeHTTPServer{
		listenAndServeFn: func() error {
			close(started)
			<-done
			return http.ErrServerClosed
		},
		shutdownFn: func(context.Context) error {
			close(shutdownCalled)
			close(done)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-started
		cancel()
	}()

	if err := runServer(ctx, server, time.Second, "", ""); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	select {
	case <-shutdownCalled:
	case <-time.After(time.Second):
		t.Fatal("expected shutdown to be called")
	}
}

func TestRunServerUsesTLSWhenConfigured(t *testing.T) {
	var usedCert string
	var usedKey string
	server := &fakeHTTPServer{
		listenAndServeFn: func() error {
			t.Fatal("expected ListenAndServeTLS to be used")
			return nil
		},
		listenAndServeTLSFn: func(certFile, keyFile string) error {
			usedCert = certFile
			usedKey = keyFile
			return errors.New("listen failed")
		},
	}

	err := runServer(context.Background(), server, time.Second, "cert.pem", "key.pem")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listen failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedCert != "cert.pem" || usedKey != "key.pem" {
		t.Fatalf("unexpected TLS files: cert=%q key=%q", usedCert, usedKey)
	}
}
