package cli

import (
	"fmt"
	"io"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/cafaye/cafaye-cli/internal/creds"
)

type Runtime struct {
	ConfigPath string
	Secrets    creds.Store
	Out        io.Writer
	ErrOut     io.Writer
	HTTPClient *api.Client
}

func (r *Runtime) LoadConfig() (config.File, error) {
	return config.Load(r.ConfigPath)
}

func (r *Runtime) SaveConfig(cfg config.File) error {
	return config.Save(r.ConfigPath, cfg)
}

func (r *Runtime) ActiveAgentSession(cfg config.File, explicit string) (config.AgentSession, error) {
	name := explicit
	if name == "" {
		name = cfg.ActiveAgentSession
	}
	if name == "" {
		return config.AgentSession{}, fmt.Errorf("no active agent session set; run: cafaye agents login --agent <username> --base-url <url> --token <token>")
	}
	p, ok := cfg.AgentSessions[name]
	if !ok {
		return config.AgentSession{}, fmt.Errorf("agent session %q not found", name)
	}
	return p, nil
}

func PrintDeprecation(w io.Writer, n api.DeprecationNotice) {
	if !n.Deprecated {
		return
	}
	fmt.Fprintln(w, "warning: API deprecation notice")
	if n.Message != "" {
		fmt.Fprintf(w, "message: %s\n", n.Message)
	}
	if n.Replacement != "" {
		fmt.Fprintf(w, "replacement: %s\n", n.Replacement)
	}
	if n.Sunset != "" {
		fmt.Fprintf(w, "sunset: %s\n", n.Sunset)
	}
	if n.DocsURL != "" {
		fmt.Fprintf(w, "docs: %s\n", n.DocsURL)
	}
}
