package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

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
