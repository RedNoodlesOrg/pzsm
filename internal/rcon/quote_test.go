package rcon

import (
	"errors"
	"testing"
)

func TestFormatArg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", `""`},
		{"bare", "rj", `rj`},
		{"unicode bare", "Bjørn", `Bjørn`},
		{"with space", "two words", `"two words"`},
		{"with quote", `say "hi"`, `"say \"hi\""`},
		{"with backslash", `a\b`, `"a\\b"`},
		{"leading space", " rj", `" rj"`},
		{"only quote", `"`, `"\""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatArg(tt.in)
			if err != nil {
				t.Fatalf("FormatArg(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("FormatArg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatArg_RejectsControlChars(t *testing.T) {
	tests := []string{
		"\x00",
		"abc\x01def",
		"line\nfeed",
		"carriage\rreturn",
		"a\tb",
		"del\x7f",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := FormatArg(in)
			if !errors.Is(err, ErrControlChar) {
				t.Errorf("FormatArg(%q) err = %v, want ErrControlChar", in, err)
			}
		})
	}
}
