package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/spf13/cobra"
)

func newTokenCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	var yes bool
	cmd := &cobra.Command{Use: "token", Short: "Manage current API token"}
	show := &cobra.Command{
		Use:   "show",
		Short: "Show current API key metadata",
		Example: `  cafaye token show
  cafaye token show --profile noel-agent-write`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", "/api/key", nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("token show failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
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
		Short: "Rotate current profile token",
		Example: `  cafaye token rotate
  cafaye token rotate --profile noel-agent-write`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			p, err := rt.ActiveProfile(cfg, profile)
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
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
				return fmt.Errorf("token rotate failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			newToken, _ := payload["token"].(string)
			if newToken == "" {
				return fmt.Errorf("rotate response did not include token")
			}
			if err := rt.Secrets.Set(p.TokenRef, newToken); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "token_rotated: true")
			fmt.Fprintf(cmd.OutOrStdout(), "profile: %s\n", p.Name)
			return nil
		},
	}

	revoke := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke current profile token",
		Example: `  cafaye token revoke --yes
  cafaye token revoke --profile noel-agent-write --yes`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return fmt.Errorf("refusing revoke without --yes\n  cafaye token revoke --yes [--profile <name>]")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			p, err := rt.ActiveProfile(cfg, profile)
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
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
				return fmt.Errorf("token revoke failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			}
			_ = rt.Secrets.Delete(p.TokenRef)
			fmt.Fprintln(cmd.OutOrStdout(), "token_revoked: true")
			fmt.Fprintf(cmd.OutOrStdout(), "profile: %s\n", p.Name)
			return nil
		},
	}

	rotate.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	revoke.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	revoke.Flags().BoolVar(&yes, "yes", false, "Confirm revocation without interactive prompt")
	show.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")

	cmd.AddCommand(show, rotate, revoke)
	return cmd
}
