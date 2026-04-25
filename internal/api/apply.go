package api

import (
	"net/http"

	"github.com/RedNoodlesOrg/pzsm/internal/serverini"
)

type applyResponse struct {
	EnabledCount  int `json:"enabled_count"`
	WorkshopCount int `json:"workshop_count"`
}

func (a *API) handleApply(w http.ResponseWriter, r *http.Request) {
	if a.servertestINI == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "PZ_SERVERTEST_INI is not configured")
		return
	}
	list, err := a.mods.ListByPosition(r.Context())
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: list for apply", "err", err)
		writeError(w, r, a.log, http.StatusInternalServerError, "list failed")
		return
	}
	var enabled, workshops []string
	for _, m := range list {
		workshops = append(workshops, m.WorkshopID)
		for _, id := range m.ModIDs {
			if id.Enabled {
				enabled = append(enabled, id.ID)
			}
		}
	}
	if err := serverini.UpdateMods(a.servertestINI, enabled, workshops); err != nil {
		a.log.ErrorContext(r.Context(), "api: apply", "err", err, "path", a.servertestINI)
		a.activity.Record(r.Context(), "mods.apply", a.servertestINI, map[string]any{"error": err.Error()})
		writeError(w, r, a.log, http.StatusInternalServerError, err.Error())
		return
	}
	a.activity.Record(r.Context(), "mods.apply", a.servertestINI, map[string]any{
		"enabled":   len(enabled),
		"workshops": len(workshops),
	})
	writeJSON(w, r, a.log, http.StatusOK, applyResponse{
		EnabledCount:  len(enabled),
		WorkshopCount: len(workshops),
	})
}
