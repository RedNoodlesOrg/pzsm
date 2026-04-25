// Package activity records user actions to both structured logs and persistent storage.
package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/RedNoodlesOrg/pzsm/internal/identity"
)

// Logger writes audit entries attributing user actions.
type Logger struct {
	db  *sql.DB
	log *slog.Logger
}

// New constructs a Logger that writes to the given database and structured logger.
func New(db *sql.DB, log *slog.Logger) *Logger {
	return &Logger{db: db, log: log}
}

// Record persists one activity entry, attributing it to the user carried in ctx.
// Background actions without an attached user are recorded as "system". Audit
// failures are logged at error level rather than returned, since callers cannot
// meaningfully react to a missing audit row after the user-facing action has
// already taken effect.
func (l *Logger) Record(ctx context.Context, action, target string, details map[string]any) {
	user := identity.User(ctx)
	if user == "" {
		user = "system"
	}
	var detailsJSON string
	if len(details) > 0 {
		b, err := json.Marshal(details)
		if err != nil {
			l.log.ErrorContext(ctx, "activity: marshal details", "action", action, "err", err)
		} else {
			detailsJSON = string(b)
		}
	}
	if _, err := l.db.ExecContext(ctx,
		`INSERT INTO activity_log (user_email, action, target, details, created_at) VALUES (?, ?, ?, ?, ?)`,
		user, action, target, detailsJSON, time.Now().Unix(),
	); err != nil {
		l.log.ErrorContext(ctx, "activity: insert",
			slog.String("user", user),
			slog.String("action", action),
			slog.String("target", target),
			slog.Any("err", err),
		)
		return
	}
	l.log.InfoContext(ctx, "activity",
		slog.String("user", user),
		slog.String("action", action),
		slog.String("target", target),
	)
}
