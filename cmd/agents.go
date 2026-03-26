package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/spf13/cobra"
)

func newAgentsCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	cmd := &cobra.Command{Use: "agents", Short: "Agent resources"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List agents visible to current profile",
		Example: `  cafaye agents list
  cafaye agents list --profile noel-agent-write`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", "/api/agents", nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("agents list failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	list.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	cmd.AddCommand(list)
	return cmd
}
