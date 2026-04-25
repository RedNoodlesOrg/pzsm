package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/fakeapate/pzsm/internal/rcon"
)

type serverMsgRequest struct {
	Message string `json:"message"`
}

type reloadLuaRequest struct {
	Filename string `json:"filename"`
}

func (a *API) handleSave(w http.ResponseWriter, r *http.Request) {
	a.runRCON(w, r, "rcon.save", "", func() (string, error) {
		return a.rcon.Save(r.Context())
	})
}

func (a *API) handleQuit(w http.ResponseWriter, r *http.Request) {
	a.runRCON(w, r, "rcon.quit", "", func() (string, error) {
		return a.rcon.Quit(r.Context())
	})
}

func (a *API) handleServerMsg(w http.ResponseWriter, r *http.Request) {
	var req serverMsgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid JSON body")
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "message is required")
		return
	}
	a.runRCON(w, r, "rcon.servermsg", msg, func() (string, error) {
		return a.rcon.ServerMsg(r.Context(), msg)
	})
}

func (a *API) handleReloadOptions(w http.ResponseWriter, r *http.Request) {
	a.runRCON(w, r, "rcon.reloadoptions", "", func() (string, error) {
		return a.rcon.ReloadOptions(r.Context())
	})
}

func (a *API) handleReloadLua(w http.ResponseWriter, r *http.Request) {
	var req reloadLuaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Filename)
	if name == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "filename is required")
		return
	}
	a.runRCON(w, r, "rcon.reloadlua", name, func() (string, error) {
		return a.rcon.ReloadLua(r.Context(), name)
	})
}

// runRCON wraps the common pattern: invoke, classify error, audit, respond.
func (a *API) runRCON(w http.ResponseWriter, r *http.Request, action, target string, fn func() (string, error)) {
	resp, err := fn()
	if errors.Is(err, rcon.ErrNotConfigured) {
		writeError(w, r, a.log, http.StatusServiceUnavailable, "rcon is not configured")
		return
	}
	if errors.Is(err, rcon.ErrControlChar) {
		writeError(w, r, a.log, http.StatusBadRequest, "argument contains control characters")
		return
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: "+action, "err", err)
		writeError(w, r, a.log, http.StatusInternalServerError, "rcon: "+err.Error())
		return
	}
	a.activity.Record(r.Context(), action, target, map[string]any{
		"response": truncate(resp, rconResponseAuditLimit),
	})
	writeJSON(w, r, a.log, http.StatusOK, rconExecResponse{OK: true, Response: resp})
}
