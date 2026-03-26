package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
)

func clientForProfile(rt *cli.Runtime, cfg config.File, profileName string) (*api.Client, error) {
	p, err := rt.ActiveProfile(cfg, profileName)
	if err != nil {
		return nil, err
	}
	token, err := rt.Secrets.Get(p.TokenRef)
	if err != nil {
		return nil, fmt.Errorf("token for profile %q not available: %w", p.Name, err)
	}
	return &api.Client{BaseURL: p.BaseURL, Token: token}, nil
}

func printJSON(out io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(b))
	return nil
}
