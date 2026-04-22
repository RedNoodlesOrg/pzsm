// Package activity records user actions to both structured logs and persistent storage.
package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
// Background actions without an attached user are recorded as "system".
func (l *Logger) Record(ctx context.Context, action, target string, details map[string]any) error {
	user := identity.User(ctx)
	if user == "" {
		user = "system"
	}
	var detailsJSON string
	if len(details) > 0 {
		b, err := json.Marshal(details)
		if err != nil {
			return fmt.Errorf("activity: marshal details: %w", err)
		}
		detailsJSON = string(b)
	}
	_, err := l.db.ExecContext(ctx,
		`INSERT INTO activity_log (user_email, action, target, details, created_at) VALUES (?, ?, ?, ?, ?)`,
		user, action, target, detailsJSON, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("activity: insert %q: %w", action, err)
	}
	l.log.InfoContext(ctx, "activity",
		slog.String("user", user),
		slog.String("action", action),
		slog.String("target", target),
	)
	return nil
}
