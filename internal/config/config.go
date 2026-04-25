// Package config loads runtime configuration from a YAML file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the runtime configuration loaded from the YAML file.
type Config struct {
	ListenAddr        string `yaml:"listen_addr"`
	DatabasePath      string `yaml:"database_path"`
	PZServerFolder    string `yaml:"pz_server_folder"`
	ServertestINI     string `yaml:"servertest_ini"`
	DockerContainer   string `yaml:"docker_container"`
	DockerSocket      string `yaml:"docker_socket"`
	RCONHost          string `yaml:"rcon_host"`
	RCONPort          string `yaml:"rcon_port"`
	RCONPassword      string `yaml:"rcon_password"`
	SteamCollectionID string `yaml:"steam_collection_id"`
	SteamWebAPIKey    string `yaml:"steam_web_api_key"`
	// DevUser is honored only by builds compiled with -tags devbypass; the
	// default (prod) build drops the bypass branch entirely via dead-code
	// elimination, so this field has no effect there.
	DevUser string `yaml:"dev_user_email"`
}

// Load reads the YAML file at path, applies defaults, and validates required fields.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.DockerContainer == "" {
		cfg.DockerContainer = "pzserver"
	}
	if cfg.DockerSocket == "" {
		cfg.DockerSocket = "unix:///var/run/docker.sock"
	}
	if cfg.RCONPort == "" {
		cfg.RCONPort = "27015"
	}

	if cfg.DatabasePath == "" {
		return nil, errors.New("config: database_path is required")
	}
	if cfg.SteamWebAPIKey == "" {
		return nil, errors.New("config: steam_web_api_key is required (get one at https://steamcommunity.com/dev/apikey)")
	}

	if cfg.ServertestINI == "" && cfg.PZServerFolder != "" {
		cfg.ServertestINI = filepath.Join(cfg.PZServerFolder, "Server", "servertest.ini")
	}

	return &cfg, nil
}
