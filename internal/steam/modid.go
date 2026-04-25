package steam

import (
	"regexp"
	"sort"
	"strings"
)

// Anchored to the start of a line via (?m). The prefix before `Mod ID:` may
// contain BBCode tags or digit-bearing tokens (`v1.1`, `B41`, `2024-01-01`)
// but not pure-letter qualifier words, so `OLD MOD ID:` and `make sure to set
// the same Mod ID:` are rejected while `v1.1 Mod ID: snowiswater` is kept.
// Also tolerates an optional parenthetical between ID and colon
// (`Mod ID (b41): X`). Label whitespace stays `[ \t]` so a bare `Mod ID:`
// followed by `\n` does not gobble the newline.
var modIDRegex = regexp.MustCompile(
	`(?im)^[ \t]*(?:(?:\[[^\]]+\]|\S*\d\S*)[ \t]*)*Mod[ \t]?ID(?:[ \t]*\([^)]*\))?[ \t]*:[ \t]*(?:\[/[a-z0-9]+\][ \t]*)*([^\r\n]*?)(?:\r|\[/hr\]|\n|$)`,
)

// ExtractModIDs parses unique PZ in-game mod identifiers from a workshop item
// description, deduplicated and sorted by (length, lex). Whether to enable any
// of them is a persistence concern and lives in the caller.
func ExtractModIDs(description string) []string {
	matches := modIDRegex.FindAllStringSubmatch(description, -1)
	seen := make(map[string]struct{}, len(matches))
	var unique []string
	for _, m := range matches {
		id := m[1]
		// Cut at the first `[` so trailing BBCode like `Mod ID: Foo[/b]`
		// (and inline annotations like `Foo  [b](deprecated)[/b]`) don't
		// leak into the id.
		if i := strings.IndexByte(id, '['); i >= 0 {
			id = id[:i]
		}
		id = strings.TrimSpace(id)
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
	return unique
}
