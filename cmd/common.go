package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
)

func clientForAgentSession(rt *cli.Runtime, cfg config.File, agentSessionName string) (*api.Client, error) {
	session, err := rt.ActiveAgentSession(cfg, agentSessionName)
	if err != nil {
		return nil, err
	}
	token, err := rt.Secrets.Get(session.TokenRef)
	if err != nil {
		return nil, fmt.Errorf("token for agent session %q not available: %w", session.Name, err)
	}
	return &api.Client{BaseURL: session.BaseURL, Token: token}, nil
}

func resolveAgentSession(cfg config.File, agentSelector string, baseURLSelector string) (config.AgentSession, error) {
	agentSelector = strings.TrimSpace(agentSelector)
	baseURLSelector = strings.TrimSpace(baseURLSelector)

	if agentSelector == "" {
		name := cfg.ActiveAgentSession
		if name == "" {
			return config.AgentSession{}, fmt.Errorf("no active agent session set; run: cafaye agents login --agent <username> --base-url <url> --token <token>")
		}
		session, ok := cfg.AgentSessions[name]
		if !ok {
			return config.AgentSession{}, fmt.Errorf("agent session %q not found", name)
		}
		return session, nil
	}

	matches := make([]config.AgentSession, 0)
	for _, session := range cfg.AgentSessions {
		if session.AgentUsername != agentSelector {
			continue
		}
		if baseURLSelector != "" && session.BaseURL != baseURLSelector {
			continue
		}
		matches = append(matches, session)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })

	if len(matches) == 0 {
		return config.AgentSession{}, fmt.Errorf("no saved agent session for agent %q", agentSelector)
	}
	if len(matches) > 1 {
		return config.AgentSession{}, fmt.Errorf("multiple agent sessions match agent %q; provide --base-url", agentSelector)
	}
	return matches[0], nil
}

func printJSON(out io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(b))
	return nil
}
