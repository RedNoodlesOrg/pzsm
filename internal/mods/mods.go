// Package mods is the service layer over the mods/mod_ids tables: Steam sync,
// listing for the UI, and per-mod-id toggle persistence.
package mods

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/RedNoodlesOrg/pzsm/internal/steam"
)

// ErrNotFound is returned by SetEnabled/Toggle when the mod id doesn't exist.
var ErrNotFound = errors.New("mods: not found")

// Service owns mod persistence and Steam synchronisation.
type Service struct {
	db    *sql.DB
	steam *steam.Client
}

// New constructs a Service.
func New(db *sql.DB, client *steam.Client) *Service {
	return &Service{db: db, steam: client}
}

// ModID is one in-game mod identifier together with its enabled flag.
type ModID struct {
	ID      string `json:"id,omitempty"`
	Enabled bool   `json:"enabled,omitempty"`
}

// Mod is one workshop mod plus the ids extracted from its description.
type Mod struct {
	WorkshopID  string    `json:"workshop_id,omitempty"`
	Name        string    `json:"name,omitempty"`
	Thumbnail   string    `json:"thumbnail,omitempty"`
	Description string    `json:"description,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	ModIDs      []ModID   `json:"mod_i_ds,omitempty"`
}

// SyncResult summarises what a Sync call changed.
type SyncResult struct {
	Fetched     int
	NewMods     int
	UpdatedMods int
	NewModIDs   int
}

// Sync fetches the given collection from Steam and upserts its mods into the
// database. Existing per-id enabled flags are preserved. Mods removed from the
// Steam collection are kept in the DB so their toggle state survives until
// explicitly deleted.
func (s *Service) Sync(ctx context.Context, collectionID string) (SyncResult, error) {
	if collectionID == "" {
		return SyncResult{}, errors.New("mods: collection id is empty")
	}
	ids, err := s.steam.ExpandCollection(ctx, collectionID)
	if err != nil {
		return SyncResult{}, err
	}
	details, err := s.steam.GetPublishedFileDetails(ctx, ids)
	if err != nil {
		return SyncResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("mods: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var result SyncResult
	now := time.Now().Unix()
	for _, d := range details {
		if d.Result != 1 {
			continue
		}
		result.Fetched++

		var existed int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM mods WHERE workshop_id = ?`, d.PublishedFileID,
		).Scan(&existed); err != nil {
			return SyncResult{}, fmt.Errorf("mods: check %s: %w", d.PublishedFileID, err)
		}

		if existed == 0 {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mods (workshop_id, name, thumbnail, description, updated_at)
				 VALUES (?, ?, ?, ?, ?)`,
				d.PublishedFileID, d.Title, d.PreviewURL, d.Description, now,
			); err != nil {
				return SyncResult{}, fmt.Errorf("mods: insert %s: %w", d.PublishedFileID, err)
			}
			result.NewMods++
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE mods SET name = ?, thumbnail = ?, description = ?, updated_at = ?
				 WHERE workshop_id = ?`,
				d.Title, d.PreviewURL, d.Description, now, d.PublishedFileID,
			); err != nil {
				return SyncResult{}, fmt.Errorf("mods: update %s: %w", d.PublishedFileID, err)
			}
			result.UpdatedMods++
		}

		for _, mid := range steam.ExtractModIDs(d.Description) {
			var idExists int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM mod_ids WHERE workshop_id = ? AND mod_id = ?`,
				d.PublishedFileID, mid.ID,
			).Scan(&idExists); err != nil {
				return SyncResult{}, fmt.Errorf("mods: check id %s/%s: %w", d.PublishedFileID, mid.ID, err)
			}
			if idExists != 0 {
				continue
			}
			enabled := 0
			if mid.Enabled {
				enabled = 1
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mod_ids (workshop_id, mod_id, enabled) VALUES (?, ?, ?)`,
				d.PublishedFileID, mid.ID, enabled,
			); err != nil {
				return SyncResult{}, fmt.Errorf("mods: insert id %s/%s: %w", d.PublishedFileID, mid.ID, err)
			}
			result.NewModIDs++
		}
	}

	if err := tx.Commit(); err != nil {
		return SyncResult{}, fmt.Errorf("mods: commit: %w", err)
	}
	return result, nil
}

// List returns every mod in the DB along with its mod ids, ordered for display.
func (s *Service) List(ctx context.Context) ([]Mod, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workshop_id, name, thumbnail, description, updated_at
		 FROM mods
		 ORDER BY name COLLATE NOCASE`,
	)
	if err != nil {
		return nil, fmt.Errorf("mods: list: %w", err)
	}
	defer rows.Close()

	var out []Mod
	for rows.Next() {
		var m Mod
		var ts int64
		if err := rows.Scan(&m.WorkshopID, &m.Name, &m.Thumbnail, &m.Description, &ts); err != nil {
			return nil, fmt.Errorf("mods: scan: %w", err)
		}
		m.UpdatedAt = time.Unix(ts, 0)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mods: rows: %w", err)
	}

	for i := range out {
		ids, err := s.listModIDs(ctx, out[i].WorkshopID)
		if err != nil {
			return nil, err
		}
		out[i].ModIDs = ids
	}
	return out, nil
}

func (s *Service) listModIDs(ctx context.Context, workshopID string) ([]ModID, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT mod_id, enabled FROM mod_ids
		 WHERE workshop_id = ?
		 ORDER BY length(mod_id), mod_id`,
		workshopID,
	)
	if err != nil {
		return nil, fmt.Errorf("mods: list ids %s: %w", workshopID, err)
	}
	defer rows.Close()

	var out []ModID
	for rows.Next() {
		var mid ModID
		var enabled int
		if err := rows.Scan(&mid.ID, &enabled); err != nil {
			return nil, fmt.Errorf("mods: scan id: %w", err)
		}
		mid.Enabled = enabled != 0
		out = append(out, mid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mods: id rows: %w", err)
	}
	return out, nil
}

// Toggle flips the enabled flag for one mod id atomically and returns the new state.
func (s *Service) Toggle(ctx context.Context, workshopID, modID string) (bool, error) {
	var enabled int
	err := s.db.QueryRowContext(ctx,
		`UPDATE mod_ids SET enabled = 1 - enabled
		 WHERE workshop_id = ? AND mod_id = ?
		 RETURNING enabled`,
		workshopID, modID,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("%s/%s: %w", workshopID, modID, ErrNotFound)
	}
	if err != nil {
		return false, fmt.Errorf("mods: toggle %s/%s: %w", workshopID, modID, err)
	}
	return enabled != 0, nil
}
