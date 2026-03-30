package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	osExec "os/exec"
	"strconv"
	"strings"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/version"
	"github.com/spf13/cobra"
)

const latestReleaseAPIURL = "https://api.github.com/repos/cafaye/cafaye-cli/releases/latest"

var (
	detectBrewInstallFn   = detectBrewInstall
	runBrewUpgradeFn      = runBrewUpgrade
	runInstallerUpgradeFn = runInstallerUpgrade
	fetchLatestVersionFn  = fetchLatestVersion
)

func newUpdateCmd(rt *cli.Runtime) *cobra.Command {
	var checkOnly bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update CLI to latest release (runs check first)",
		Example: `  cafaye update
  cafaye update --check
  cafaye update --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			latestVersion, err := fetchLatestVersionFn()
			if err != nil {
				return fmt.Errorf("update check: %w", err)
			}

			updateAvailable := isUpdateAvailable(version.Current, latestVersion)
			currentVersion := normalizeVersion(version.Current)
			latestVersionNormalized := normalizeVersion(latestVersion)

			result := map[string]any{
				"current_version": currentVersion,
				"latest_version":  latestVersionNormalized,
			}
			if checkOnly {
				result["mode"] = "check"
				result["update_available"] = updateAvailable
				result["up_to_date"] = !updateAvailable
				if jsonOutput {
					return printJSON(cmd.OutOrStdout(), result)
				}
				printUpdateHumanCheck(cmd.OutOrStdout(), currentVersion, latestVersionNormalized, updateAvailable)
				return nil
			}

			result["mode"] = "update"
			result["update_available"] = updateAvailable
			result["up_to_date"] = !updateAvailable
			if !updateAvailable {
				result["updated"] = false
				result["message"] = "already up to date"
				if jsonOutput {
					return printJSON(cmd.OutOrStdout(), result)
				}
				printUpdateHumanCheck(cmd.OutOrStdout(), currentVersion, latestVersionNormalized, false)
				fmt.Fprintln(cmd.OutOrStdout(), "Already up to date.")
				return nil
			}

			if !jsonOutput {
				printUpdateHumanCheck(cmd.OutOrStdout(), currentVersion, latestVersionNormalized, true)
			}
			if detectBrewInstallFn() {
				if err := runBrewUpgradeFn(); err != nil {
					result["updated"] = false
					result["method"] = "brew"
					result["error"] = err.Error()
					if jsonOutput {
						return printJSON(cmd.OutOrStdout(), result)
					}
					return fmt.Errorf("update failed (brew): %w", err)
				}
				result["updated"] = true
				result["method"] = "brew"
				if jsonOutput {
					return printJSON(cmd.OutOrStdout(), result)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Updating via Homebrew...")
				fmt.Fprintf(cmd.OutOrStdout(), "Update complete: v%s\n", latestVersionNormalized)
				return nil
			}

			if err := runInstallerUpgradeFn(); err != nil {
				result["updated"] = false
				result["method"] = "install-script"
				result["error"] = err.Error()
				result["next_step"] = "if installed with Homebrew, run: brew upgrade cafaye/cafaye-cli/cafaye"
				if jsonOutput {
					return printJSON(cmd.OutOrStdout(), result)
				}
				return fmt.Errorf("update failed (install-script): %w", err)
			}
			result["updated"] = true
			result["method"] = "install-script"
			if jsonOutput {
				return printJSON(cmd.OutOrStdout(), result)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Updating via install script...")
			fmt.Fprintf(cmd.OutOrStdout(), "Update complete: v%s\n", latestVersionNormalized)
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Check only; do not perform update")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func printUpdateHumanCheck(w io.Writer, currentVersion, latestVersion string, updateAvailable bool) {
	fmt.Fprintln(w, "Checking latest release...")
	fmt.Fprintf(w, "Current version: v%s\n", currentVersion)
	fmt.Fprintf(w, "Latest version: v%s\n", latestVersion)
	if updateAvailable {
		fmt.Fprintln(w, "Update available.")
	} else {
		fmt.Fprintln(w, "Up to date.")
	}
}

func fetchLatestVersion() (string, error) {
	req, err := http.NewRequest(http.MethodGet, latestReleaseAPIURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cafaye-cli/"+version.Current)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	latestVersion := firstNonEmptyString(payload.TagName, payload.Name)
	if latestVersion == "" {
		return "", fmt.Errorf("missing latest version from release metadata")
	}
	return latestVersion, nil
}

func firstNonEmptyString(vals ...string) string {
	for _, s := range vals {
		if strings.TrimSpace(s) != "" {
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
	currentNorm := normalizeVersion(current)
	latestNorm := normalizeVersion(latest)
	if currentNorm == "" || latestNorm == "" {
		return false
	}
	cmp, ok := compareVersionNumbers(currentNorm, latestNorm)
	if !ok {
		return false
	}
	return cmp < 0
}

func compareVersionNumbers(current, latest string) (int, bool) {
	currentParts, ok := parseVersionParts(current)
	if !ok {
		return 0, false
	}
	latestParts, ok := parseVersionParts(latest)
	if !ok {
		return 0, false
	}
	maxLen := len(currentParts)
	if len(latestParts) > maxLen {
		maxLen = len(latestParts)
	}
	for i := range maxLen {
		currentValue := 0
		latestValue := 0
		if i < len(currentParts) {
			currentValue = currentParts[i]
		}
		if i < len(latestParts) {
			latestValue = latestParts[i]
		}
		if currentValue < latestValue {
			return -1, true
		}
		if currentValue > latestValue {
			return 1, true
		}
	}
	return 0, true
}

func parseVersionParts(versionValue string) ([]int, bool) {
	parts := strings.Split(versionValue, ".")
	parsed := make([]int, 0, len(parts))
	for _, part := range parts {
		segment := strings.TrimSpace(part)
		if segment == "" {
			return nil, false
		}
		num, err := strconv.Atoi(segment)
		if err != nil {
			return nil, false
		}
		parsed = append(parsed, num)
	}
	return parsed, true
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
