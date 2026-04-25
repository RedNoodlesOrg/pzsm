package api

import (
	"errors"
	"net/http"

	"github.com/RedNoodlesOrg/pzsm/internal/mods"
)

type toggleResponse struct {
	Enabled bool `json:"enabled"`
}

func (a *API) handleToggle(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	mid := r.PathValue("mid")
	enabled, err := a.mods.Toggle(r.Context(), ws, mid)
	if errors.Is(err, mods.ErrNotFound) {
		writeError(w, r, a.log, http.StatusNotFound, "mod id not found")
		return
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: toggle", "err", err, "ws", ws, "mid", mid)
		writeError(w, r, a.log, http.StatusInternalServerError, "toggle failed")
		return
	}
	a.activity.Record(r.Context(), "mod.toggle", ws+"/"+mid, map[string]any{"enabled": enabled})
	writeJSON(w, r, a.log, http.StatusOK, toggleResponse{Enabled: enabled})
}
