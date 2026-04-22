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

	"github.com/RedNoodlesOrg/pzsm/internal/activity"
	"github.com/RedNoodlesOrg/pzsm/internal/config"
	"github.com/RedNoodlesOrg/pzsm/internal/middleware"
	"github.com/RedNoodlesOrg/pzsm/internal/mods"
	"github.com/RedNoodlesOrg/pzsm/internal/server"
	"github.com/RedNoodlesOrg/pzsm/internal/steam"
	"github.com/RedNoodlesOrg/pzsm/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	al := activity.New(db.DB(), log)

	steamClient := steam.New()
	modsSvc := mods.New(db.DB(), steamClient)

	app, err := server.New(modsSvc, al, log, cfg.SteamCollectionID, cfg.ServertestINI)
	if err != nil {
		return err
	}

	if middleware.DevBypassEnabled && cfg.DevUser != "" {
		log.Warn("auth bypass active: DEV_USER_EMAIL is set; unauthenticated requests will be attributed to this user",
			"user", cfg.DevUser,
		)
	}

	cfAccess := middleware.CFAccess(cfg.DevUser)
	authed := func(h http.Handler) http.Handler {
		return cfAccess(middleware.RequestLog(log)(h))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", authed(app.Routes()))

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		close(errCh)
	}()

	select {
	case <-shutdownCtx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
