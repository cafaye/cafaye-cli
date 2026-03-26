package cmd

import (
	"fmt"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/spf13/cobra"
)

func newProfileCmd(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage local CLI profiles"}
	cmd.AddCommand(newProfileAddCmd(rt), newProfileUseCmd(rt), newProfileListCmd(rt))
	return cmd
}

func newProfileAddCmd(rt *cli.Runtime) *cobra.Command {
	var name, baseURL, agent, token string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update a profile",
		Example: `  cafaye profile add --name noel-agent-write --base-url https://cafaye.example.com --agent noel-agent --token $CAFAYE_API_TOKEN
  cafaye profile add --name noel-agent-publish --base-url https://cafaye.example.com --agent noel-agent --token $PUBLISH_TOKEN`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || baseURL == "" || agent == "" || token == "" {
				return fmt.Errorf("missing required flags\n  cafaye profile add --name <name> --base-url <url> --agent <username> --token <token>")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			ref := "profile:" + name
			if err := rt.Secrets.Set(ref, token); err != nil {
				return fmt.Errorf("failed to store token securely: %w", err)
			}
			if cfg.Profiles == nil {
				cfg.Profiles = map[string]config.Profile{}
			}
			cfg.Profiles[name] = config.Profile{Name: name, BaseURL: baseURL, AgentUsername: agent, TokenRef: ref}
			if cfg.ActiveProfile == "" {
				cfg.ActiveProfile = name
			}
			if err := rt.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "profile_saved: %s\n", name)
			fmt.Fprintf(cmd.OutOrStdout(), "active_profile: %s\n", cfg.ActiveProfile)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile name")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Cafaye base URL")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username")
	cmd.Flags().StringVar(&token, "token", "", "Agent API token")
	return cmd
}

func newProfileUseCmd(rt *cli.Runtime) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:     "use",
		Short:   "Set active profile",
		Example: `  cafaye profile use --name noel-agent-write`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return fmt.Errorf("missing --name\n  cafaye profile use --name <name>")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[name]; !ok {
				return fmt.Errorf("profile %q not found", name)
			}
			cfg.ActiveProfile = name
			if err := rt.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active_profile: %s\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile name")
	return cmd
}

func newProfileListCmd(rt *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			for name, p := range cfg.Profiles {
				marker := ""
				if name == cfg.ActiveProfile {
					marker = " (active)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", p.Name, marker)
			}
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no profiles configured")
			}
			return nil
		},
	}
}
