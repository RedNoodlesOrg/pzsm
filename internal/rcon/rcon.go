// Package rcon is a thin client over Project Zomboid's Source RCON listener.
// Each Exec call dials, authenticates, sends one command, and closes; PZ's
// RCON drops idle connections and pooling adds bugs without saving meaningful
// latency for human-triggered actions.
package rcon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/gorcon/rcon"
)

// ErrNotConfigured is returned by Exec when host or password are unset.
var ErrNotConfigured = errors.New("rcon: host or password not configured")

// Service holds the RCON connection parameters.
type Service struct {
	host     string
	port     string
	password string
	timeout  time.Duration
	log      *slog.Logger
}

// New constructs a Service. host and password may be empty; Exec will then
// return ErrNotConfigured. port falls back to "27015" if empty.
func New(host, port, password string, log *slog.Logger) *Service {
	if port == "" {
		port = "27015"
	}
	return &Service{
		host:     host,
		port:     port,
		password: password,
		timeout:  5 * time.Second,
		log:      log,
	}
}

// Configured reports whether the service has the minimum needed to dial.
func (s *Service) Configured() bool {
	return s.host != "" && s.password != ""
}

// Exec dials, authenticates, runs cmd, and closes. The caller is responsible
// for any argument quoting (see FormatArg). ctx is checked between dial and
// execute; gorcon does not accept a context directly, so cancellation during
// the network round-trip relies on the configured deadline.
func (s *Service) Exec(ctx context.Context, cmd string) (string, error) {
	if !s.Configured() {
		return "", ErrNotConfigured
	}
	addr := net.JoinHostPort(s.host, s.port)
	conn, err := rcon.Dial(addr, s.password,
		rcon.SetDialTimeout(s.timeout),
		rcon.SetDeadline(s.timeout),
	)
	if err != nil {
		return "", fmt.Errorf("rcon: dial %s: %w", addr, err)
	}
	defer conn.Close()

	if err := ctx.Err(); err != nil {
		return "", err
	}

	resp, err := conn.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("rcon: execute: %w", err)
	}
	return resp, nil
}
