package config

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	in := File{ActiveAgentSession: "p1", AgentSessions: map[string]AgentSession{"p1": {Name: "p1", BaseURL: "https://x", AgentUsername: "a", TokenRef: "r"}}}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.ActiveAgentSession != "p1" || out.AgentSessions["p1"].BaseURL != "https://x" {
		t.Fatalf("unexpected config: %+v", out)
	}
}
