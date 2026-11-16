package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Whitedukecmr/openstack-go-api/internal/api"
	"github.com/Whitedukecmr/openstack-go-api/internal/cloud"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := getenv("LISTEN_ADDR", ":8080")
	token := os.Getenv("API_TOKEN")
	if token == "" {
		log.Warn("API_TOKEN is empty: /api/* is NOT protected")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	oc, err := cloud.NewOpenStack(authCtx)
	cancel()
	if err != nil {
		log.Error("openstack init failed", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(oc, token, log),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      120 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("stopped")
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
