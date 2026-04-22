// Package serverini rewrites the Mods= and WorkshopItems= lines of a Project
// Zomboid server ini file, preserving every other byte including original line
// endings.
package serverini

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UpdateMods atomically replaces the first Mods= and WorkshopItems= lines at
// path with ;-joined values. Both lines must already exist; leading whitespace
// on the matched lines is tolerated for detection but dropped in the rewrite
// (matching the Python reference implementation). CRLF line endings on the
// rewritten lines are preserved.
func UpdateMods(path string, enabledModIDs, workshopIDs []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("serverini: read %s: %w", path, err)
	}

	lines := bytes.Split(data, []byte("\n"))

	modsIdx, workshopIdx := -1, -1
	for i, line := range lines {
		trimmed := bytes.TrimLeft(line, " \t")
		if modsIdx == -1 && bytes.HasPrefix(trimmed, []byte("Mods=")) {
			modsIdx = i
		}
		if workshopIdx == -1 && bytes.HasPrefix(trimmed, []byte("WorkshopItems=")) {
			workshopIdx = i
		}
		if modsIdx != -1 && workshopIdx != -1 {
			break
		}
	}
	if modsIdx == -1 {
		return fmt.Errorf("serverini: %s: missing Mods= line", path)
	}
	if workshopIdx == -1 {
		return fmt.Errorf("serverini: %s: missing WorkshopItems= line", path)
	}

	lines[modsIdx] = rewriteLine(lines[modsIdx], "Mods=", strings.Join(enabledModIDs, ";"))
	lines[workshopIdx] = rewriteLine(lines[workshopIdx], "WorkshopItems=", strings.Join(workshopIDs, ";"))

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("serverini: stat %s: %w", path, err)
	}

	return writeAtomic(path, bytes.Join(lines, []byte("\n")), info.Mode().Perm())
}

// rewriteLine replaces the body of a "key=..." line with value, preserving a
// trailing \r if the original line had one (CRLF files).
func rewriteLine(line []byte, key, value string) []byte {
	suffix := ""
	if len(line) > 0 && line[len(line)-1] == '\r' {
		suffix = "\r"
	}
	return []byte(key + value + suffix)
}

// writeAtomic writes data to a sibling temp file and renames it over path so a
// crash mid-write cannot leave the original truncated.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("serverini: create temp: %w", err)
	}
	tmpName := tmp.Name()
	// os.Rename below removes tmpName on success; this covers the failure paths.
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("serverini: write temp: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("serverini: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("serverini: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("serverini: rename: %w", err)
	}
	return nil
}
