// OpenPak news: one platform-neutral news service (PRD NB-03/NB-07) with
// console channels as renderers (NB-08). This binary runs the authoring
// gRPC, the read API and the Switch BCAT renderer standalone (dev, emulator
// delivery, tests). In production the Switch adapter mounts
// render/switchbcat on the three bcat hostnames (contract request NB-1).
package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	newsv1 "github.com/openpak/news/gen/news/v1"
	"github.com/openpak/news/internal/config"
	"github.com/openpak/news/internal/content"
	"github.com/openpak/news/internal/grpcapi"
	"github.com/openpak/news/internal/metrics"
	"github.com/openpak/news/internal/objects"
	"github.com/openpak/news/internal/store"
	"github.com/openpak/news/render/switchbcat"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}
	objs, err := objects.NewFS(cfg.ObjectsDir)
	if err != nil {
		return err
	}
	st := store.New(pool, objs)

	// Authoring gRPC (operator key required on every call).
	gs := grpc.NewServer(grpc.UnaryInterceptor(grpcapi.OperatorKeyInterceptor(cfg.OperatorKey)))
	newsv1.RegisterNewsServer(gs, grpcapi.New(st, st, objs, cfg, slog.Default()))
	healthv1.RegisterHealthServer(gs, health.NewServer())

	gLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	go gs.Serve(gLis)
	slog.Info("grpc listening", "addr", cfg.GRPCAddr)

	// Standalone HTTP: read API (NB-07) + Switch BCAT renderer (NB-08,
	// NB-01/02/05/06). The adapter can mount the same renderer mux instead.
	mux := http.NewServeMux()
	mux.Handle("/v1/articles", metrics.Middleware("readapi", &content.ReadAPI{Reader: st, PublicBaseURL: cfg.PublicBaseURL}))
	bcat := switchbcat.NewService(st, cfg.PublicBaseURL, slog.Default())
	mux.Handle("/", metrics.Middleware("switchbcat", bcat.Mux()))

	hLis, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	hs := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go hs.Serve(hLis)
	slog.Info("http listening", "addr", cfg.HTTPAddr)

	// metrics/pprof (+2, ports.md).
	mLis, err := net.Listen("tcp", cfg.MetricsAddr)
	if err != nil {
		return err
	}
	go (&http.Server{Handler: metrics.Handler(), ReadHeaderTimeout: 10 * time.Second}).Serve(mLis)
	slog.Info("metrics listening", "addr", cfg.MetricsAddr)

	<-ctx.Done()
	gs.GracefulStop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return hs.Shutdown(shutdownCtx)
}
