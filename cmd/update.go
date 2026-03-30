package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/version"
	"github.com/spf13/cobra"
)

func newUpdateCmd(rt *cli.Runtime) *cobra.Command {
	var agent string
	var baseURL string
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for CLI updates and migration guidance",
		Example: `  cafaye update --check
  cafaye update --check --agent noel-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			p, err := resolveContext(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, p.Name)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", "/api/cli/update?current_version="+version.Current, nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode == 404 {
				fmt.Fprintln(cmd.OutOrStdout(), "update_endpoint: unavailable")
				fmt.Fprintf(cmd.OutOrStdout(), "current_version: %s\n", version.Current)
				fmt.Fprintln(cmd.OutOrStdout(), "next_step: check release notes for your install channel")
				return nil
			}
			if resp.StatusCode >= 300 {
				return apiError("update check", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			if checkOnly {
				payload["mode"] = "check"
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().BoolVar(&checkOnly, "check", true, "Check only; do not self-update in place")
	return cmd
}
