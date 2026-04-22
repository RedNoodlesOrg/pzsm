package config

import (
	"errors"
	"os"
	"path/filepath"
)

// Config holds the runtime configuration loaded from environment variables.
type Config struct {
	ListenAddr        string `json:"listen_addr,omitempty"`
	DatabasePath      string `json:"database_path,omitempty"`
	PZServerFolder    string `json:"pz_server_folder,omitempty"`
	ServertestINI     string `json:"servertest_ini,omitempty"`
	DockerContainer   string `json:"docker_container,omitempty"`
	DockerSocket      string `json:"docker_socket,omitempty"`
	RCONHost          string `json:"rcon_host,omitempty"`
	RCONPort          string `json:"rcon_port,omitempty"`
	RCONPassword      string `json:"rcon_password,omitempty"`
	SteamCollectionID string `json:"steam_collection_id,omitempty"`
	// DevUser, when set, bypasses Cloudflare Access: unauthenticated requests
	// are treated as this user. Leave unset in production.
	DevUser string `json:"dev_user,omitempty"`
}

// Load reads configuration from the environment. Variables required for the
// current slice are validated; optional ones used by later features are read
// but not checked.
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr:        envOr("LISTEN_ADDR", ":8080"),
		DatabasePath:      os.Getenv("DATABASE_PATH"),
		PZServerFolder:    os.Getenv("PZ_SERVER_FOLDER"),
		ServertestINI:     os.Getenv("PZ_SERVERTEST_INI"),
		DockerContainer:   envOr("DOCKER_CONTAINER", "pzserver"),
		DockerSocket:      envOr("DOCKER_SOCKET", "unix:///var/run/docker.sock"),
		RCONHost:          os.Getenv("RCON_HOST"),
		RCONPort:          envOr("RCON_PORT", "27015"),
		RCONPassword:      os.Getenv("RCON_PASSWORD"),
		SteamCollectionID: os.Getenv("STEAM_COLLECTION_ID"),
		DevUser:           os.Getenv("DEV_USER_EMAIL"),
	}
	if cfg.DatabasePath == "" {
		return nil, errors.New("config: DATABASE_PATH is required")
	}
	if cfg.ServertestINI == "" && cfg.PZServerFolder != "" {
		cfg.ServertestINI = filepath.Join(cfg.PZServerFolder, "Server", "servertest.ini")
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
