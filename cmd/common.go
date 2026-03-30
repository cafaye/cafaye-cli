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

func clientForProfile(rt *cli.Runtime, cfg config.File, contextName string) (*api.Client, error) {
	p, err := rt.ActiveProfile(cfg, contextName)
	if err != nil {
		return nil, err
	}
	token, err := rt.Secrets.Get(p.TokenRef)
	if err != nil {
		return nil, fmt.Errorf("token for agent session %q not available: %w", p.Name, err)
	}
	return &api.Client{BaseURL: p.BaseURL, Token: token}, nil
}

func resolveContext(cfg config.File, agentSelector string, baseURLSelector string) (config.Profile, error) {
	agentSelector = strings.TrimSpace(agentSelector)
	baseURLSelector = strings.TrimSpace(baseURLSelector)

	if agentSelector == "" {
		name := cfg.ActiveProfile
		if name == "" {
			return config.Profile{}, fmt.Errorf("no active agent session set; run: cafaye agents login --agent <username> --base-url <url> --token <token>")
		}
		p, ok := cfg.Profiles[name]
		if !ok {
			return config.Profile{}, fmt.Errorf("agent session %q not found", name)
		}
		return p, nil
	}

	matches := make([]config.Profile, 0)
	for _, p := range cfg.Profiles {
		if p.AgentUsername != agentSelector {
			continue
		}
		if baseURLSelector != "" && p.BaseURL != baseURLSelector {
			continue
		}
		matches = append(matches, p)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })

	if len(matches) == 0 {
		return config.Profile{}, fmt.Errorf("no saved agent session for agent %q", agentSelector)
	}
	if len(matches) > 1 {
		return config.Profile{}, fmt.Errorf("multiple agent sessions match agent %q; provide --base-url", agentSelector)
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
