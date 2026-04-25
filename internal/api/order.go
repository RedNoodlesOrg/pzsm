package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fakeapate/pzsm/internal/mods"
)

type reorderRequest struct {
	Order []string `json:"order"`
}

type moveRequest struct {
	Position int `json:"position"`
}

func (a *API) handleReorder(w http.ResponseWriter, r *http.Request) {
	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := a.mods.Reorder(r.Context(), req.Order); err != nil {
		a.log.ErrorContext(r.Context(), "api: reorder", "err", err)
		writeError(w, r, a.log, http.StatusBadRequest, err.Error())
		return
	}
	a.activity.Record(r.Context(), "mods.reorder", "", map[string]any{"count": len(req.Order)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleResetOrder(w http.ResponseWriter, r *http.Request) {
	if a.collectionID == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "steam_collection_id is not configured")
		return
	}
	if err := a.mods.ResetOrderToCollection(r.Context(), a.collectionID); err != nil {
		a.log.ErrorContext(r.Context(), "api: reset-order", "err", err, "collection", a.collectionID)
		writeError(w, r, a.log, http.StatusInternalServerError, "reset-order failed: "+err.Error())
		return
	}
	a.activity.Record(r.Context(), "mods.reset-order", a.collectionID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMove(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Position < 1 {
		writeError(w, r, a.log, http.StatusBadRequest, "position must be >= 1")
		return
	}
	if err := a.mods.MoveTo(r.Context(), ws, req.Position); err != nil {
		if errors.Is(err, mods.ErrNotFound) {
			writeError(w, r, a.log, http.StatusNotFound, "mod is not enabled or does not exist")
			return
		}
		a.log.ErrorContext(r.Context(), "api: move", "err", err, "ws", ws, "pos", req.Position)
		writeError(w, r, a.log, http.StatusInternalServerError, "move failed")
		return
	}
	a.activity.Record(r.Context(), "mods.move", ws, map[string]any{"position": req.Position})
	w.WriteHeader(http.StatusNoContent)
}
