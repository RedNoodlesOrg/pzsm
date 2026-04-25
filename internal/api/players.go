package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/fakeapate/pzsm/internal/rcon"
)

type playersResponse struct {
	Players []rcon.Player `json:"players"`
}

type kickRequest struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type whitelistAddRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	players, err := a.rcon.Players(r.Context())
	if errors.Is(err, rcon.ErrNotConfigured) {
		writeError(w, r, a.log, http.StatusServiceUnavailable, "rcon is not configured")
		return
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: players", "err", err)
		writeError(w, r, a.log, http.StatusInternalServerError, "rcon: "+err.Error())
		return
	}
	writeJSON(w, r, a.log, http.StatusOK, playersResponse{Players: players})
}

func (a *API) handleKick(w http.ResponseWriter, r *http.Request) {
	var req kickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "name is required")
		return
	}

	resp, err := a.rcon.Kick(r.Context(), name, strings.TrimSpace(req.Reason))
	if errors.Is(err, rcon.ErrNotConfigured) {
		writeError(w, r, a.log, http.StatusServiceUnavailable, "rcon is not configured")
		return
	}
	if errors.Is(err, rcon.ErrControlChar) {
		writeError(w, r, a.log, http.StatusBadRequest, "name or reason contains control characters")
		return
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: kick", "err", err, "name", name)
		writeError(w, r, a.log, http.StatusInternalServerError, "rcon: "+err.Error())
		return
	}
	a.activity.Record(r.Context(), "rcon.kick", name, map[string]any{
		"reason":   req.Reason,
		"response": truncate(resp, rconResponseAuditLimit),
	})
	writeJSON(w, r, a.log, http.StatusOK, rconExecResponse{OK: true, Response: resp})
}

func (a *API) handleWhitelistAdd(w http.ResponseWriter, r *http.Request) {
	var req whitelistAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user := strings.TrimSpace(req.Username)
	if user == "" || req.Password == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "username and password are required")
		return
	}

	resp, err := a.rcon.WhitelistAdd(r.Context(), user, req.Password)
	if errors.Is(err, rcon.ErrNotConfigured) {
		writeError(w, r, a.log, http.StatusServiceUnavailable, "rcon is not configured")
		return
	}
	if errors.Is(err, rcon.ErrControlChar) {
		writeError(w, r, a.log, http.StatusBadRequest, "username or password contains control characters")
		return
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: whitelist add", "err", err, "user", user)
		writeError(w, r, a.log, http.StatusInternalServerError, "rcon: "+err.Error())
		return
	}
	a.activity.Record(r.Context(), "rcon.whitelist.add", user, map[string]any{
		"response": truncate(resp, rconResponseAuditLimit),
	})
	writeJSON(w, r, a.log, http.StatusOK, rconExecResponse{OK: true, Response: resp})
}

func (a *API) handleWhitelistRemove(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSpace(r.PathValue("user"))
	if user == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "user is required")
		return
	}

	resp, err := a.rcon.WhitelistRemove(r.Context(), user)
	if errors.Is(err, rcon.ErrNotConfigured) {
		writeError(w, r, a.log, http.StatusServiceUnavailable, "rcon is not configured")
		return
	}
	if errors.Is(err, rcon.ErrControlChar) {
		writeError(w, r, a.log, http.StatusBadRequest, "user contains control characters")
		return
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: whitelist remove", "err", err, "user", user)
		writeError(w, r, a.log, http.StatusInternalServerError, "rcon: "+err.Error())
		return
	}
	a.activity.Record(r.Context(), "rcon.whitelist.remove", user, map[string]any{
		"response": truncate(resp, rconResponseAuditLimit),
	})
	writeJSON(w, r, a.log, http.StatusOK, rconExecResponse{OK: true, Response: resp})
}
