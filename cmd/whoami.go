package cmd

import (
	"encoding/json"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/spf13/cobra"
)

func newWhoAmICmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show active Cafaye identity",
		Example: `  cafaye whoami
  cafaye whoami --profile noel-agent-write`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", "/agents/home", nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("whoami", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	return cmd
}
