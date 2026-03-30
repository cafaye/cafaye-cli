package cmd

import (
	"encoding/json"
	"fmt"
	osExec "os/exec"
	"strings"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	detectBrewInstallFn   = detectBrewInstall
	runBrewUpgradeFn      = runBrewUpgrade
	runInstallerUpgradeFn = runInstallerUpgrade
	updateBaseURL         = defaultRegisterBaseURL
)

func newUpdateCmd(rt *cli.Runtime) *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update CLI to latest release (runs check first)",
		Example: `  cafaye update
  cafaye update --check`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := &api.Client{BaseURL: updateBaseURL}
			resp, err := client.DoPublic("GET", "/api/cli/update?current_version="+version.Current, nil, "")
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

			latestVersion := firstNonEmptyString(payload["latest_version"], payload["latest"])
			minSupported := firstNonEmptyString(payload["minimum_supported_version"])
			result := map[string]any{
				"current_version":           version.Current,
				"latest_version":            latestVersion,
				"minimum_supported_version": minSupported,
			}
			if checkOnly {
				result["mode"] = "check"
				result["update_available"] = isUpdateAvailable(version.Current, latestVersion)
				result["up_to_date"] = !result["update_available"].(bool)
				return printJSON(cmd.OutOrStdout(), result)
			}

			updateAvailable := isUpdateAvailable(version.Current, latestVersion)
			result["mode"] = "update"
			result["update_available"] = updateAvailable
			result["up_to_date"] = !updateAvailable
			if !updateAvailable {
				result["updated"] = false
				result["message"] = "already up to date"
				return printJSON(cmd.OutOrStdout(), result)
			}

			if detectBrewInstallFn() {
				if err := runBrewUpgradeFn(); err != nil {
					result["updated"] = false
					result["method"] = "brew"
					result["error"] = err.Error()
					return printJSON(cmd.OutOrStdout(), result)
				}
				result["updated"] = true
				result["method"] = "brew"
				return printJSON(cmd.OutOrStdout(), result)
			}

			if err := runInstallerUpgradeFn(); err != nil {
				result["updated"] = false
				result["method"] = "install-script"
				result["error"] = err.Error()
				result["next_step"] = "if installed with Homebrew, run: brew upgrade cafaye/cafaye-cli/cafaye"
				return printJSON(cmd.OutOrStdout(), result)
			}
			result["updated"] = true
			result["method"] = "install-script"
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Check only; do not perform update")
	return cmd
}

func firstNonEmptyString(vals ...any) string {
	for _, v := range vals {
		s, ok := v.(string)
		if ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func normalizeVersion(v string) string {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	return s
}

func isUpdateAvailable(current, latest string) bool {
	if normalizeVersion(current) == "" || normalizeVersion(latest) == "" {
		return false
	}
	return normalizeVersion(current) != normalizeVersion(latest)
}

func detectBrewInstall() bool {
	if _, err := osExec.LookPath("brew"); err != nil {
		return false
	}
	cmd := osExec.Command("brew", "list", "--versions", "cafaye")
	return cmd.Run() == nil
}

func runBrewUpgrade() error {
	cmd := osExec.Command("brew", "upgrade", "cafaye/cafaye-cli/cafaye")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runInstallerUpgrade() error {
	script := "curl -fsSL https://raw.githubusercontent.com/cafaye/cafaye-cli/master/scripts/install.sh | bash"
	cmd := osExec.Command("bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
