package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type AgentSession struct {
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	AgentUsername string `json:"agent_username"`
	AgentRef      string `json:"agent_ref,omitempty"`
	TokenRef      string `json:"token_ref"`
}

type File struct {
	ActiveAgentSession string                  `json:"active_agent_session"`
	AgentSessions      map[string]AgentSession `json:"agent_sessions"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "cafaye", "config.json"), nil
}

func Load(path string) (File, error) {
	cfg := File{AgentSessions: map[string]AgentSession{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return File{}, err
	}
	if len(data) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return File{}, err
	}
	if cfg.AgentSessions == nil {
		cfg.AgentSessions = map[string]AgentSession{}
	}
	return cfg, nil
}

func Save(path string, cfg File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
