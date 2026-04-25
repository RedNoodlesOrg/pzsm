// Package api serves the JSON HTTP surface under /api consumed by the SPA.
// It coexists with the server-rendered HTMX handlers in internal/server for
// as long as the legacy UI is still mounted.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/RedNoodlesOrg/pzsm/internal/activity"
	"github.com/RedNoodlesOrg/pzsm/internal/mods"
)

// API owns the JSON endpoints. Construct with New.
type API struct {
	mods          *mods.Service
	activity      *activity.Logger
	log           *slog.Logger
	collectionID  string
	servertestINI string
}

// New returns a configured API. servertestINI may be empty; apply-mods then
// surfaces a 400 with a clear message.
func New(modsSvc *mods.Service, act *activity.Logger, log *slog.Logger, collectionID, servertestINI string) *API {
	return &API{
		mods:          modsSvc,
		activity:      act,
		log:           log,
		collectionID:  collectionID,
		servertestINI: servertestINI,
	}
}

// Routes returns the /api/* handler. Mount it behind CFAccess + RequestLog
// middleware exactly like the HTML handler.
func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/mods", a.handleListMods)
	mux.HandleFunc("GET /api/mods/ordered", a.handleListOrdered)
	mux.HandleFunc("GET /api/mods/sync", a.handleSync)
	mux.HandleFunc("POST /api/mods/{ws}/ids/{mid}/toggle", a.handleToggle)
	mux.HandleFunc("POST /api/mods/apply", a.handleApply)
	mux.HandleFunc("POST /api/mods/reorder", a.handleReorder)
	mux.HandleFunc("POST /api/mods/{ws}/move", a.handleMove)
	mux.HandleFunc("GET /api/serverini", a.handleGetServerini)
	mux.HandleFunc("PUT /api/serverini", a.handlePutServerini)
	return mux
}

func writeJSON(w http.ResponseWriter, r *http.Request, log *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.ErrorContext(r.Context(), "api: encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, log *slog.Logger, status int, msg string) {
	writeJSON(w, r, log, status, map[string]string{"error": msg})
}
