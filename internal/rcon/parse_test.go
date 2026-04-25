package rcon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlayers_Fixtures(t *testing.T) {
	tests := []struct {
		file string
		want []string
	}{
		{"players_empty.txt", nil},
		{"players_one.txt", []string{"rj"}},
		{"players_many.txt", []string{"rj", "toUser", "Bjørn"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got := ParsePlayers(string(data))
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got=%v)", len(got), len(tt.want), got)
			}
			for i, p := range got {
				if p.Name != tt.want[i] {
					t.Errorf("[%d] name = %q, want %q", i, p.Name, tt.want[i])
				}
			}
		})
	}
}

func TestParsePlayers_CRLF(t *testing.T) {
	in := "Players connected (2):\r\n-rj\r\n-toUser\r\n"
	got := ParsePlayers(in)
	want := []string{"rj", "toUser"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Name != want[i] {
			t.Errorf("[%d] name = %q, want %q", i, p.Name, want[i])
		}
	}
}

func TestParsePlayers_NoTrailingNewline(t *testing.T) {
	got := ParsePlayers("Players connected (1):\n-rj")
	if len(got) != 1 || got[0].Name != "rj" {
		t.Errorf("unexpected: %v", got)
	}
}

func TestParsePlayers_GarbagePassthrough(t *testing.T) {
	// Defensive: PZ versions might omit the header. Treat any `-name` line
	// as a player.
	got := ParsePlayers("-rj\n-toUser\n")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}
