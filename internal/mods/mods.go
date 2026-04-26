// Package mods is the service layer over the mods/mod_ids tables: Steam sync,
// listing for the UI, and per-mod-id toggle persistence.
package mods

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fakeapate/pzsm/internal/steam"
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
	WorkshopID string    `json:"workshop_id,omitempty"`
	Name       string    `json:"name,omitempty"`
	Thumbnail  string    `json:"thumbnail,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	ModIDs     []ModID   `json:"mod_ids,omitempty"`
}

// SyncResult summarises what a Sync call changed.
type SyncResult struct {
	Fetched     int `json:"fetched"`
	NewMods     int `json:"new_mods"`
	UpdatedMods int `json:"updated_mods"`
	NewModIDs   int `json:"new_mod_ids"`
}

// SyncStage identifies which phase of Sync a SyncEvent came from.
type SyncStage string

const (
	// SyncStageExpanding: resolving the collection into its workshop ids.
	SyncStageExpanding SyncStage = "expanding"
	// SyncStageFetching: fetching the batch of published-file details.
	SyncStageFetching SyncStage = "fetching"
	// SyncStageUpserting: walking the result and writing rows; Current/Total
	// are meaningful here.
	SyncStageUpserting SyncStage = "upserting"
)

// SyncEvent is one progress frame emitted on the optional channel passed to Sync.
type SyncEvent struct {
	Stage   SyncStage `json:"stage"`
	Current int       `json:"current"`
	Total   int       `json:"total"`
	Name    string    `json:"name,omitempty"`
}

// Sync fetches the given collection from Steam and upserts its mods into the
// database. Existing per-id enabled flags are preserved. Mods removed from the
// Steam collection are kept in the DB so their toggle state survives until
// explicitly deleted. If progress is non-nil, Sync sends SyncEvent frames on it
// as it works; the caller owns the channel and is responsible for draining it.
// Sync does not close the channel.
func (s *Service) Sync(ctx context.Context, collectionID string, progress chan<- SyncEvent) (SyncResult, error) {
	if collectionID == "" {
		return SyncResult{}, errors.New("mods: collection id is empty")
	}
	emit(progress, SyncEvent{Stage: SyncStageExpanding})
	ids, err := s.steam.ExpandCollection(ctx, collectionID)
	if err != nil {
		return SyncResult{}, err
	}
	emit(progress, SyncEvent{Stage: SyncStageFetching, Total: len(ids)})
	details, err := s.steam.GetPublishedFileDetails(ctx, ids)
	if err != nil {
		return SyncResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("mods: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextPosition int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM mods`).Scan(&nextPosition); err != nil {
		return SyncResult{}, fmt.Errorf("mods: read max position: %w", err)
	}

	var result SyncResult
	now := time.Now().Unix()
	total := len(details)
	for i, d := range details {
		if !d.OK() {
			continue
		}
		result.Fetched++
		emit(progress, SyncEvent{
			Stage:   SyncStageUpserting,
			Current: i + 1,
			Total:   total,
			Name:    d.Title,
		})

		var existed int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM mods WHERE workshop_id = ?`, d.PublishedFileID,
		).Scan(&existed); err != nil {
			return SyncResult{}, fmt.Errorf("mods: check %s: %w", d.PublishedFileID, err)
		}

		if existed == 0 {
			nextPosition++
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mods (workshop_id, name, thumbnail, updated_at, position)
				 VALUES (?, ?, ?, ?, ?)`,
				d.PublishedFileID, d.Title, d.PreviewURL, now, nextPosition,
			); err != nil {
				return SyncResult{}, fmt.Errorf("mods: insert %s: %w", d.PublishedFileID, err)
			}
			result.NewMods++
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE mods SET name = ?, thumbnail = ?, updated_at = ?
				 WHERE workshop_id = ?`,
				d.Title, d.PreviewURL, now, d.PublishedFileID,
			); err != nil {
				return SyncResult{}, fmt.Errorf("mods: update %s: %w", d.PublishedFileID, err)
			}
			result.UpdatedMods++
		}

		extracted := steam.ExtractModIDs(d.Description)
		// A mod that publishes exactly one id is enabled by default; mods that
		// publish multiple require an explicit pick.
		defaultEnabled := 0
		if len(extracted) == 1 {
			defaultEnabled = 1
		}
		for _, modID := range extracted {
			var idExists int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM mod_ids WHERE workshop_id = ? AND mod_id = ?`,
				d.PublishedFileID, modID,
			).Scan(&idExists); err != nil {
				return SyncResult{}, fmt.Errorf("mods: check id %s/%s: %w", d.PublishedFileID, modID, err)
			}
			if idExists != 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mod_ids (workshop_id, mod_id, enabled) VALUES (?, ?, ?)`,
				d.PublishedFileID, modID, defaultEnabled,
			); err != nil {
				return SyncResult{}, fmt.Errorf("mods: insert id %s/%s: %w", d.PublishedFileID, modID, err)
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
		`SELECT workshop_id, name, thumbnail, updated_at
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
		if err := rows.Scan(&m.WorkshopID, &m.Name, &m.Thumbnail, &ts); err != nil {
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

// ListByPosition returns every mod ordered by stored position, with mod ids
// populated. Used by the load-order page and by apply.
func (s *Service) ListByPosition(ctx context.Context) ([]Mod, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workshop_id, name, thumbnail, updated_at
		 FROM mods
		 ORDER BY position, name COLLATE NOCASE`,
	)
	if err != nil {
		return nil, fmt.Errorf("mods: list by position: %w", err)
	}
	defer rows.Close()

	var out []Mod
	for rows.Next() {
		var m Mod
		var ts int64
		if err := rows.Scan(&m.WorkshopID, &m.Name, &m.Thumbnail, &ts); err != nil {
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

// Reorder persists a new position for every mod. enabledOrder is the
// user-chosen ordering of mods with at least one enabled mod id; disabled
// mods are appended afterwards in their existing relative order so the stored
// positions remain dense and unique across the whole set.
//
// enabledOrder must be a permutation of the currently-enabled workshop ids
// (no duplicates, no extras, no missing ones); any mismatch is rejected
// rather than silently coerced.
func (s *Service) Reorder(ctx context.Context, enabledOrder []string) error {
	seen := make(map[string]struct{}, len(enabledOrder))
	for _, id := range enabledOrder {
		if id == "" {
			return errors.New("mods: empty workshop id in order")
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("mods: duplicate workshop id %s in order", id)
		}
		seen[id] = struct{}{}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mods: begin reorder: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	enabledActual, err := queryIDs(ctx, tx, `
		SELECT DISTINCT m.workshop_id
		FROM mods m
		INNER JOIN mod_ids mi ON mi.workshop_id = m.workshop_id
		WHERE mi.enabled = 1
	`)
	if err != nil {
		return fmt.Errorf("mods: load enabled ids: %w", err)
	}
	if len(enabledActual) != len(seen) {
		return fmt.Errorf("mods: order has %d ids, %d mods are enabled", len(seen), len(enabledActual))
	}
	for _, id := range enabledActual {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("mods: workshop %s is enabled but not in supplied order", id)
		}
	}

	disabled, err := queryIDs(ctx, tx, `
		SELECT m.workshop_id FROM mods m
		WHERE m.workshop_id NOT IN (
			SELECT DISTINCT workshop_id FROM mod_ids WHERE enabled = 1
		)
		ORDER BY m.position, m.name COLLATE NOCASE
	`)
	if err != nil {
		return fmt.Errorf("mods: load disabled ids: %w", err)
	}

	full := make([]string, 0, len(enabledOrder)+len(disabled))
	full = append(full, enabledOrder...)
	full = append(full, disabled...)

	stmt, err := tx.PrepareContext(ctx, `UPDATE mods SET position = ? WHERE workshop_id = ?`)
	if err != nil {
		return fmt.Errorf("mods: prepare update: %w", err)
	}
	defer stmt.Close()

	for i, id := range full {
		res, err := stmt.ExecContext(ctx, i+1, id)
		if err != nil {
			return fmt.Errorf("mods: update %s: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("mods: %s: %w", id, ErrNotFound)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mods: commit reorder: %w", err)
	}
	return nil
}

// ResetOrderToCollection re-expands the Steam collection and rewrites every
// mod's position to match collection order. Mods present in the DB but no
// longer in the collection are pushed to the end in their current relative
// order. Workshop ids in the collection without a DB row are skipped — run
// Sync first to populate them.
//
// Toggle / per-id-enabled state is preserved; only `position` changes.
func (s *Service) ResetOrderToCollection(ctx context.Context, collectionID string) error {
	if collectionID == "" {
		return errors.New("mods: collection id is required")
	}
	collectionIDs, err := s.steam.ExpandCollection(ctx, collectionID)
	if err != nil {
		return fmt.Errorf("mods: expand collection: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mods: begin reset-order: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	allOrdered, err := queryIDs(ctx, tx, `
		SELECT workshop_id FROM mods
		ORDER BY position, name COLLATE NOCASE
	`)
	if err != nil {
		return fmt.Errorf("mods: load existing ids: %w", err)
	}
	known := make(map[string]struct{}, len(allOrdered))
	for _, id := range allOrdered {
		known[id] = struct{}{}
	}

	seen := make(map[string]struct{}, len(collectionIDs))
	full := make([]string, 0, len(allOrdered))
	for _, id := range collectionIDs {
		if _, ok := known[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		full = append(full, id)
	}
	for _, id := range allOrdered {
		if _, ok := seen[id]; !ok {
			full = append(full, id)
		}
	}

	stmt, err := tx.PrepareContext(ctx, `UPDATE mods SET position = ? WHERE workshop_id = ?`)
	if err != nil {
		return fmt.Errorf("mods: prepare update: %w", err)
	}
	defer stmt.Close()

	for i, id := range full {
		if _, err := stmt.ExecContext(ctx, i+1, id); err != nil {
			return fmt.Errorf("mods: update %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mods: commit reset-order: %w", err)
	}
	return nil
}

// MoveTo places workshopID at 1-indexed position within the enabled ordering,
// shifting the rest to accommodate. position is clamped to [1, len(enabled)].
func (s *Service) MoveTo(ctx context.Context, workshopID string, position int) error {
	if position < 1 {
		return errors.New("mods: position must be >= 1")
	}
	current, err := queryIDs(ctx, s.db, `
		SELECT DISTINCT m.workshop_id
		FROM mods m
		INNER JOIN mod_ids mi ON mi.workshop_id = m.workshop_id
		WHERE mi.enabled = 1
		ORDER BY m.position, m.name COLLATE NOCASE
	`)
	if err != nil {
		return fmt.Errorf("mods: load enabled: %w", err)
	}

	rest := make([]string, 0, len(current))
	found := false
	for _, id := range current {
		if id == workshopID {
			found = true
			continue
		}
		rest = append(rest, id)
	}
	if !found {
		return fmt.Errorf("mods: %s: %w", workshopID, ErrNotFound)
	}
	if position > len(current) {
		position = len(current)
	}
	target := position - 1
	ordered := make([]string, 0, len(current))
	ordered = append(ordered, rest[:target]...)
	ordered = append(ordered, workshopID)
	ordered = append(ordered, rest[target:]...)
	return s.Reorder(ctx, ordered)
}

// querier is the subset of *sql.DB / *sql.Tx that queryIDs needs, so the
// helper works inside or outside a transaction.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func queryIDs(ctx context.Context, q querier, sqlStr string) ([]string, error) {
	rows, err := q.QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// emit sends ev on progress if the channel is non-nil. Non-blocking: if the
// receiver is slow and the channel is full, the event is dropped rather than
// stall the sync transaction.
func emit(progress chan<- SyncEvent, ev SyncEvent) {
	if progress == nil {
		return
	}
	select {
	case progress <- ev:
	default:
	}
}
