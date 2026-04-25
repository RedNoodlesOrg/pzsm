package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/fakeapate/pzsm/internal/mods"
)

// handleSync runs a Steam collection sync and streams progress to the client
// as Server-Sent Events. Registered as GET because EventSource only issues
// GETs; the single-user internal-tool context makes the non-idempotent GET
// acceptable.
//
// Event frames:
//
//	event: progress   (mods.SyncEvent)
//	event: done       (mods.SyncResult)
//	event: error      ({"message": "..."})
func (a *API) handleSync(w http.ResponseWriter, r *http.Request) {
	if a.collectionID == "" {
		writeError(w, r, a.log, http.StatusBadRequest, "STEAM_COLLECTION_ID is not set")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, a.log, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := make(chan mods.SyncEvent, 16)
	var (
		result  mods.SyncResult
		syncErr error
	)
	go func() {
		defer close(ch)
		result, syncErr = a.mods.Sync(r.Context(), a.collectionID, ch)
	}()

	for ev := range ch {
		if err := writeSSE(w, "progress", ev); err != nil {
			a.log.ErrorContext(r.Context(), "api: sync sse write", "err", err)
			return
		}
		flusher.Flush()
	}

	// Channel close establishes happens-before for result/syncErr.
	if syncErr != nil {
		a.activity.Record(r.Context(), "collection.sync", a.collectionID, map[string]any{"error": syncErr.Error()})
		_ = writeSSE(w, "error", map[string]string{"message": syncErr.Error()})
		flusher.Flush()
		return
	}
	a.activity.Record(r.Context(), "collection.sync", a.collectionID, map[string]any{
		"fetched":      result.Fetched,
		"new_mods":     result.NewMods,
		"updated_mods": result.UpdatedMods,
		"new_mod_ids":  result.NewModIDs,
	})
	_ = writeSSE(w, "done", result)
	flusher.Flush()
}

func writeSSE(w io.Writer, event string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("api: sse marshal %s: %w", event, err)
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	return err
}
