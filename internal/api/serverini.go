package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/fakeapate/pzsm/internal/serverini"
)

// Field policy buckets. The set of keys in each bucket is intentionally narrow
// and is the only place we encode "this field is special" -- the SPA mirrors
// these classifications but the server is authoritative.
var (
	// hiddenKeys are managed by other parts of the app (mods sync / apply) and
	// must not appear in the GET response or be accepted by PUT.
	hiddenKeys = map[string]bool{
		"Mods":          true,
		"WorkshopItems": true,
		"Map":           true,
	}
	// readonlyKeys are surfaced for visibility but reject any update -- they
	// require infra coordination (ports) or risk save-game corruption (IDs).
	readonlyKeys = map[string]bool{
		"ResetID":        true,
		"ServerPlayerID": true,
		"DefaultPort":    true,
		"UDPPort":        true,
		"RCONPort":       true,
	}
	// secretKeys are returned with an empty string value to avoid leaking
	// credentials over the wire. PUT updates with an empty value are silently
	// dropped (treated as "no change") so the SPA can submit unchanged forms.
	secretKeys = map[string]bool{
		"RCONPassword": true,
		"Password":     true,
		"DiscordToken": true,
	}
)

const (
	policyEditable = "editable"
	policyReadonly = "readonly"
	policySecret   = "secret"
)

type serveriniEntryDTO struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
	Policy  string `json:"policy"`
}

type serveriniUpdateRequest struct {
	Updates map[string]string `json:"updates"`
}

type serveriniUpdateResponse struct {
	Updated []string `json:"updated"`
}

func policyFor(key string) string {
	switch {
	case secretKeys[key]:
		return policySecret
	case readonlyKeys[key]:
		return policyReadonly
	default:
		return policyEditable
	}
}

func (a *API) handleGetServerini(w http.ResponseWriter, r *http.Request) {
	if a.servertestINI == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "servertest_ini is not configured")
		return
	}
	entries, err := serverini.Read(a.servertestINI)
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: read serverini", "err", err, "path", a.servertestINI)
		writeError(w, r, a.log, http.StatusInternalServerError, "read failed")
		return
	}

	out := make([]serveriniEntryDTO, 0, len(entries))
	for _, e := range entries {
		if hiddenKeys[e.Key] {
			continue
		}
		dto := serveriniEntryDTO{
			Key:     e.Key,
			Value:   e.Value,
			Comment: e.Comment,
			Policy:  policyFor(e.Key),
		}
		if dto.Policy == policySecret {
			dto.Value = ""
		}
		out = append(out, dto)
	}
	writeJSON(w, r, a.log, http.StatusOK, out)
}

func (a *API) handlePutServerini(w http.ResponseWriter, r *http.Request) {
	if a.servertestINI == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "servertest_ini is not configured")
		return
	}

	var req serveriniUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, a.log, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Updates) == 0 {
		writeJSON(w, r, a.log, http.StatusOK, serveriniUpdateResponse{Updated: []string{}})
		return
	}

	entries, err := serverini.Read(a.servertestINI)
	if err != nil {
		a.log.ErrorContext(r.Context(), "api: read serverini", "err", err, "path", a.servertestINI)
		writeError(w, r, a.log, http.StatusInternalServerError, "read failed")
		return
	}
	known := make(map[string]bool, len(entries))
	for _, e := range entries {
		known[e.Key] = true
	}

	filtered := make(map[string]string, len(req.Updates))
	for key, val := range req.Updates {
		if hiddenKeys[key] {
			writeError(w, r, a.log, http.StatusBadRequest, "key is managed elsewhere: "+key)
			return
		}
		if readonlyKeys[key] {
			writeError(w, r, a.log, http.StatusBadRequest, "key is read-only: "+key)
			return
		}
		if !known[key] {
			writeError(w, r, a.log, http.StatusBadRequest, "unknown key: "+key)
			return
		}
		// Empty string for a secret = "no change".
		if secretKeys[key] && val == "" {
			continue
		}
		filtered[key] = val
	}

	if len(filtered) == 0 {
		writeJSON(w, r, a.log, http.StatusOK, serveriniUpdateResponse{Updated: []string{}})
		return
	}

	if err := serverini.WriteFields(a.servertestINI, filtered); err != nil {
		a.log.ErrorContext(r.Context(), "api: write serverini", "err", err, "path", a.servertestINI)
		a.activity.Record(r.Context(), "serverini.update", a.servertestINI, map[string]any{"error": err.Error()})
		writeError(w, r, a.log, http.StatusInternalServerError, err.Error())
		return
	}

	updated := make([]string, 0, len(filtered))
	for k := range filtered {
		updated = append(updated, k)
	}
	sort.Strings(updated)
	a.activity.Record(r.Context(), "serverini.update", a.servertestINI, map[string]any{"keys": updated})
	writeJSON(w, r, a.log, http.StatusOK, serveriniUpdateResponse{Updated: updated})
}
