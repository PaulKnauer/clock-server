package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/paul/clock-server/internal/api"
	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/bootstrap"
	"github.com/paul/clock-server/internal/config"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	sender, checkers, cleanup, err := bootstrap.BuildCompositeSender(cfg)
	if err != nil {
		log.Fatalf("build senders: %v", err)
	}
	defer cleanup()

	dispatcher := application.NewCommandDispatcher(sender)
	handler := api.NewHandler(
		dispatcher,
		cfg.AuthCredentials,
		cfg.TrustProxyTLS,
		cfg.RequireTLS,
		cfg.ReadinessRequireAuth,
		cfg.MaxBodyBytes,
		cfg.AuthFailLimitPerMin,
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
