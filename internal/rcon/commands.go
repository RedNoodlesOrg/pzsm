package rcon

import (
	"context"
	"fmt"
	"strings"
)

// Players runs the `players` command and returns the parsed list.
func (s *Service) Players(ctx context.Context) ([]Player, error) {
	resp, err := s.Exec(ctx, "players")
	if err != nil {
		return nil, err
	}
	return ParsePlayers(resp), nil
}

// Kick runs `kickuser "name"` with an optional reason. Returns the raw
// response from PZ.
func (s *Service) Kick(ctx context.Context, name, reason string) (string, error) {
	cmd, err := build("kickuser", name)
	if err != nil {
		return "", err
	}
	if reason != "" {
		quoted, err := FormatArg(reason)
		if err != nil {
			return "", err
		}
		cmd += " -r " + quoted
	}
	return s.Exec(ctx, cmd)
}

// WhitelistAdd runs `adduser "user" "password"`.
func (s *Service) WhitelistAdd(ctx context.Context, user, password string) (string, error) {
	cmd, err := build("adduser", user, password)
	if err != nil {
		return "", err
	}
	return s.Exec(ctx, cmd)
}

// WhitelistRemove runs `removeuserfromwhitelist "user"`.
func (s *Service) WhitelistRemove(ctx context.Context, user string) (string, error) {
	cmd, err := build("removeuserfromwhitelist", user)
	if err != nil {
		return "", err
	}
	return s.Exec(ctx, cmd)
}

// Save runs `save`.
func (s *Service) Save(ctx context.Context) (string, error) {
	return s.Exec(ctx, "save")
}

// Quit runs `quit` (saves world and stops the server).
func (s *Service) Quit(ctx context.Context) (string, error) {
	return s.Exec(ctx, "quit")
}

// ServerMsg broadcasts a chat message to all connected players.
func (s *Service) ServerMsg(ctx context.Context, message string) (string, error) {
	cmd, err := build("servermsg", message)
	if err != nil {
		return "", err
	}
	return s.Exec(ctx, cmd)
}

// ReloadOptions reloads servertest.ini from disk.
func (s *Service) ReloadOptions(ctx context.Context) (string, error) {
	return s.Exec(ctx, "reloadoptions")
}

// ReloadLua reloads a Lua script on the server by filename.
func (s *Service) ReloadLua(ctx context.Context, filename string) (string, error) {
	cmd, err := build("reloadlua", filename)
	if err != nil {
		return "", err
	}
	return s.Exec(ctx, cmd)
}

func build(verb string, args ...string) (string, error) {
	var b strings.Builder
	b.WriteString(verb)
	for _, a := range args {
		quoted, err := FormatArg(a)
		if err != nil {
			return "", fmt.Errorf("rcon %s: %w", verb, err)
		}
		b.WriteByte(' ')
		b.WriteString(quoted)
	}
	return b.String(), nil
}
