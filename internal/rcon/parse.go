package rcon

import "strings"

// Player is one connected player as reported by the `players` command.
type Player struct {
	Name string `json:"name"`
}

// ParsePlayers parses the response of the `players` RCON command.
// PZ replies with `Players connected (N):\n-name1\n-name2`. The header line
// is skipped; remaining non-empty lines are returned with the leading `-`
// trimmed. The result is never nil.
func ParsePlayers(s string) []Player {
	out := []Player{}
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Players connected") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if name == "" {
			continue
		}
		out = append(out, Player{Name: name})
	}
	return out
}
