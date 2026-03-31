package cmd

import (
	"encoding/json"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/spf13/cobra"
)

func newWhoAmICmd(rt *cli.Runtime) *cobra.Command {
	var agent string
	var agentRef string
	var baseURL string
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show active Cafaye identity",
		Example: `  cafaye whoami
  cafaye whoami --agent noel-agent
  cafaye whoami --agent noel-agent --base-url https://cafaye.com`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			agentSelector, err := resolveAgentSelector(agent, agentRef)
			if err != nil {
				return err
			}
			currSession, err := resolveAgentSession(cfg, agentSelector, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForAgentSession(rt, cfg, currSession.Name)
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
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&agentRef, "agent-ref", "", "Agent reference ID (agent_...)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	return cmd
}
