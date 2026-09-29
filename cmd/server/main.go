package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShotaKitazawa/glidepath/internal/auth"
	"github.com/ShotaKitazawa/glidepath/internal/config"
	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
	"github.com/ShotaKitazawa/glidepath/internal/handler"
)

func main() {
	disableOIDC := flag.Bool("disable-oidc", false, "disable OIDC authentication (local development only)")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	if err := run(*addr, *disableOIDC); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(addr string, disableOIDC bool) error {
	cfg, err := config.Load(addr, disableOIDC)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authMiddleware, err := auth.New(ctx, cfg)
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	queries := sqlcgen.New(pool)

	mux := http.NewServeMux()
	handler.Register(mux, queries)
	authMiddleware.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: authMiddleware.Wrap(mux),
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	slog.Info("starting server", "addr", addr, "disable_oidc", disableOIDC)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
