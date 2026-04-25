package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/RedNoodlesOrg/pzsm/internal/rcon"
)

// rconExecBlocklist holds commands that the generic passthrough refuses.
// Destructive verbs are routed through dedicated typed handlers in later
// phases so the SPA can attach a confirm modal.
var rconExecBlocklist = map[string]struct{}{
	"quit":  {},
	"clear": {},
}

const rconResponseAuditLimit = 512

type rconExecRequest struct {
	Cmd string `json:"cmd"`
}

type rconExecResponse struct {
	OK       bool   `json:"ok"`
	Response string `json:"response"`
}

func (a *API) handleRCONExec(w http.ResponseWriter, r *http.Request) {
	var req rconExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cmd := strings.TrimSpace(req.Cmd)
	if cmd == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "cmd is required")
		return
	}

	verb := cmd
	if i := strings.IndexAny(cmd, " \t"); i >= 0 {
		verb = cmd[:i]
	}
	if _, blocked := rconExecBlocklist[verb]; blocked {
		writeError(w, r, a.log, http.StatusUnprocessableEntity, "command not allowed via passthrough; use the dedicated endpoint")
		return
	}

	resp, err := a.rcon.Exec(r.Context(), cmd)
	if errors.Is(err, rcon.ErrNotConfigured) {
		writeError(w, r, a.log, http.StatusServiceUnavailable, "rcon is not configured")
		return
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: rcon exec", "err", err, "verb", verb)
		writeError(w, r, a.log, http.StatusInternalServerError, "rcon exec failed: "+err.Error())
		return
	}

	a.activity.Record(r.Context(), "rcon.exec", verb, map[string]any{
		"cmd":      cmd,
		"response": truncate(resp, rconResponseAuditLimit),
	})
	writeJSON(w, r, a.log, http.StatusOK, rconExecResponse{OK: true, Response: resp})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
