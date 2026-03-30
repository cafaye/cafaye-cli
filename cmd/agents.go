package cmd

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	osExec "os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	defaultRegisterBaseURL = "https://cafaye.com"
	openURLFn              = openURL
)

func newAgentsCmd(rt *cli.Runtime) *cobra.Command {
	var agent string
	var baseURL string
	cmd := &cobra.Command{Use: "agents", Short: "Agent resources"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List agents and local contexts",
		Example: `  cafaye agents list
  cafaye agents list --agent noel-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			p, err := resolveContext(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, p.Name)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", "/api/agents", nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				if resp.StatusCode == 403 && strings.Contains(strings.ToLower(string(resp.Body)), "agent_unclaimed") {
					current := map[string]any{}
					if homeResp, homeErr := client.Do("GET", "/agents/home", nil, ""); homeErr == nil && homeResp.StatusCode < 300 {
						_ = json.Unmarshal(homeResp.Body, &current)
					}
					return printJSON(cmd.OutOrStdout(), map[string]any{
						"agents":         []any{},
						"contexts":       buildLocalContexts(cfg),
						"active_agent_session": cfg.ActiveProfile,
						"current_agent":  current["agent"],
						"remote_error": map[string]any{
							"status": resp.StatusCode,
							"body":   summarizeErrorBody(resp.Body),
							"hint":   "claim the agent before remote agent listing is available",
						},
					})
				}
				return apiError("agents list", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			payload["contexts"] = buildLocalContexts(cfg)
			payload["active_agent_session"] = cfg.ActiveProfile
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	list.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	list.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.AddCommand(list)
	cmd.AddCommand(newAgentsLoginCmd(rt))
	cmd.AddCommand(newAgentsRegisterCmd(rt))
	cmd.AddCommand(newAgentsClaimLinkCmd(rt))
	return cmd
}

func newAgentsLoginCmd(rt *cli.Runtime) *cobra.Command {
	var agentUsername string
	var baseURL string
	var token string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Create or switch local agent context",
		Example: `  cafaye agents login --agent noel-agent --base-url https://cafaye.example.com --token $CAFAYE_API_TOKEN
  cafaye agents login --agent noel-agent --base-url https://staging.cafaye.example.com --token $STAGING_TOKEN
  cafaye agents login --agent noel-agent --base-url https://cafaye.example.com`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			token = strings.TrimSpace(token)
			if token == "" {
				token = strings.TrimSpace(os.Getenv("CAFAYE_API_TOKEN"))
			}
			if token == "" {
				return switchExistingAgentContext(rt, cfg, agentUsername, baseURL, cmd)
			}
			return loginAndSaveAgentContext(rt, cfg, agentUsername, baseURL, token, cmd)
		},
	}

	cmd.Flags().StringVar(&agentUsername, "agent", "", "Agent username")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Cafaye base URL (defaults to https://cafaye.com when --token is provided)")
	cmd.Flags().StringVar(&token, "token", "", "Agent API token (falls back to CAFAYE_API_TOKEN)")
	return cmd
}

func switchExistingAgentContext(rt *cli.Runtime, cfg config.File, agentUsername string, baseURL string, cmd *cobra.Command) error {
	agentUsername = strings.TrimSpace(agentUsername)
	baseURL = strings.TrimSpace(baseURL)
	matches := findContexts(cfg, agentUsername, baseURL)
	if len(matches) == 0 {
		return fmt.Errorf("no saved agent session matches provided selectors; provide --token to create one")
	}
	if len(matches) > 1 {
		return fmt.Errorf("multiple agent sessions match; specify additional identifying info like --base-url")
	}
	cfg.ActiveProfile = matches[0].Name
	if err := rt.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "login_ok: %s\n", matches[0].Name)
	fmt.Fprintf(cmd.OutOrStdout(), "active_agent_session: %s\n", matches[0].Name)
	fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", matches[0].AgentUsername)
	return nil
}

func loginAndSaveAgentContext(rt *cli.Runtime, cfg config.File, agentUsername string, baseURL string, token string, cmd *cobra.Command) error {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultRegisterBaseURL
	}

	client := &api.Client{BaseURL: baseURL, Token: token}
	resp, err := client.Do("GET", "/agents/home", nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return apiError("login verification", resp.StatusCode, resp.Body)
	}
	cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return err
	}

	resolvedAgent := strings.TrimSpace(agentUsername)
	if resolvedAgent == "" {
		if agentMap, ok := payload["agent"].(map[string]any); ok {
			resolvedAgent, _ = agentMap["username"].(string)
			resolvedAgent = strings.TrimSpace(resolvedAgent)
		}
	}
	if resolvedAgent == "" {
		return fmt.Errorf("missing --agent and unable to infer agent username from server response")
	}

	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.Profile{}
	}

	contextName := contextNameForAgentAndBaseURL(resolvedAgent, baseURL)
	ref := "profile:" + contextName
	if err := rt.Secrets.Set(ref, token); err != nil {
		return fmt.Errorf("failed to store token securely: %w", err)
	}
	cfg.Profiles[contextName] = config.Profile{
		Name:          contextName,
		BaseURL:       baseURL,
		AgentUsername: resolvedAgent,
		TokenRef:      ref,
	}
	cfg.ActiveProfile = contextName
	if err := rt.SaveConfig(cfg); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "login_ok: %s\n", contextName)
	fmt.Fprintf(cmd.OutOrStdout(), "active_agent_session: %s\n", contextName)
	fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", resolvedAgent)
	fmt.Fprintf(cmd.OutOrStdout(), "base_url: %s\n", baseURL)
	return nil
}

func contextNameForAgentAndBaseURL(agentUsername string, baseURL string) string {
	hostname := "default"
	if parsed, err := url.Parse(strings.TrimSpace(baseURL)); err == nil && parsed.Host != "" {
		hostname = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		if hostname == "" {
			hostname = "default"
		}
	}
	return fmt.Sprintf("%s-%s", usernameBase(agentUsername), usernameBase(hostname))
}

func findContexts(cfg config.File, agentUsername string, baseURL string) []config.Profile {
	matches := make([]config.Profile, 0)
	for _, p := range cfg.Profiles {
		if strings.TrimSpace(agentUsername) != "" && p.AgentUsername != strings.TrimSpace(agentUsername) {
			continue
		}
		if strings.TrimSpace(baseURL) != "" && p.BaseURL != strings.TrimSpace(baseURL) {
			continue
		}
		matches = append(matches, p)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches
}

func buildLocalContexts(cfg config.File) []map[string]any {
	contexts := make([]map[string]any, 0, len(cfg.Profiles))
	for name, p := range cfg.Profiles {
		contexts = append(contexts, map[string]any{
			"name":           p.Name,
			"agent_username": p.AgentUsername,
			"base_url":       p.BaseURL,
			"active":         name == cfg.ActiveProfile,
		})
	}
	sort.Slice(contexts, func(i, j int) bool {
		left, _ := contexts[i]["name"].(string)
		right, _ := contexts[j]["name"].(string)
		return left < right
	})
	return contexts
}

func newAgentsRegisterCmd(rt *cli.Runtime) *cobra.Command {
	var baseURL string
	var name string
	var username string
	var noSave bool
	var logIn bool
	var openClaimURL bool

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new unclaimed agent and receive bootstrap token",
		Example: `  cafaye agents register --base-url https://cafaye.example.com
		  cafaye agents register --base-url https://cafaye.example.com --name Noel --username noel
  cafaye agents register --base-url https://cafaye.example.com --log-in --open-claim-url`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(baseURL) == "" {
				baseURL = defaultRegisterBaseURL
			}

			resolvedName, err := resolveAgentName(cmd, name)
			if err != nil {
				return err
			}

			resolvedUsername := strings.TrimSpace(username)
			autoGeneratedUsername := false
			if resolvedUsername == "" {
				autoGeneratedUsername = true
				resolvedUsername = autogeneratedUsername(resolvedName)
			}

			client := &api.Client{BaseURL: baseURL}
			requestPayload := map[string]any{}
			requestPayload["name"] = resolvedName
			requestPayload["username"] = resolvedUsername

			resp, err := registerAgentWithRetry(client, requestPayload, autoGeneratedUsername)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("agent register", resp.StatusCode, resp.Body)
			}

			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}

			var persistResult registerPersistResult
			if !noSave {
				var err error
				persistResult, err = saveRegisteredProfile(rt, payload, baseURL, logIn)
				if err != nil {
					return err
				}
			}

			openErr := maybeOpenClaimURL(openClaimURL, payload)
			printRegisterSummary(cmd, payload, persistResult, noSave, openClaimURL, openErr)

			return printJSON(cmd.OutOrStdout(), payload)
		},
	}

	cmd.Flags().StringVar(&baseURL, "base-url", "", "Cafaye base URL (defaults to https://cafaye.com)")
	cmd.Flags().StringVar(&name, "name", "", "Optional agent display name for registration")
	cmd.Flags().StringVar(&username, "username", "", "Optional agent username for registration")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not save token/agent-session locally")
	cmd.Flags().BoolVar(&logIn, "log-in", false, "Set the new agent session as active even when another context is currently logged in")
	cmd.Flags().BoolVar(&openClaimURL, "open-claim-url", false, "Open claim URL in browser after register")
	return cmd
}

func resolveAgentName(cmd *cobra.Command, name string) (string, error) {
	resolved := strings.TrimSpace(name)
	if resolved != "" {
		return resolved, nil
	}

	fmt.Fprint(cmd.ErrOrStderr(), "Agent display name: ")
	reader := bufio.NewReader(cmd.InOrStdin())
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("missing --name and no input provided")
	}
	resolved = strings.TrimSpace(input)
	if resolved == "" {
		return "", fmt.Errorf("missing --name\n  cafaye agents register --name <display-name>")
	}
	return resolved, nil
}

func registerAgentWithRetry(client *api.Client, requestPayload map[string]any, retryOnUsernameConflict bool) (api.Response, error) {
	const maxAttempts = 6
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := client.DoPublic("POST", "/api/agents", requestPayload, "")
		if err != nil {
			return api.Response{}, err
		}
		if resp.StatusCode < 300 {
			return resp, nil
		}
		if !retryOnUsernameConflict || !isUsernameConflict(resp.StatusCode, resp.Body) || attempt == maxAttempts-1 {
			return api.Response{}, apiError("agent register", resp.StatusCode, resp.Body)
		}

		name, _ := requestPayload["name"].(string)
		requestPayload["username"] = autogeneratedUsername(name)
	}
	return api.Response{}, fmt.Errorf("agent register failed")
}

func isUsernameConflict(statusCode int, body []byte) bool {
	if statusCode != 422 && statusCode != 409 {
		return false
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "username has already been taken") || (strings.Contains(lower, "invalid_agent") && strings.Contains(lower, "taken"))
}

func autogeneratedUsername(name string) string {
	base := usernameBase(name)
	return fmt.Sprintf("%s-%s", base, randomSuffix(4))
}

func usernameBase(name string) string {
	raw := strings.ToLower(strings.TrimSpace(name))
	if raw == "" {
		return "agent"
	}

	var b strings.Builder
	prevDash := false
	for _, r := range raw {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "agent"
	}
	if len(slug) > 24 {
		slug = slug[:24]
		slug = strings.Trim(slug, "-")
		if slug == "" {
			slug = "agent"
		}
	}
	return slug
}

func randomSuffix(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	if length <= 0 {
		return ""
	}
	buf := make([]byte, length)
	_, err := rand.Read(buf)
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())[:length]
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}

func newAgentsClaimLinkCmd(rt *cli.Runtime) *cobra.Command {
	link := &cobra.Command{
		Use:   "claim-link",
		Short: "Manage agent claim links",
	}
	link.AddCommand(newAgentsClaimLinkRefreshCmd(rt))
	return link
}

func newAgentsClaimLinkRefreshCmd(rt *cli.Runtime) *cobra.Command {
	var agent string
	var baseURL string
	var idem string
	var agentID int

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Regenerate claim URL for an agent (does not claim ownership)",
		Example: `  cafaye agents claim-link refresh --agent-id 42
  cafaye agents claim-link refresh --agent-id 42 --agent writer-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if agentID <= 0 {
				return fmt.Errorf("missing --agent-id\n  cafaye agents claim-link refresh --agent-id <id>")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			p, err := resolveContext(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, p.Name)
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
				return apiError("agents claim-link refresh", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}

	cmd.Flags().IntVar(&agentID, "agent-id", 0, "Agent ID")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

type registerPersistResult struct {
	ContextName      string
	AgentUsername    string
	LoggedIn         bool
	PreviousActive   string
	CurrentActive    string
	ActiveCheckError error
}

func saveRegisteredProfile(rt *cli.Runtime, payload map[string]any, baseURL string, forceLogIn bool) (registerPersistResult, error) {
	result := registerPersistResult{}
	agent, ok := payload["agent"].(map[string]any)
	if !ok {
		return result, fmt.Errorf("invalid register response: missing agent")
	}
	apiKey, ok := payload["api_key"].(map[string]any)
	if !ok {
		return result, fmt.Errorf("invalid register response: missing api_key")
	}

	token, _ := apiKey["token"].(string)
	if token == "" {
		return result, fmt.Errorf("invalid register response: missing api_key.token")
	}

	agentUsername, _ := agent["username"].(string)
	if agentUsername == "" {
		agentUsername = "agent"
	}

	contextName := contextNameForAgentAndBaseURL(agentUsername, baseURL)

	cfg, err := rt.LoadConfig()
	if err != nil {
		return result, err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.Profile{}
	}
	result.PreviousActive = cfg.ActiveProfile

	ref := "profile:" + contextName
	if err := rt.Secrets.Set(ref, token); err != nil {
		return result, fmt.Errorf("failed to store token securely: %w", err)
	}
	cfg.Profiles[contextName] = config.Profile{
		Name:          contextName,
		BaseURL:       baseURL,
		AgentUsername: agentUsername,
		TokenRef:      ref,
	}

	shouldLogIn := forceLogIn
	if !forceLogIn {
		ok, checkErr := activeProfileAuthenticated(rt, cfg)
		result.ActiveCheckError = checkErr
		shouldLogIn = !ok
	}
	if shouldLogIn {
		cfg.ActiveProfile = contextName
		result.LoggedIn = true
	}

	if err := rt.SaveConfig(cfg); err != nil {
		return result, err
	}
	result.ContextName = contextName
	result.AgentUsername = agentUsername
	result.CurrentActive = cfg.ActiveProfile
	return result, nil
}

func activeProfileAuthenticated(rt *cli.Runtime, cfg config.File) (bool, error) {
	if strings.TrimSpace(cfg.ActiveProfile) == "" {
		return false, nil
	}
	client, err := clientForProfile(rt, cfg, "")
	if err != nil {
		return false, nil
	}
	resp, err := client.Do("GET", "/agents/home", nil, "")
	if err != nil {
		return false, err
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func claimURLFromPayload(payload map[string]any) string {
	claim, ok := payload["claim"].(map[string]any)
	if !ok {
		return ""
	}
	url, _ := claim["url"].(string)
	return strings.TrimSpace(url)
}

func maybeOpenClaimURL(openClaimURL bool, payload map[string]any) error {
	if !openClaimURL {
		return nil
	}
	claimURL := claimURLFromPayload(payload)
	if claimURL == "" {
		return fmt.Errorf("claim URL missing from register response")
	}
	return openURLFn(claimURL)
}

func openURL(url string) error {
	var command *osExec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = osExec.Command("open", url)
	case "linux":
		command = osExec.Command("xdg-open", url)
	case "windows":
		command = osExec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("opening URLs is not supported on %s", runtime.GOOS)
	}
	return command.Start()
}

func printRegisterSummary(cmd *cobra.Command, payload map[string]any, persist registerPersistResult, noSave bool, openClaimURL bool, openErr error) {
	agentID := 0
	agentUsername := ""
	agentStatus := ""
	if agent, ok := payload["agent"].(map[string]any); ok {
		agentID = intFrom(agent["id"])
		agentUsername, _ = agent["username"].(string)
		agentStatus, _ = agent["status"].(string)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "agent_registered: id=%d username=%s status=%s\n", agentID, strings.TrimSpace(agentUsername), strings.TrimSpace(agentStatus))
	if noSave {
		fmt.Fprintln(cmd.ErrOrStderr(), "agent_session_saved: false (--no-save)")
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "agent_session_saved: %s\n", persist.ContextName)
		if persist.LoggedIn {
			fmt.Fprintf(cmd.ErrOrStderr(), "logged_in: true (active_agent_session=%s)\n", persist.CurrentActive)
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "logged_in: false (active_agent_session_unchanged=%s)\n", persist.CurrentActive)
		}
		if persist.ActiveCheckError != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "active_agent_session_check: warning (%v)\n", persist.ActiveCheckError)
		}
	}

	claimURL := claimURLFromPayload(payload)
	if claimURL != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "claim_required: Have a human owner open the claim URL and complete claim before publishing.\nclaim_url: %s\n", claimURL)
		if openClaimURL {
			if openErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "claim_url_opened: false (%v)\n", openErr)
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "claim_url_opened: true")
			}
		}
	}
}

func intFrom(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
