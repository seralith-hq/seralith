// Command sigil-control runs Sigil Control, the organization control plane.
//
// Copyright 2026 Sigil Systems, Inc. Licensed under the Sigil Commercial License; see ee/LICENSE.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ryan-tobin/new-startup/ee/control"
	"github.com/ryan-tobin/new-startup/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sigil-control:", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", envOr("SIGIL_CONTROL_ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", envOr("SIGIL_CONTROL_DB", "sigil-control.db"), "SQLite database path")
	tlsCert := flag.String("tls-cert", os.Getenv("SIGIL_CONTROL_TLS_CERT"), "TLS certificate file")
	tlsKey := flag.String("tls-key", os.Getenv("SIGIL_CONTROL_TLS_KEY"), "TLS private key file")
	jsonLogs := flag.Bool("json-logs", os.Getenv("SIGIL_CONTROL_JSON_LOGS") == "1", "emit JSON logs")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("sigil-control", version.String())
		return nil
	}
	var handler slog.Handler = slog.NewTextHandler(os.Stderr, nil)
	if *jsonLogs {
		handler = slog.NewJSONHandler(os.Stderr, nil)
	}
	logger := slog.New(handler)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := control.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	srv := &http.Server{
		Addr: *addr,
		Handler: control.NewServer(control.ServerConfig{
			Store: store, BootstrapToken: os.Getenv("SIGIL_CONTROL_BOOTSTRAP_TOKEN"), Logger: logger, Version: version.Version,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() {
		logger.Info("sigil-control listening", "addr", *addr, "version", version.Version, "tls", *tlsCert != "")
		if *tlsCert != "" {
			errc <- srv.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			errc <- srv.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
