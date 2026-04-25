package api

import "net/http"

func (a *API) handleListMods(w http.ResponseWriter, r *http.Request) {
	list, err := a.mods.List(r.Context())
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: list mods", "err", err)
		writeError(w, r, a.log, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, r, a.log, http.StatusOK, toModDTOs(list))
}

func (a *API) handleListOrdered(w http.ResponseWriter, r *http.Request) {
	list, err := a.mods.ListByPosition(r.Context())
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: list ordered", "err", err)
		writeError(w, r, a.log, http.StatusInternalServerError, "list failed")
		return
	}
	// Ordered listing is intended for the load-order page: enabled only.
	var enabled []modDTO
	for _, m := range list {
		for _, id := range m.ModIDs {
			if id.Enabled {
				enabled = append(enabled, toModDTO(m))
				break
			}
		}
	}
	if enabled == nil {
		enabled = []modDTO{}
	}
	writeJSON(w, r, a.log, http.StatusOK, enabled)
}
