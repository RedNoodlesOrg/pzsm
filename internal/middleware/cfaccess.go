// Package middleware holds HTTP handler middleware shared across routes.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/fakeapate/pzsm/internal/identity"
)

const cfAccessEmailHeader = "Cf-Access-Authenticated-User-Email"

// CFAccess extracts the authenticated user email injected by Cloudflare Access
// and attaches it to the request context. Requests arriving without the header
// are rejected with 401 since the tunnel is the only trusted ingress.
//
// In builds compiled with -tags devbypass, a non-empty devUser is accepted as
// a fallback identity when the header is absent. The default build drops this
// branch entirely via dead-code elimination, so devUser has no effect in
// production binaries.
func CFAccess(devUser string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			email := r.Header.Get(cfAccessEmailHeader)
			if DevBypassEnabled && email == "" {
				email = devUser
			}
			if email == "" {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(identity.WithUser(r.Context(), email)))
		})
	}
}

// RequestLog emits a single structured log line per handled request.
func RequestLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sr, r)
			log.InfoContext(r.Context(), "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sr.status),
				slog.Duration("dur", time.Since(start)),
				slog.String("user", identity.User(r.Context())),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush forwards to the underlying writer so SSE handlers can stream
// through the middleware chain. Method promotion from the embedded
// http.ResponseWriter interface does not include Flush, so the assertion
// w.(http.Flusher) inside a wrapped handler fails without this.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
