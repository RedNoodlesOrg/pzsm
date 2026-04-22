package steam

import (
	"regexp"
	"sort"
	"strings"
)

// Label whitespace must be restricted to horizontal whitespace so a bare
// `Mod ID:` followed by a newline doesn't let \s* gobble the newline and
// anchor the capture on the *following* line.
var modIDRegex = regexp.MustCompile(`(?i)Mod[ \t]?ID:[ \t]*(?:\[/b\][ \t]*)?([^\r\n]*?)(?:\r|\[/hr\]|\n|$)`)

// ModID is a PZ in-game mod identifier extracted from a workshop description,
// paired with the enabled flag we persist per id.
type ModID struct {
	ID      string
	Enabled bool
}

// ExtractModIDs parses PZ mod IDs from a workshop item description. When
// exactly one unique id is present it is marked enabled by default, matching
// PZ's single-mod convention; if there are multiple the caller must enable
// specific ids explicitly.
func ExtractModIDs(description string) []ModID {
	matches := modIDRegex.FindAllStringSubmatch(description, -1)
	seen := make(map[string]struct{}, len(matches))
	var unique []string
	for _, m := range matches {
		id := strings.TrimSpace(m[1])
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	sort.SliceStable(unique, func(i, j int) bool {
		if len(unique[i]) != len(unique[j]) {
			return len(unique[i]) < len(unique[j])
		}
		return unique[i] < unique[j]
	})
	if len(unique) == 0 {
		return nil
	}
	defaultEnabled := len(unique) == 1
	out := make([]ModID, len(unique))
	for i, id := range unique {
		out[i] = ModID{ID: id, Enabled: defaultEnabled}
	}
	return out
}
