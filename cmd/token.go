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

func newAgentsTokenCmd(rt *cli.Runtime) *cobra.Command {
	var agent string
	var baseURL string
	var token string
	var yes bool

	cmd := &cobra.Command{Use: "token", Short: "Manage agent session tokens"}

	create := &cobra.Command{
		Use:   "create",
		Short: "Create or update stored token for an agent session",
		Example: `  cafaye agents token create --agent noel-agent --base-url https://cafaye.example.com --token $CAFAYE_API_TOKEN
  cafaye agents token create --base-url https://cafaye.example.com --token $CAFAYE_API_TOKEN`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedToken := strings.TrimSpace(token)
			if resolvedToken == "" {
				return fmt.Errorf("missing --token\n  cafaye agents token create --agent <username> --base-url <url> --token <token>")
			}

			resolvedBaseURL := strings.TrimSpace(baseURL)
			if resolvedBaseURL == "" {
				resolvedBaseURL = defaultRegisterBaseURL
			}

			client := &api.Client{BaseURL: resolvedBaseURL, Token: resolvedToken}
			resp, err := client.Do("GET", "/agents/home", nil, "")
			if err != nil {
				return err
			}
			if resp.StatusCode >= 300 {
				return apiError("agents token create verification", resp.StatusCode, resp.Body)
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)

			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}

			resolvedAgent := strings.TrimSpace(agent)
			if agentMap, ok := payload["agent"].(map[string]any); ok {
				serverAgent, _ := agentMap["username"].(string)
				serverAgent = strings.TrimSpace(serverAgent)
				if resolvedAgent == "" {
					resolvedAgent = serverAgent
				}
				if serverAgent != "" && resolvedAgent != "" && resolvedAgent != serverAgent {
					return fmt.Errorf("provided --agent %q does not match token identity %q", resolvedAgent, serverAgent)
				}
			}
			if resolvedAgent == "" {
				return fmt.Errorf("missing --agent and unable to infer agent username from server response")
			}

			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			if cfg.AgentSessions == nil {
				cfg.AgentSessions = map[string]config.AgentSession{}
			}

			agentSessionName := agentSessionNameForAgentAndBaseURL(resolvedAgent, resolvedBaseURL)
			ref := "agent_session:" + agentSessionName
			if err := rt.Secrets.Set(ref, resolvedToken); err != nil {
				return fmt.Errorf("failed to store token securely: %w", err)
			}
			upsertAgentSession(&cfg, config.AgentSession{
				Name:          agentSessionName,
				BaseURL:       resolvedBaseURL,
				AgentUsername: resolvedAgent,
				TokenRef:      ref,
			})
			if strings.TrimSpace(cfg.ActiveAgentSession) == "" {
				cfg.ActiveAgentSession = agentSessionName
			}
			if err := rt.SaveConfig(cfg); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "token_created: true")
			fmt.Fprintf(cmd.OutOrStdout(), "agent_session: %s\n", agentSessionName)
			fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", resolvedAgent)
			fmt.Fprintf(cmd.OutOrStdout(), "base_url: %s\n", resolvedBaseURL)
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

	create.Flags().StringVar(&agent, "agent", "", "Agent username (optional when token can infer identity)")
	create.Flags().StringVar(&baseURL, "base-url", "", "Cafaye base URL (defaults to https://cafaye.com)")
	create.Flags().StringVar(&token, "token", "", "Agent API token")
	_ = create.MarkFlagRequired("token")

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
