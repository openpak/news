// SPDX-License-Identifier: AGPL-3.0-only

// bcatserve hosts operator-supplied datasets through OpenPak's existing renderer.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openpak/news/internal/bcatcontainer"
	"github.com/openpak/news/internal/datasetfiles"
	"github.com/openpak/news/render/nativebcat"
	"github.com/openpak/news/render/switchbcat"
)

func main() {
	root := flag.String("root", "", "directory containing per-title manifests and files (required)")
	listen := flag.String("listen", ":20044", "internal HTTP listener")
	check := flag.Bool("check", false, "validate datasets and exit without listening")
	signing := flag.String("signing-key", "", "private PEM for native BCAT containers; unset disables native delivery")
	verify := flag.String("verify-key", "", "public PEM installed by the console verification-key patch (required with -signing-key)")
	dataOrigin := flag.String("native-data-origin", "https://bcat-data-lp1.cdn.nintendo.net", "native HTTPS file origin")
	flag.Parse()
	if *root == "" {
		slog.Error("-root is required")
		os.Exit(1)
	}
	source, err := datasetfiles.Load(*root)
	if err != nil {
		slog.Error("datasets rejected", "error", err)
		os.Exit(1)
	}
	titles, _, _ := source.Titles(context.Background())
	mux := http.NewServeMux()
	if *signing != "" {
		publicPEM, err := os.ReadFile(*verify)
		if err != nil {
			slog.Error("native verification public key unavailable; -verify-key is required")
			os.Exit(1)
		}
		publicKey, err := bcatcontainer.ParsePublicKeyPEM(publicPEM)
		if err != nil {
			slog.Error("native verification public key rejected", "error", err)
			os.Exit(1)
		}
		encoded, err := os.ReadFile(*signing)
		if err != nil {
			slog.Error("native signing key unavailable")
			os.Exit(1)
		}
		key, err := bcatcontainer.ParsePrivateKeyPEM(encoded)
		if err != nil {
			slog.Error("native signing key rejected", "error", err)
			os.Exit(1)
		}
		if !key.PublicKey.Equal(publicKey) {
			slog.Error("native signing key does not match the console verification public key")
			os.Exit(1)
		}
		native, err := nativebcat.NewService(source, key, *dataOrigin)
		if err == nil {
			err = native.Validate(context.Background(), titles)
		}
		if err != nil {
			slog.Error("native datasets rejected", "error", err)
			os.Exit(1)
		}
		native.RegisterRoutes(mux)
	}
	registerInternal(mux, source)
	mux.Handle("/", switchbcat.NewService(source, "", nil).Mux())
	slog.Info("datasets validated", "current_titles", titles)
	if *check {
		return
	}
	server := &http.Server{Addr: *listen, Handler: mux,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	slog.Info("BCAT delivery listening", "address", *listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP listener", "error", err)
		os.Exit(1)
	}
}
