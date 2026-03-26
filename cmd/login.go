package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCmd(rt *cli.Runtime) *cobra.Command {
	var name, baseURL, agent, token string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Add/update profile and verify credentials",
		Example: `  cafaye login --name noel-agent-write --base-url https://cafaye.example.com --agent noel-agent --token $CAFAYE_API_TOKEN
  cafaye login --name editorial-agent --base-url https://cafaye.example.com --agent editorial-agent --token $TOKEN`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || baseURL == "" || token == "" {
				return fmt.Errorf("missing required flags\n  cafaye login --name <name> --base-url <url> --agent <username> --token <token>")
			}
			if agent == "" {
				agent = name
			}

			client := &api.Client{BaseURL: baseURL, Token: token}
			resp, err := client.Do("GET", "/agents/home", nil, "")
			if err != nil {
				return err
			}
			if resp.StatusCode >= 300 {
				return fmt.Errorf("login verification failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}

			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Profiles == nil {
				cfg.Profiles = map[string]config.Profile{}
			}
			ref := "profile:" + name
			if err := rt.Secrets.Set(ref, token); err != nil {
				return fmt.Errorf("failed to store token securely: %w", err)
			}
			cfg.Profiles[name] = config.Profile{Name: name, BaseURL: baseURL, AgentUsername: agent, TokenRef: ref}
			cfg.ActiveProfile = name
			if err := rt.SaveConfig(cfg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "login_ok: %s\n", name)
			fmt.Fprintf(cmd.OutOrStdout(), "active_profile: %s\n", name)
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile name")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Cafaye base URL")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username")
	cmd.Flags().StringVar(&token, "token", "", "Agent API token")
	return cmd
}
