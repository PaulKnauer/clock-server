package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/paul/clock-server/internal/api"
	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/bootstrap"
	"github.com/paul/clock-server/internal/config"
	"github.com/paul/clock-server/internal/metrics"
	"github.com/paul/clock-server/internal/security"
)

// certExpiryWarningThreshold is the window before expiry at which a warning is emitted.
const certExpiryWarningThreshold = 30 * 24 * time.Hour

// checkCertExpiry checks whether the certificate is expired or close to expiry.
// It returns warn=true when the cert expires within the warning threshold, and a
// non-nil error when the cert is already expired. now is injected for testability.
func checkCertExpiry(cert *x509.Certificate, now time.Time) (warn bool, err error) {
	if now.After(cert.NotAfter) {
		return false, fmt.Errorf("TLS certificate expired on %s", cert.NotAfter.Format(time.RFC3339))
	}
	if cert.NotAfter.Sub(now) < certExpiryWarningThreshold {
		return true, nil
	}
	return false, nil
}

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		pair, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			log.Fatalf("load TLS key pair: %v", err)
		}
		cert, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			log.Fatalf("parse TLS certificate: %v", err)
		}
		warn, err := checkCertExpiry(cert, time.Now())
		if err != nil {
			log.Fatalf("TLS certificate check failed: %v", err)
		}
		if warn {
			daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)
			log.Printf("WARNING: TLS certificate expires in %d days (%s); renew soon", daysLeft, cert.NotAfter.Format(time.RFC3339))
		}
	}

	sender, checkers, cleanup, err := bootstrap.BuildCompositeSender(cfg)
	if err != nil {
		log.Fatalf("build senders: %v", err)
	}
	defer cleanup()

	dispatcher := application.NewCommandDispatcher(sender)
	m := metrics.NewMetrics()
	handler := api.NewHandlerWithOptions(
		dispatcher,
		cfg.AuthCredentials,
		cfg.TrustProxyTLS,
		cfg.RequireTLS,
		cfg.ReadinessRequireAuth,
		cfg.MaxBodyBytes,
		cfg.AuthFailLimitPerMin,
		api.HandlerOptions{
			Metrics:            m,
			MetricsRequireAuth: cfg.MetricsRequireAuth,
		},
		checkers...,
	)

	server := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           handler.Routes(),
		ReadTimeout:       cfg.ServerReadTimeout,
		ReadHeaderTimeout: cfg.ServerHeaderTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Handle SIGHUP for zero-downtime credential rotation.
	// On SIGHUP: re-read API_AUTH_CREDENTIALS, parse, and atomically swap the store.
	// Parse errors are logged and the existing credentials are kept (fail-safe).
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-sighupCh:
				raw := os.Getenv("API_AUTH_CREDENTIALS")
				newCreds, parseErr := security.ParseCredentials(raw)
				if parseErr != nil {
					log.Printf("credential reload failed (keeping existing credentials): %v", parseErr)
					continue
				}
				if len(newCreds) == 0 {
					log.Printf("credential reload skipped: parsed zero credentials (keeping existing credentials)")
					continue
				}
				cfg.AuthCredentials.Replace(newCreds)
				ids := make([]string, len(newCreds))
				for i, c := range newCreds {
					ids[i] = c.ID
				}
				log.Printf("credentials reloaded, count=%d ids=%v", len(newCreds), ids)
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Printf("clock command dispatcher listening on %s", cfg.ServerAddr)
	if err := runServer(ctx, server, cfg.ServerShutdownPeriod, cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

type httpServer interface {
	ListenAndServe() error
	ListenAndServeTLS(certFile, keyFile string) error
	Shutdown(ctx context.Context) error
}

func runServer(ctx context.Context, server httpServer, shutdownPeriod time.Duration, tlsCertFile, tlsKeyFile string) error {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownPeriod)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	var serveErr error
	if tlsCertFile != "" && tlsKeyFile != "" {
		serveErr = server.ListenAndServeTLS(tlsCertFile, tlsKeyFile)
	} else {
		serveErr = server.ListenAndServe()
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}
