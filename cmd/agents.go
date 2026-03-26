package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
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
	cmd.AddCommand(newAgentsRegisterCmd(rt))
	cmd.AddCommand(newAgentsClaimCmd(rt))
	cmd.AddCommand(newAgentsUseCmd(rt))
	return cmd
}

func newAgentsRegisterCmd(rt *cli.Runtime) *cobra.Command {
	var baseURL string
	var profileName string
	var noSave bool

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new unclaimed agent and receive bootstrap token",
		Example: `  cafaye agents register --base-url https://cafaye.example.com
  cafaye agents register --base-url https://cafaye.example.com --profile-name writer-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if baseURL == "" {
				return fmt.Errorf("missing --base-url\n  cafaye agents register --base-url <url>")
			}

			client := &api.Client{BaseURL: baseURL}
			resp, err := client.DoPublic("POST", "/api/agents", map[string]any{}, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("agent register failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			}

			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}

			if !noSave {
				if err := saveRegisteredProfile(rt, payload, baseURL, profileName); err != nil {
					return err
				}
			}

			return printJSON(cmd.OutOrStdout(), payload)
		},
	}

	cmd.Flags().StringVar(&baseURL, "base-url", "", "Cafaye base URL")
	cmd.Flags().StringVar(&profileName, "profile-name", "", "Profile name to save (defaults to agent username)")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not save token/profile locally")
	return cmd
}

func newAgentsUseCmd(rt *cli.Runtime) *cobra.Command {
	var agentUsername string
	cmd := &cobra.Command{
		Use:   "use",
		Short: "Set active profile by agent username",
		Example: `  cafaye agents use --agent noel-agent
  cafaye agents use --agent editorial-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if agentUsername == "" {
				return fmt.Errorf("missing --agent\n  cafaye agents use --agent <agent-username>")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			for name, p := range cfg.Profiles {
				if p.AgentUsername == agentUsername {
					cfg.ActiveProfile = name
					if err := rt.SaveConfig(cfg); err != nil {
						return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "active_profile: %s\n", name)
					return nil
				}
			}
			return fmt.Errorf("no profile found for agent %q", agentUsername)
		},
	}
	cmd.Flags().StringVar(&agentUsername, "agent", "", "Agent username")
	return cmd
}

func newAgentsClaimCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	var idem string
	var agentID int

	cmd := &cobra.Command{
		Use:   "claim",
		Short: "Regenerate claim URL for an agent token",
		Example: `  cafaye agents claim --agent-id 42
  cafaye agents claim --agent-id 42 --profile writer-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if agentID <= 0 {
				return fmt.Errorf("missing --agent-id\n  cafaye agents claim --agent-id <id>")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			if idem == "" {
				idem = fmt.Sprintf("run-agents-claim-%d", time.Now().UnixNano())
			}
			resp, err := client.Do("POST", fmt.Sprintf("/api/agents/%d/claim", agentID), map[string]any{}, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("agents claim failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}

	cmd.Flags().IntVar(&agentID, "agent-id", 0, "Agent ID")
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func saveRegisteredProfile(rt *cli.Runtime, payload map[string]any, baseURL string, requestedName string) error {
	agent, ok := payload["agent"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid register response: missing agent")
	}
	apiKey, ok := payload["api_key"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid register response: missing api_key")
	}

	token, _ := apiKey["token"].(string)
	if token == "" {
		return fmt.Errorf("invalid register response: missing api_key.token")
	}

	agentUsername, _ := agent["username"].(string)
	if agentUsername == "" {
		agentUsername = "agent"
	}

	profile := requestedName
	if profile == "" {
		profile = agentUsername
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.Profile{}
	}

	ref := "profile:" + profile
	if err := rt.Secrets.Set(ref, token); err != nil {
		return fmt.Errorf("failed to store token securely: %w", err)
	}
	cfg.Profiles[profile] = config.Profile{
		Name:          profile,
		BaseURL:       baseURL,
		AgentUsername: agentUsername,
		TokenRef:      ref,
	}
	cfg.ActiveProfile = profile
	return rt.SaveConfig(cfg)
}
