// Package server wires the HTTP layer: template loading, static serving, and
// route registration against the mods service and activity logger.
package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/RedNoodlesOrg/pzsm/internal/activity"
	"github.com/RedNoodlesOrg/pzsm/internal/mods"
	"github.com/RedNoodlesOrg/pzsm/internal/web"
)

// Server owns the rendered HTTP layer. Construct with New.
type Server struct {
	tmpl          *template.Template
	mods          *mods.Service
	activity      *activity.Logger
	log           *slog.Logger
	collectionID  string
	servertestINI string
}

// New loads templates and returns a configured Server. servertestINI may be
// empty; the apply-mods handler then surfaces a user-visible error.
func New(modsSvc *mods.Service, act *activity.Logger, log *slog.Logger, collectionID, servertestINI string) (*Server, error) {
	tmpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	return &Server{
		tmpl:          tmpl,
		mods:          modsSvc,
		activity:      act,
		log:           log,
		collectionID:  collectionID,
		servertestINI: servertestINI,
	}, nil
}

// Routes returns an http.Handler that serves the authenticated portion of the app.
// The caller is expected to place this behind CFAccess and RequestLog middleware.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	staticFS, _ := fs.Sub(web.FS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /logs", s.handleLogs)
	mux.HandleFunc("POST /cmd/sync", s.handleSync)
	mux.HandleFunc("POST /cmd/applymods", s.handleApplyMods)
	mux.HandleFunc("POST /cmd/mod/{ws}/{mid}/toggle", s.handleToggle)

	return mux
}

func loadTemplates() (*template.Template, error) {
	funcs := template.FuncMap{
		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("dict: odd argument count")
			}
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key %d not a string", i)
				}
				m[key] = pairs[i+1]
			}
			return m, nil
		},
	}
	tmpl := template.New("").Funcs(funcs)
	sub, err := fs.Sub(web.FS, "templates")
	if err != nil {
		return nil, fmt.Errorf("server: sub templates: %w", err)
	}
	tmpl, err = tmpl.ParseFS(sub, "*.html", "partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("server: parse templates: %w", err)
	}
	return tmpl, nil
}
