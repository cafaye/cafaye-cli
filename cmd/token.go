package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/spf13/cobra"
)

func newAgentsTokenCmd(rt *cli.Runtime) *cobra.Command {
	var agent string
	var baseURL string
	var token string
	var yes bool

	cmd := &cobra.Command{Use: "token", Short: "Manage agent session tokens"}

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a new token server-side and store it for an agent session",
		Example: `  cafaye agents token create
  cafaye agents token create --agent noel-agent --base-url https://cafaye.example.com`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(token) != "" {
				return fmt.Errorf("--token import is no longer supported; run without --token to issue a fresh server token")
			}

			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			currSession, err := resolveAgentSession(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForAgentSession(rt, cfg, currSession.Name)
			if err != nil {
				return err
			}

			idem := fmt.Sprintf("run-token-create-%d", time.Now().UnixNano())
			resp, err := client.Do("POST", "/api/key", map[string]any{"name": "cli-issued"}, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("agents token create", resp.StatusCode, resp.Body)
			}

			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			resolvedToken, _ := payload["token"].(string)
			if strings.TrimSpace(resolvedToken) == "" {
				return fmt.Errorf("token create response did not include token")
			}
			ref := currSession.TokenRef
			if strings.TrimSpace(ref) == "" {
				ref = "agent_session:" + currSession.Name
			}
			if err := rt.Secrets.Set(ref, resolvedToken); err != nil {
				return fmt.Errorf("failed to store token securely: %w", err)
			}
			if cfg.AgentSessions == nil {
				cfg.AgentSessions = map[string]config.AgentSession{}
			}
			currSession.TokenRef = ref
			cfg.AgentSessions[currSession.Name] = currSession
			if err := rt.SaveConfig(cfg); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "token_created: true")
			fmt.Fprintf(cmd.OutOrStdout(), "agent_session: %s\n", currSession.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", currSession.AgentUsername)
			fmt.Fprintf(cmd.OutOrStdout(), "base_url: %s\n", currSession.BaseURL)
			return nil
		},
	}

	show := &cobra.Command{
		Use:   "show",
		Short: "Show current API key metadata",
		Example: `  cafaye agents token show
  cafaye agents token show --agent noel-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			currSession, err := resolveAgentSession(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForAgentSession(rt, cfg, currSession.Name)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", "/api/key", nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("token show", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}

	rotate := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate current agent token",
		Example: `  cafaye agents token rotate
  cafaye agents token rotate --agent noel-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			currSession, err := resolveAgentSession(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForAgentSession(rt, cfg, currSession.Name)
			if err != nil {
				return err
			}
			idem := fmt.Sprintf("run-rotate-%d", time.Now().UnixNano())
			resp, err := client.Do("PATCH", "/api/key/rotate", map[string]any{}, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("token rotate", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			newToken, _ := payload["token"].(string)
			if newToken == "" {
				return fmt.Errorf("rotate response did not include token")
			}
			if err := rt.Secrets.Set(currSession.TokenRef, newToken); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "token_rotated: true")
			fmt.Fprintf(cmd.OutOrStdout(), "agent_session: %s\n", currSession.Name)
			return nil
		},
	}

	revoke := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke current agent token",
		Example: `  cafaye agents token revoke --yes
  cafaye agents token revoke --agent noel-agent --yes`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return fmt.Errorf("refusing revoke without --yes\n  cafaye agents token revoke --yes [--agent <username>] [--base-url <url>]")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			currSession, err := resolveAgentSession(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForAgentSession(rt, cfg, currSession.Name)
			if err != nil {
				return err
			}
			idem := fmt.Sprintf("run-revoke-%d", time.Now().UnixNano())
			resp, err := client.Do("PATCH", "/api/key/revoke", map[string]any{}, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("token revoke", resp.StatusCode, resp.Body)
			}
			_ = rt.Secrets.Delete(currSession.TokenRef)
			fmt.Fprintln(cmd.OutOrStdout(), "token_revoked: true")
			fmt.Fprintf(cmd.OutOrStdout(), "agent_session: %s\n", currSession.Name)
			return nil
		},
	}

	create.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	create.Flags().StringVar(&baseURL, "base-url", "", "Cafaye base URL (defaults to https://cafaye.com)")
	create.Flags().StringVar(&token, "token", "", "Deprecated token import flag (unsupported)")

	rotate.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	rotate.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	revoke.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	revoke.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	revoke.Flags().BoolVar(&yes, "yes", false, "Confirm revocation without interactive prompt")
	show.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	show.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")

	cmd.AddCommand(create, show, rotate, revoke)
	return cmd
}
