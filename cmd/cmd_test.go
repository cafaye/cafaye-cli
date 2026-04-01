package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/cafaye/cafaye-cli/internal/creds"
	"github.com/cafaye/cafaye-cli/internal/version"
	"github.com/spf13/cobra"
)

func testRuntime(t *testing.T) (*cli.Runtime, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("CAFAYE_BOOKS_DIR", filepath.Join(tmp, "books"))
	cfgPath := filepath.Join(tmp, "config.json")
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	rt := &cli.Runtime{
		ConfigPath: cfgPath,
		Secrets:    creds.NewMemoryStore(),
		Out:        out,
		ErrOut:     errOut,
	}
	return rt, out, errOut, cfgPath
}

func exec(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	root.SetArgs(args)
	return root.Execute()
}

func seedAgentSession(t *testing.T, rt *cli.Runtime, name string, baseURL string, token string) {
	t.Helper()
	cfg := config.File{ActiveAgentSession: name, AgentSessions: map[string]config.AgentSession{
		name: {Name: name, BaseURL: baseURL, AgentUsername: "agent", TokenRef: "agent_session:" + name},
	}}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := rt.Secrets.Set("agent_session:"+name, token); err != nil {
		t.Fatal(err)
	}
}

func TestAgentsTokenCreateAndList(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/key":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer oldtok" {
				t.Fatalf("expected bearer oldtok, got: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"api_key":{"id":7,"name":"cli-issued","scopes":["books:write"]},"token":"newtok"}`))
		case "/api/agents":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"agents":[{"id":1,"username":"a1"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()

	cfg := config.File{ActiveAgentSession: "a1-localhost", AgentSessions: map[string]config.AgentSession{
		"a1-localhost": {Name: "a1-localhost", BaseURL: s.URL, AgentUsername: "a1", TokenRef: "agent_session:a1-localhost"},
	}}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := rt.Secrets.Set("agent_session:a1-localhost", "oldtok"); err != nil {
		t.Fatal(err)
	}

	if err := exec(t, root, "agents", "token", "create", "--agent", "a1", "--base-url", s.URL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "token_created: true") {
		t.Fatalf("expected token create output, got: %s", out.String())
	}
	stored, err := rt.Secrets.Get("agent_session:a1-localhost")
	if err != nil {
		t.Fatal(err)
	}
	if stored != "newtok" {
		t.Fatalf("expected rotated token to be stored, got: %q", stored)
	}
	out.Reset()

	if err := exec(t, root, "agents", "list", "--agent", "a1", "--base-url", s.URL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"agent_sessions"`) {
		t.Fatalf("expected agent_sessions in agents list, got: %s", out.String())
	}
}

func TestAgentsLoginWithoutSavedSessionFails(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	err := exec(t, root, "agents", "login", "--agent", "a1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agents register") {
		t.Fatalf("expected agent session selection error, got: %v", err)
	}
}

func TestAgentsLoginRequiresUsernameNotDisplayName(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	cfg := config.File{AgentSessions: map[string]config.AgentSession{
		"noel-localhost": {
			Name:          "noel-localhost",
			BaseURL:       "https://cafaye.com",
			AgentUsername: "noel",
			TokenRef:      "agent_session:noel-localhost",
		},
	}}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	err := exec(t, root, "agents", "login", "--agent", "Noel")
	if err == nil {
		t.Fatal("expected login selector failure for display name")
	}
	if !strings.Contains(err.Error(), "no saved agent session matches provided selectors") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWhoamiShowsDeprecationGuidance(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cafaye-Deprecated", "true")
		w.Header().Set("X-Cafaye-Replacement", "/agents/home")
		_, _ = w.Write([]byte(`{"agent":{"id":1}}`))
	}))
	defer s.Close()

	rt, out, errOut, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "whoami"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "agent") {
		t.Fatalf("expected payload, got: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "warning: API deprecation notice") {
		t.Fatalf("expected deprecation warning, got: %s", errOut.String())
	}
}

func TestWhoamiFailureSummarizesHTMLBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>boom</body></html>"))
	}))
	defer s.Close()

	rt, _, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	err := exec(t, root, "whoami")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("expected status in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "<html error response omitted>") {
		t.Fatalf("expected summarized html marker, got: %v", err)
	}
}

func TestAPIErrorPrefersJSONMessage(t *testing.T) {
	err := apiError("books list", 422, []byte(`{"error":"price_cents must be positive"}`))
	if !strings.Contains(err.Error(), "price_cents must be positive") {
		t.Fatalf("expected json message in error, got: %v", err)
	}
}

func TestUpdateCheckFailsWhenReleaseLookupFails(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	prev := fetchLatestVersionFn
	fetchLatestVersionFn = func() (string, error) { return "", fmt.Errorf("boom") }
	defer func() { fetchLatestVersionFn = prev }()

	err := exec(t, root, "update", "--check")
	if err == nil {
		t.Fatal("expected update check error")
	}
	if !strings.Contains(err.Error(), "update check: boom") {
		t.Fatalf("expected wrapped update check error, got: %v", err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected no stdout on failure, got: %s", out.String())
	}
}

func TestUploadDryRun(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "upload", "--file", "bundle.zip", "--idempotency-key", "run-12345", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dry_run: true") {
		t.Fatalf("expected dry_run output, got: %s", out.String())
	}
}

func TestUploadRequiresIdempotencyKey(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	err := exec(t, root, "books", "upload", "--file", "bundle.zip")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing --idempotency-key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadSupportsStdin(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/uploads" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload":{"id":1,"status":"accepted"}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	root.SetIn(strings.NewReader("zipbytes"))

	if err := exec(t, root, "books", "upload", "--stdin", "--idempotency-key", "run-12345"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "upload") {
		t.Fatalf("expected upload payload, got: %s", out.String())
	}
}

func TestRootHelpHasExamples(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "--help"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "cafaye agents login") {
		t.Fatalf("expected examples in help, got: %s", out.String())
	}
}

func TestSkillsInstallWritesSkillToTargetRoot(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	target := filepath.Join(t.TempDir(), "bundle")
	if err := exec(t, root, "skills", "install", "--root", target); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "skill_path:") {
		t.Fatalf("expected skill path output, got: %s", out.String())
	}
	skillPath := filepath.Join(target, ".agents", "skills", "cafaye", "SKILL.md")
	body, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected skill at %s: %v", skillPath, err)
	}
	if !strings.Contains(string(body), "managed-by: cafaye-cli") {
		t.Fatalf("expected managed skill header, got: %s", string(body))
	}
}

func TestBooksCreateCreatesSlugWorkspaceAndInstallsSkill(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/books" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"book":{"id":42,"slug":"my-new-book","title":"My New Book","subtitle":"Sub","author":"Noel Agent"}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	custom := filepath.Join(t.TempDir(), "books")
	if err := exec(t, root, "books", "create", "--title", "My New Book", "--subtitle", "Sub", "--books-dir", custom); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"workspace_path": "`+filepath.Join(custom, "my-new-book")+`"`) {
		t.Fatalf("expected slug workspace in output, got: %s", out.String())
	}
	skillPath := filepath.Join(custom, "my-new-book", ".agents", "skills", "cafaye", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected workspace skill at %s: %v", skillPath, err)
	}
	bookYML := filepath.Join(custom, "my-new-book", "book.yml")
	body, err := os.ReadFile(bookYML)
	if err != nil {
		t.Fatalf("expected book.yml in slug workspace: %v", err)
	}
	if !strings.Contains(string(body), `title: "My New Book"`) {
		t.Fatalf("expected book.yml title to be set from API response: %s", string(body))
	}
}

func TestBooksCreateSupportsSkipTemplates(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/books" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"book":{"id":43,"slug":"bare-workspace","title":"Bare Workspace","author":"Noel Agent"}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	custom := filepath.Join(t.TempDir(), "books")
	if err := exec(t, root, "books", "create", "--title", "Bare Workspace", "--books-dir", custom, "--skip-templates"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"templates_skipped": true`) {
		t.Fatalf("expected templates_skipped=true output, got: %s", out.String())
	}
	workspacePath := filepath.Join(custom, "bare-workspace")
	if _, err := os.Stat(filepath.Join(workspacePath, "book.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected no starter book.yml when templates are skipped")
	}
	if _, err := os.Stat(filepath.Join(workspacePath, ".agents", "skills", "cafaye", "SKILL.md")); err != nil {
		t.Fatalf("expected skill to still be installed: %v", err)
	}
}

func TestUpdateCheckReturnsLatestReleasePayload(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	prev := fetchLatestVersionFn
	fetchLatestVersionFn = func() (string, error) { return "v9.9.9", nil }
	defer func() { fetchLatestVersionFn = prev }()

	if err := exec(t, root, "update", "--check"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Checking latest release...") {
		t.Fatalf("expected human update check header, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "Current version: v") {
		t.Fatalf("expected current version in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "Latest version: v9.9.9") {
		t.Fatalf("expected latest version in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "Update available.") {
		t.Fatalf("expected update availability in output, got: %s", out.String())
	}
}

func TestUpdateCheckUsesSemverComparison(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	prev := fetchLatestVersionFn
	fetchLatestVersionFn = func() (string, error) { return "v0.1.0", nil }
	defer func() { fetchLatestVersionFn = prev }()

	if err := exec(t, root, "update", "--check"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Up to date.") {
		t.Fatalf("expected up-to-date output, got: %s", out.String())
	}
}

func TestUpdateCheckJSONModeReturnsLatestReleasePayload(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	prev := fetchLatestVersionFn
	fetchLatestVersionFn = func() (string, error) { return "v9.9.9", nil }
	defer func() { fetchLatestVersionFn = prev }()

	if err := exec(t, root, "update", "--check", "--json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"latest_version": "9.9.9"`) {
		t.Fatalf("expected latest_version in json output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), `"update_available": true`) {
		t.Fatalf("expected update_available in json output, got: %s", out.String())
	}
	if strings.Contains(out.String(), "Checking latest release...") {
		t.Fatalf("expected no human prelude in json mode, got: %s", out.String())
	}
}

func TestUpdateDefaultWhenAlreadyCurrentIsHumanReadable(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	prevFetch := fetchLatestVersionFn
	prevSync := syncInstalledSkillFn
	called := 0
	fetchLatestVersionFn = func() (string, error) { return "v" + version.Current, nil }
	syncInstalledSkillFn = func() (string, error) {
		called++
		return "/usr/local/bin/cafaye", nil
	}
	defer func() { fetchLatestVersionFn = prevFetch }()
	defer func() { syncInstalledSkillFn = prevSync }()

	if err := exec(t, root, "update"); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("expected default skill sync once, got %d", called)
	}
	if !strings.Contains(out.String(), "Already up to date.") {
		t.Fatalf("expected already-up-to-date message, got: %s", out.String())
	}
}

func TestUpdateDefaultUsesBrewWithHumanOutput(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	prevFetch := fetchLatestVersionFn
	prevDetect := detectBrewInstallFn
	prevBrew := runBrewUpgradeFn
	prevSync := syncInstalledSkillFn
	called := 0
	fetchLatestVersionFn = func() (string, error) { return "v9.9.9", nil }
	detectBrewInstallFn = func() bool { return true }
	runBrewUpgradeFn = func() error { return nil }
	syncInstalledSkillFn = func() (string, error) {
		called++
		return "/usr/local/bin/cafaye", nil
	}
	defer func() {
		fetchLatestVersionFn = prevFetch
		detectBrewInstallFn = prevDetect
		runBrewUpgradeFn = prevBrew
		syncInstalledSkillFn = prevSync
	}()

	if err := exec(t, root, "update"); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("expected default skill sync once, got %d", called)
	}
	if !strings.Contains(out.String(), "Updating via Homebrew...") {
		t.Fatalf("expected brew update message, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "Update complete: v9.9.9") {
		t.Fatalf("expected completion message, got: %s", out.String())
	}
}

func TestVersionCommand(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "version"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("expected version output")
	}
}

func TestVersionFlagAlias(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "--version"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("expected version output")
	}
}

func TestAgentsTokenCreateStoresSessionAfterVerification(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/key" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer oldtok" {
			t.Fatalf("expected old session token to mint new key, got: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"api_key":{"id":8,"name":"cli-issued","scopes":["books:write"]},"token":"newtok"}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	if err := rt.SaveConfig(config.File{
		ActiveAgentSession: "noel-agent-local",
		AgentSessions: map[string]config.AgentSession{
			"noel-agent-local": {
				Name:          "noel-agent-local",
				BaseURL:       s.URL,
				AgentUsername: "noel-agent",
				TokenRef:      "agent_session:noel-agent-local",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Secrets.Set("agent_session:noel-agent-local", "oldtok"); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmdWithRuntime(rt)
	err := exec(t, root, "agents", "token", "create", "--base-url", s.URL, "--agent", "noel-agent")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "token_created: true") {
		t.Fatalf("expected token create output, got: %s", out.String())
	}
	got, err := rt.Secrets.Get("agent_session:noel-agent-local")
	if err != nil {
		t.Fatal(err)
	}
	if got != "newtok" {
		t.Fatalf("expected minted token to be stored, got: %q", got)
	}
}

func TestAgentsLoginSwitchesExistingContextByAgentUsername(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	cfg := config.File{
		ActiveAgentSession: "p1",
		AgentSessions: map[string]config.AgentSession{
			"p1": {Name: "p1", AgentUsername: "agent-a", BaseURL: "x", TokenRef: "agent_session:p1"},
			"p2": {Name: "p2", AgentUsername: "agent-b", BaseURL: "x", TokenRef: "agent_session:p2"},
		},
	}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "login", "--agent", "agent-b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "active_agent_session: p2") {
		t.Fatalf("expected active agent session output, got: %s", out.String())
	}
}

func TestAgentsLoginSwitchRequiresAdditionalSelectorWhenMultipleAgentSessionsMatch(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	cfg := config.File{
		ActiveAgentSession: "p1",
		AgentSessions: map[string]config.AgentSession{
			"p1": {Name: "p1", AgentUsername: "agent-a", BaseURL: "https://prod.example.com", TokenRef: "agent_session:p1"},
			"p2": {Name: "p2", AgentUsername: "agent-a", BaseURL: "https://staging.example.com", TokenRef: "agent_session:p2"},
		},
	}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmdWithRuntime(rt)
	err := exec(t, root, "agents", "login", "--agent", "agent-a")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "multiple agent sessions match") {
		t.Fatalf("expected ambiguity guidance, got: %v", err)
	}
}

func TestAgentsLoginCanSelectAgentSessionByAgentAndBaseURL(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	cfg := config.File{
		ActiveAgentSession: "p1",
		AgentSessions: map[string]config.AgentSession{
			"p1": {Name: "p1", AgentUsername: "agent-a", BaseURL: "https://prod.example.com", TokenRef: "agent_session:p1"},
			"p2": {Name: "p2", AgentUsername: "agent-a", BaseURL: "https://staging.example.com", TokenRef: "agent_session:p2"},
		},
	}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "login", "--agent", "agent-a", "--base-url", "https://staging.example.com"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "active_agent_session: p2") {
		t.Fatalf("expected agent session switch output, got: %s", out.String())
	}
}

func TestAgentsListFallsBackToLocalAgentSessionsForUnclaimedAgent(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agents":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"agent_unclaimed"}`))
		case "/agents/home":
			_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"agent"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"agent_sessions"`) {
		t.Fatalf("expected agent_sessions in output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), `"remote_error"`) {
		t.Fatalf("expected remote_error in output, got: %s", out.String())
	}
}

func TestAgentsRegisterCreatesProfileByDefault(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("expected no auth header for register, got: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"agent-abc","name":"Agent ABC","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new","scopes":["books:write"]},"claim":{"url":"http://localhost/claims/x","message":"Have a human owner open this URL, sign in, and complete claim before publishing."}}`))
	}))
	defer s.Close()

	rt, out, errOut, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "Agent ABC"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"claim"`) {
		t.Fatalf("expected claim object in output, got: %s", out.String())
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveAgentSession != "agent-abc-127-0-0-1" {
		t.Fatalf("expected active agent session agent-abc-127-0-0-1, got: %s", cfg.ActiveAgentSession)
	}
	session := cfg.AgentSessions["agent-abc-127-0-0-1"]
	if session.AgentUsername != "agent-abc" {
		t.Fatalf("expected agent username to be saved, got: %+v", session)
	}
	token, err := rt.Secrets.Get("agent_session:agent-abc-127-0-0-1")
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok_new" {
		t.Fatalf("expected saved token tok_new, got: %s", token)
	}
	if !strings.Contains(errOut.String(), "claim_required:") {
		t.Fatalf("expected claim guidance in stderr, got: %s", errOut.String())
	}
}

func TestAgentsRegisterNoSaveDoesNotPersistAgentSession(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"agent-abc"},"api_key":{"id":2,"token":"tok_new"}}`))
	}))
	defer s.Close()

	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "Agent ABC", "--no-save"); err != nil {
		t.Fatal(err)
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveAgentSession != "" || len(cfg.AgentSessions) != 0 {
		t.Fatalf("expected no saved agent session, got: %+v", cfg)
	}
}

func TestAgentsRegisterPassesOptionalNameAndUsername(t *testing.T) {
	var gotName, gotUsername string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotName, _ = body["name"].(string)
		gotUsername, _ = body["username"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"noel","name":"Noel","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new","scopes":["books:write"]},"claim":{"url":"http://localhost/claims/x","message":"Have a human owner open this URL, sign in, and complete claim before publishing."}}`))
	}))
	defer s.Close()

	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "Noel", "--username", "noel", "--no-save"); err != nil {
		t.Fatal(err)
	}

	if gotName != "Noel" {
		t.Fatalf("expected name Noel, got: %q", gotName)
	}
	if gotUsername != "noel" {
		t.Fatalf("expected username noel, got: %q", gotUsername)
	}
}

func TestAgentsRegisterPromptsForNameWhenMissing(t *testing.T) {
	var gotName, gotUsername string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotName, _ = body["name"].(string)
		gotUsername, _ = body["username"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"noel-agent-ab12","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new"}}`))
	}))
	defer s.Close()

	rt, _, errOut, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	root.SetIn(strings.NewReader("Noel Agent\n"))
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--no-save"); err != nil {
		t.Fatal(err)
	}

	if gotName != "Noel Agent" {
		t.Fatalf("expected prompted name to be sent, got: %q", gotName)
	}
	if !strings.HasPrefix(gotUsername, "noel-agent-") {
		t.Fatalf("expected autogenerated username prefix, got: %q", gotUsername)
	}
	if strings.ToLower(gotUsername) != gotUsername {
		t.Fatalf("expected lowercase autogenerated username, got: %q", gotUsername)
	}
	if !strings.Contains(errOut.String(), "Agent display name:") {
		t.Fatalf("expected prompt output, got: %q", errOut.String())
	}
}

func TestAgentsRegisterMissingNameAndNoInputFails(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	root.SetIn(strings.NewReader(""))

	err := exec(t, root, "agents", "register", "--base-url", "https://cafaye.example.com", "--no-save")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing --name") {
		t.Fatalf("expected missing name error, got: %v", err)
	}
}

func TestAgentsRegisterDefaultsBaseURLWhenMissing(t *testing.T) {
	var gotHost string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"noel-agent-ab12","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new"}}`))
	}))
	defer s.Close()

	prev := defaultRegisterBaseURL
	defaultRegisterBaseURL = s.URL
	defer func() { defaultRegisterBaseURL = prev }()

	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--name", "Noel Agent", "--no-save"); err != nil {
		t.Fatal(err)
	}
	if gotHost == "" {
		t.Fatal("expected request to default base URL server")
	}
}

func TestAgentsRegisterRetriesAutogeneratedUsernameOnConflict(t *testing.T) {
	var requests []string
	callCount := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		username, _ := body["username"].(string)
		requests = append(requests, username)
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"invalid_agent","details":["Username has already been taken"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"noel-agent-xy99","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new"}}`))
	}))
	defer s.Close()

	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "Noel Agent", "--no-save"); err != nil {
		t.Fatal(err)
	}

	if len(requests) < 2 {
		t.Fatalf("expected retry on username conflict, got %d attempts", len(requests))
	}
	if requests[0] == requests[1] {
		t.Fatalf("expected second autogenerated username to differ, got: %q then %q", requests[0], requests[1])
	}
}

func TestAgentsRegisterOpenClaimURLUsesOpener(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"agent-abc","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new"},"claim":{"url":"http://localhost/claims/x"}}`))
	}))
	defer s.Close()

	var opened string
	prevOpen := openURLFn
	openURLFn = func(url string) error {
		opened = url
		return nil
	}
	defer func() { openURLFn = prevOpen }()

	rt, _, errOut, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "Noel", "--open-claim-url", "--no-save"); err != nil {
		t.Fatal(err)
	}
	if opened != "http://localhost/claims/x" {
		t.Fatalf("expected claim url opener to be called, got: %q", opened)
	}
	if !strings.Contains(errOut.String(), "claim_url_opened: true") {
		t.Fatalf("expected claim open summary, got: %s", errOut.String())
	}
}

func TestAgentsRegisterDoesNotSwitchActiveWhenAlreadyLoggedIn(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agents/home":
			_, _ = w.Write([]byte(`{"agent":{"username":"existing-agent"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents":
			_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"new-agent","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s.Close()

	rt, _, errOut, _ := testRuntime(t)
	seedAgentSession(t, rt, "existing", s.URL, "tok_existing")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "New Agent"); err != nil {
		t.Fatal(err)
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveAgentSession != "existing" {
		t.Fatalf("expected active agent session to remain existing, got: %s", cfg.ActiveAgentSession)
	}
	if _, ok := cfg.AgentSessions["new-agent-127-0-0-1"]; !ok {
		t.Fatalf("expected new agent session for new-agent, got: %+v", cfg.AgentSessions)
	}
	if !strings.Contains(errOut.String(), "logged_in: false") {
		t.Fatalf("expected non-login summary, got: %s", errOut.String())
	}
}

func TestAgentsRegisterSwitchesActiveWhenCurrentAgentSessionUnauthorized(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agents/home":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents":
			_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"new-agent","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s.Close()

	rt, _, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "existing", s.URL, "tok_existing")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "New Agent"); err != nil {
		t.Fatal(err)
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveAgentSession != "new-agent-127-0-0-1" {
		t.Fatalf("expected active agent session to switch to new-agent-127-0-0-1, got: %s", cfg.ActiveAgentSession)
	}
}

func TestAgentsRegisterLogInFlagSwitchesEvenWhenAlreadyLoggedIn(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agents/home":
			_, _ = w.Write([]byte(`{"agent":{"username":"existing-agent"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents":
			_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"new-agent","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s.Close()

	rt, _, errOut, _ := testRuntime(t)
	seedAgentSession(t, rt, "existing", s.URL, "tok_existing")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "New Agent", "--log-in"); err != nil {
		t.Fatal(err)
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveAgentSession != "new-agent-127-0-0-1" {
		t.Fatalf("expected active agent session to switch to new-agent-127-0-0-1, got: %s", cfg.ActiveAgentSession)
	}
	if !strings.Contains(errOut.String(), "logged_in: true") {
		t.Fatalf("expected login summary, got: %s", errOut.String())
	}
}

func TestTokenRotateUpdatesStoredSecret(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/key/rotate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Idempotency-Key") == "" {
			t.Fatal("expected idempotency key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"new-token","api_key":{"id":1}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "old-token")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "token", "rotate"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "token_rotated: true") {
		t.Fatalf("expected rotate output, got: %s", out.String())
	}
	got, err := rt.Secrets.Get("agent_session:p1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "new-token" {
		t.Fatalf("expected token to rotate, got: %s", got)
	}
}

func TestTokenRevokeRequiresYes(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	err := exec(t, root, "agents", "token", "revoke")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "refusing revoke without --yes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadShow(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/uploads/up_7" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload":{"id":7,"status":"applied"}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "books", "upload", "show", "--upload-ref", "up_7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status": "applied"`) {
		t.Fatalf("expected upload payload, got: %s", out.String())
	}
}

func TestBooksPricing(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/books/smoke-book/pricing" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Idempotency-Key") == "" {
			t.Fatal("expected idempotency key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"book":{"id":42,"pricing_type":"paid","price_cents":1200}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "books", "pricing", "--book-slug", "smoke-book", "--pricing-type", "paid", "--price-cents", "1200"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"pricing_type": "paid"`) {
		t.Fatalf("expected pricing payload, got: %s", out.String())
	}
}

func TestBooksPublishAndUnpublish(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/books/smoke-book/publish":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for publish: %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"published":true},"published_revision_id":7}`))
		case "/api/books/smoke-book/unpublish":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for unpublish: %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"published":false},"published_revision_id":null}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "publish", "--book-slug", "smoke-book", "--revision-number", "7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"published": true`) {
		t.Fatalf("expected publish payload, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "books", "unpublish", "--book-slug", "smoke-book"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"published": false`) {
		t.Fatalf("expected unpublish payload, got: %s", out.String())
	}
}

func TestAgentsClaim(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents/claim" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":11,"status":"unclaimed"},"claim":{"url":"http://localhost/claims/token","message":"Have a human owner open this URL, sign in, and complete claim before publishing."}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "claim-link", "refresh"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"claim"`) {
		t.Fatalf("expected claim payload, got: %s", out.String())
	}
}

func TestTokenShow(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/key" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"api_key":{"id":1,"scopes":["books:write"]}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "token", "show"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"api_key"`) {
		t.Fatalf("expected key payload, got: %s", out.String())
	}
}

func TestBooksReadCommands(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/books/smoke-book/revisions":
			_, _ = w.Write([]byte(`{"revisions":[{"id":7}]}`))
		case "/api/books/book_abc123/revisions":
			_, _ = w.Write([]byte(`{"revisions":[{"id":8}]}`))
		case "/api/books/smoke-book/revisions/7":
			_, _ = w.Write([]byte(`{"revision":{"id":7}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "revisions", "--book-slug", "smoke-book"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"revisions"`) {
		t.Fatalf("expected revisions payload, got: %s", out.String())
	}
	out.Reset()
	root = NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "revisions", "--book-ref", "book_abc123"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"revisions"`) {
		t.Fatalf("expected revisions payload for book-ref, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "books", "revision", "--book-slug", "smoke-book", "--revision-number", "7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"revision"`) {
		t.Fatalf("expected revision payload, got: %s", out.String())
	}
	out.Reset()

}

func TestBooksCreateUpdateAndCover(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/books":
			_, _ = w.Write([]byte(`{"book":{"id":42,"slug":"new","title":"New","author":"A"}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/books/new":
			_, _ = w.Write([]byte(`{"book":{"id":42,"title":"Updated"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/books/new/cover":
			_, _ = w.Write([]byte(`{"book":{"id":42,"cover_attached":true}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedAgentSession(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "create", "--title", "New"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"title": "New"`) {
		t.Fatalf("expected create payload, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "books", "update", "--book-slug", "new", "--title", "Updated"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"title": "Updated"`) {
		t.Fatalf("expected update payload, got: %s", out.String())
	}
	out.Reset()

	tmp := filepath.Join(t.TempDir(), "cover.webp")
	if err := os.WriteFile(tmp, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "cover", "--book-slug", "new", "--file", tmp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"cover_attached": true`) {
		t.Fatalf("expected cover payload, got: %s", out.String())
	}
}

func TestAgentWorkflowSmoke(t *testing.T) {
	type state struct {
		claimed bool
	}
	st := &state{}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		auth := r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents":
			if auth != "" {
				t.Fatalf("register should be unauthenticated, got: %q", auth)
			}
			_, _ = w.Write([]byte(`{"agent":{"id":11,"username":"smoke-agent","status":"unclaimed"},"api_key":{"id":3,"token":"tok_smoke","scopes":["books:write","books:publish"]},"claim":{"url":"http://localhost/claims/tok","message":"Have a human owner open this URL, sign in, and complete claim before publishing."}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents/claim":
			if auth != "Bearer tok_smoke" {
				t.Fatalf("unexpected auth for claim: %q", auth)
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("claim missing idempotency key")
			}
			st.claimed = true
			_, _ = w.Write([]byte(`{"agent":{"id":11,"status":"claimed"},"claim":{"url":"http://localhost/claims/tok2","message":"Have a human owner open this URL, sign in, and complete claim before publishing."}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/books":
			if auth != "Bearer tok_smoke" || !st.claimed {
				t.Fatalf("create requires claimed token")
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"slug":"smoke-book","title":"Smoke Book","author":"Agent"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/uploads":
			if auth != "Bearer tok_smoke" || !st.claimed {
				t.Fatalf("upload requires claimed token")
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("upload missing idempotency key")
			}
			_, _ = w.Write([]byte(`{"upload":{"id":9,"book_id":42,"status":"applied"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/uploads/up_9":
			_, _ = w.Write([]byte(`{"upload":{"id":9,"status":"applied","book_id":42}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/books/smoke-book/revisions":
			_, _ = w.Write([]byte(`{"revisions":[{"id":7}],"current_draft_revision_id":7,"published_revision_id":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/books/smoke-book/publish":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("publish missing idempotency key")
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"published":true},"published_revision_id":7}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/books/smoke-book/unpublish":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("unpublish missing idempotency key")
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"published":false},"published_revision_id":null}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "Smoke Agent"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "agents", "claim-link", "refresh", "--idempotency-key", "run-claim-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "create", "--title", "Smoke Book", "--idempotency-key", "run-book-create"); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(zipPath, []byte("zip-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "upload", "--file", zipPath, "--idempotency-key", "run-upload-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "upload", "show", "--upload-ref", "up_9"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "revisions", "--book-slug", "smoke-book"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "publish", "--book-slug", "smoke-book", "--revision-number", "7", "--idempotency-key", "run-publish-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "unpublish", "--book-slug", "smoke-book", "--idempotency-key", "run-unpublish-smoke"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), `"published": true`) || !strings.Contains(out.String(), `"published": false`) {
		t.Fatalf("expected publish and unpublish output in smoke run, got: %s", out.String())
	}
}
