package mods

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := []string{
		`CREATE TABLE mods (
			workshop_id TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			thumbnail   TEXT NOT NULL,
			updated_at  INTEGER NOT NULL,
			position    INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE mod_ids (
			workshop_id TEXT NOT NULL REFERENCES mods(workshop_id) ON DELETE CASCADE,
			mod_id      TEXT NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (workshop_id, mod_id)
		)`,
	}
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

// seed inserts a mod with a single mod_id matching workshopID and the given
// enabled flag, at the supplied position.
func seed(t *testing.T, db *sql.DB, workshopID string, enabled bool, position int) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO mods (workshop_id, name, thumbnail, updated_at, position) VALUES (?, ?, '', 0, ?)`,
		workshopID, workshopID, position,
	); err != nil {
		t.Fatalf("seed mod: %v", err)
	}
	en := 0
	if enabled {
		en = 1
	}
	if _, err := db.Exec(
		`INSERT INTO mod_ids (workshop_id, mod_id, enabled) VALUES (?, ?, ?)`,
		workshopID, "id_"+workshopID, en,
	); err != nil {
		t.Fatalf("seed mod_id: %v", err)
	}
}

func positions(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	rows, err := db.Query(`SELECT workshop_id, position FROM mods`)
	if err != nil {
		t.Fatalf("select positions: %v", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var pos int
		if err := rows.Scan(&id, &pos); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = pos
	}
	return out
}

func orderedEnabled(t *testing.T, svc *Service) []string {
	t.Helper()
	mods, err := svc.ListByPosition(context.Background())
	if err != nil {
		t.Fatalf("ListByPosition: %v", err)
	}
	var out []string
	for _, m := range mods {
		for _, id := range m.ModIDs {
			if id.Enabled {
				out = append(out, m.WorkshopID)
				break
			}
		}
	}
	return out
}

func TestReorder_PersistsNewPositions(t *testing.T) {
	db := newTestDB(t)
	for i, ws := range []string{"A", "B", "C"} {
		seed(t, db, ws, true, i+1)
	}
	svc := &Service{db: db}

	if err := svc.Reorder(context.Background(), []string{"C", "A", "B"}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	got := positions(t, db)
	want := map[string]int{"C": 1, "A": 2, "B": 3}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("position[%s] = %d, want %d", k, got[k], v)
		}
	}
}

func TestReorder_DisabledAppendedInPositionOrder(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	seed(t, db, "X", false, 2)
	seed(t, db, "B", true, 3)
	seed(t, db, "Y", false, 4)
	svc := &Service{db: db}

	if err := svc.Reorder(context.Background(), []string{"B", "A"}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	got := positions(t, db)
	want := map[string]int{"B": 1, "A": 2, "X": 3, "Y": 4}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("position[%s] = %d, want %d", k, got[k], v)
		}
	}
}

func TestReorder_RejectsUnknownID(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	svc := &Service{db: db}

	err := svc.Reorder(context.Background(), []string{"ZZZ"})
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
}

func TestReorder_RejectsDuplicateID(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	seed(t, db, "B", true, 2)
	svc := &Service{db: db}

	err := svc.Reorder(context.Background(), []string{"A", "A"})
	if err == nil {
		t.Fatal("expected error for duplicate id")
	}
}

func TestReorder_RejectsWrongCount(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	seed(t, db, "B", true, 2)
	svc := &Service{db: db}

	err := svc.Reorder(context.Background(), []string{"A"})
	if err == nil {
		t.Fatal("expected error when supplied order misses an enabled mod")
	}
}

func TestMoveTo_ShiftsOthers(t *testing.T) {
	db := newTestDB(t)
	for i, ws := range []string{"A", "B", "C", "D"} {
		seed(t, db, ws, true, i+1)
	}
	svc := &Service{db: db}

	// Move D (currently position 4) to position 2.
	if err := svc.MoveTo(context.Background(), "D", 2); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}

	got := orderedEnabled(t, svc)
	want := []string{"A", "D", "B", "C"}
	if !equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestMoveTo_ClampsAboveMax(t *testing.T) {
	db := newTestDB(t)
	for i, ws := range []string{"A", "B", "C"} {
		seed(t, db, ws, true, i+1)
	}
	svc := &Service{db: db}

	if err := svc.MoveTo(context.Background(), "A", 999); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}

	got := orderedEnabled(t, svc)
	want := []string{"B", "C", "A"}
	if !equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestMoveTo_RejectsDisabledMod(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	seed(t, db, "X", false, 2)
	svc := &Service{db: db}

	err := svc.MoveTo(context.Background(), "X", 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMoveTo_RejectsZeroPosition(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	svc := &Service{db: db}

	if err := svc.MoveTo(context.Background(), "A", 0); err == nil {
		t.Fatal("expected error for position 0")
	}
}

func TestListByPosition_FollowsPosition(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "Zebra", true, 1)
	seed(t, db, "Alpha", true, 2)
	seed(t, db, "Mango", true, 3)
	svc := &Service{db: db}

	mods, err := svc.ListByPosition(context.Background())
	if err != nil {
		t.Fatalf("ListByPosition: %v", err)
	}
	var got []string
	for _, m := range mods {
		got = append(got, m.WorkshopID)
	}
	want := []string{"Zebra", "Alpha", "Mango"}
	if !equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestPruneMissing_DeletesAbsentAndCascades(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	seed(t, db, "B", false, 2)
	seed(t, db, "C", true, 3)

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	n, err := pruneMissing(context.Background(), tx, []string{"A", "C"})
	if err != nil {
		t.Fatalf("pruneMissing: %v", err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want 1", n)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got := positions(t, db)
	if _, ok := got["B"]; ok {
		t.Errorf("expected B to be deleted, still present at position %d", got["B"])
	}
	if _, ok := got["A"]; !ok {
		t.Errorf("A was deleted, want kept")
	}
	if _, ok := got["C"]; !ok {
		t.Errorf("C was deleted, want kept")
	}

	var idCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM mod_ids WHERE workshop_id = 'B'`).Scan(&idCount); err != nil {
		t.Fatalf("count mod_ids: %v", err)
	}
	if idCount != 0 {
		t.Errorf("mod_ids for B = %d, want 0 (FK cascade)", idCount)
	}
}

func TestPruneMissing_EmptyKeepIsNoop(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, "A", true, 1)
	seed(t, db, "B", true, 2)

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	n, err := pruneMissing(context.Background(), tx, nil)
	if err != nil {
		t.Fatalf("pruneMissing: %v", err)
	}
	if n != 0 {
		t.Errorf("removed = %d, want 0", n)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got := positions(t, db)
	if len(got) != 2 {
		t.Errorf("len(mods) = %d, want 2 (empty keep must not wipe table)", len(got))
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
