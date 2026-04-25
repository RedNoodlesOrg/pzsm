// Package serverini reads and rewrites a Project Zomboid server ini file,
// preserving every byte not explicitly being changed including original line
// endings and surrounding comments.
package serverini

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Entry is one Key=Value setting parsed from the ini, with any `#` comment
// lines that immediately preceded it joined by newlines (with the leading
// "# " stripped).
type Entry struct {
	Key     string
	Value   string
	Comment string
}

// Read parses the ini at path into a flat list of Entry values in file order.
// Bare comment-only blocks at the end of the file are dropped; lines that are
// neither comments nor `Key=...` are ignored. The original file is not
// modified.
func Read(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("serverini: read %s: %w", path, err)
	}

	var entries []Entry
	var commentBuf []string

	for raw := range bytes.SplitSeq(data, []byte("\n")) {
		line := strings.TrimRight(string(raw), "\r")
		trimmed := strings.TrimLeft(line, " \t")

		switch {
		case trimmed == "":
			commentBuf = nil
		case strings.HasPrefix(trimmed, "#"):
			commentBuf = append(commentBuf, strings.TrimLeft(strings.TrimPrefix(trimmed, "#"), " "))
		default:
			eq := strings.IndexByte(trimmed, '=')
			if eq <= 0 {
				commentBuf = nil
				continue
			}
			entries = append(entries, Entry{
				Key:     trimmed[:eq],
				Value:   trimmed[eq+1:],
				Comment: strings.Join(commentBuf, "\n"),
			})
			commentBuf = nil
		}
	}
	return entries, nil
}

// UpdateMods atomically replaces the first Mods= and WorkshopItems= lines at
// path with ;-joined values. Both lines must already exist; leading whitespace
// on the matched lines is tolerated for detection but dropped in the rewrite
// (matching the Python reference implementation). CRLF line endings on the
// rewritten lines are preserved.
func UpdateMods(path string, enabledModIDs, workshopIDs []string) error {
	return writeFields(path, map[string]string{
		"Mods":          strings.Join(enabledModIDs, ";"),
		"WorkshopItems": strings.Join(workshopIDs, ";"),
	}, true)
}

// WriteFields atomically rewrites the first occurrence of each `Key=` line
// listed in updates with the supplied value. Every other byte in the file is
// preserved, including line endings and surrounding comments. It returns an
// error if any key in updates does not appear as a `Key=` line in the file --
// callers should validate keys against Read before calling.
func WriteFields(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	return writeFields(path, updates, true)
}

func writeFields(path string, updates map[string]string, requireAll bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("serverini: read %s: %w", path, err)
	}

	lines := bytes.Split(data, []byte("\n"))
	found := make(map[string]bool, len(updates))

	for i, line := range lines {
		trimmed := bytes.TrimLeft(line, " \t")
		eq := bytes.IndexByte(trimmed, '=')
		if eq <= 0 {
			continue
		}
		key := string(trimmed[:eq])
		val, ok := updates[key]
		if !ok || found[key] {
			continue
		}
		lines[i] = rewriteLine(line, key+"=", val)
		found[key] = true
	}

	if requireAll {
		for key := range updates {
			if !found[key] {
				return fmt.Errorf("serverini: %s: missing %s= line", path, key)
			}
		}
	}

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
