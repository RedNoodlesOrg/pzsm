package server

import (
	"fmt"
	"net/http"

	"github.com/RedNoodlesOrg/pzsm/internal/identity"
	"github.com/RedNoodlesOrg/pzsm/internal/mods"
	"github.com/RedNoodlesOrg/pzsm/internal/serverini"
)

type indexData struct {
	User         string
	CollectionID string
	Mods         []mods.Mod
}

type toastData struct {
	Kind    string
	Message string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	list, err := s.mods.List(r.Context())
	if err != nil {
		s.log.ErrorContext(r.Context(), "list mods", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := indexData{
		User:         identity.User(r.Context()),
		CollectionID: s.collectionID,
		Mods:         list,
	}
	s.render(w, r, "base", data)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	// Placeholder: the real /logs page parses PZ server game logs (slice 5).
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintln(w, "logs page: wiring up in slice 5 (PZ game log stats).")
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if s.collectionID == "" {
		s.syncResponse(w, r, toastData{Kind: "error", Message: "STEAM_COLLECTION_ID is not set"})
		return
	}
	result, err := s.mods.Sync(r.Context(), s.collectionID)
	if err != nil {
		s.log.ErrorContext(r.Context(), "sync", "err", err)
		_ = s.activity.Record(r.Context(), "collection.sync", s.collectionID, map[string]any{"error": err.Error()})
		s.syncResponse(w, r, toastData{Kind: "error", Message: fmt.Sprintf("sync failed: %s", err)})
		return
	}
	_ = s.activity.Record(r.Context(), "collection.sync", s.collectionID, map[string]any{
		"fetched":      result.Fetched,
		"new_mods":     result.NewMods,
		"updated_mods": result.UpdatedMods,
		"new_mod_ids":  result.NewModIDs,
	})
	msg := fmt.Sprintf("synced %d mods (%d new, %d updated, %d new ids)",
		result.Fetched, result.NewMods, result.UpdatedMods, result.NewModIDs)
	s.syncResponse(w, r, toastData{Kind: "ok", Message: msg})
}

func (s *Server) syncResponse(w http.ResponseWriter, r *http.Request, toast toastData) {
	list, err := s.mods.List(r.Context())
	if err != nil {
		s.log.ErrorContext(r.Context(), "list after sync", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := indexData{
		User:         identity.User(r.Context()),
		CollectionID: s.collectionID,
		Mods:         list,
	}
	s.render(w, r, "toast", toast)
	s.render(w, r, "mod_grid", data)
}

func (s *Server) handleApplyMods(w http.ResponseWriter, r *http.Request) {
	if s.servertestINI == "" {
		s.render(w, r, "toast", toastData{Kind: "error", Message: "PZ_SERVERTEST_INI is not configured"})
		return
	}
	list, err := s.mods.List(r.Context())
	if err != nil {
		s.log.ErrorContext(r.Context(), "list mods for apply", "err", err)
		s.render(w, r, "toast", toastData{Kind: "error", Message: fmt.Sprintf("list failed: %s", err)})
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
	if err := serverini.UpdateMods(s.servertestINI, enabled, workshops); err != nil {
		s.log.ErrorContext(r.Context(), "apply mods", "err", err, "path", s.servertestINI)
		_ = s.activity.Record(r.Context(), "mods.apply", s.servertestINI, map[string]any{"error": err.Error()})
		s.render(w, r, "toast", toastData{Kind: "error", Message: fmt.Sprintf("apply failed: %s", err)})
		return
	}
	_ = s.activity.Record(r.Context(), "mods.apply", s.servertestINI, map[string]any{
		"enabled":   len(enabled),
		"workshops": len(workshops),
	})
	msg := fmt.Sprintf("wrote %d mod ids and %d workshop ids to servertest.ini", len(enabled), len(workshops))
	s.render(w, r, "toast", toastData{Kind: "ok", Message: msg})
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	mid := r.PathValue("mid")
	enabled, err := s.mods.Toggle(r.Context(), ws, mid)
	if err != nil {
		s.log.ErrorContext(r.Context(), "toggle", "err", err, "ws", ws, "mid", mid)
		http.Error(w, "toggle failed", http.StatusInternalServerError)
		return
	}
	_ = s.activity.Record(r.Context(), "mod.toggle", ws+"/"+mid, map[string]any{"enabled": enabled})
	s.render(w, r, "mod_toggle", map[string]any{
		"WorkshopID": ws,
		"ID":         mid,
		"Enabled":    enabled,
	})
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.ErrorContext(r.Context(), "render", "template", name, "err", err)
	}
}
