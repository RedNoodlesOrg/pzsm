package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fakeapate/pzsm/internal/activity"
	"github.com/fakeapate/pzsm/internal/api"
	"github.com/fakeapate/pzsm/internal/config"
	"github.com/fakeapate/pzsm/internal/middleware"
	"github.com/fakeapate/pzsm/internal/mods"
	"github.com/fakeapate/pzsm/internal/rcon"
	"github.com/fakeapate/pzsm/internal/steam"
	"github.com/fakeapate/pzsm/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	if err := run(log, *configPath); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	al := activity.New(db.DB(), log)

	steamClient := steam.New(steam.WithAPIKey(cfg.SteamWebAPIKey))
	modsSvc := mods.New(db.DB(), steamClient)
	rconSvc := rcon.New(cfg.RCONHost, cfg.RCONPort, cfg.RCONPassword, log)

	jsonAPI := api.New(modsSvc, rconSvc, al, log, cfg.SteamCollectionID, cfg.ServertestINI)

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
	mux.Handle("/api/", authed(jsonAPI.Routes()))

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
