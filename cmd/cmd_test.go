package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/cafaye/cafaye-cli/internal/creds"
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

func seedProfile(t *testing.T, rt *cli.Runtime, name string, baseURL string, token string) {
	t.Helper()
	cfg := config.File{ActiveProfile: name, Profiles: map[string]config.Profile{
		name: {Name: name, BaseURL: baseURL, AgentUsername: "agent", TokenRef: "profile:" + name},
	}}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := rt.Secrets.Set("profile:"+name, token); err != nil {
		t.Fatal(err)
	}
}

func TestAgentsLoginWithTokenAndList(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agents/home":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"agent":{"username":"a1"}}`))
		case "/api/agents":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"agents":[{"id":1,"username":"a1"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()

	if err := exec(t, root, "agents", "login", "--agent", "a1", "--base-url", s.URL, "--token", "tok"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "login_ok: a1-127-0-0-1") {
		t.Fatalf("expected login output, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "agents", "list", "--profile", "a1-127-0-0-1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"contexts"`) {
		t.Fatalf("expected contexts in agents list, got: %s", out.String())
	}
}

func TestAgentsLoginWithoutTokenAndNoContextFails(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	err := exec(t, root, "agents", "login", "--agent", "a1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no saved context matches") {
		t.Fatalf("expected context selection error, got: %v", err)
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
	seedProfile(t, rt, "p1", s.URL, "tok")
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
	seedProfile(t, rt, "p1", s.URL, "tok")
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

func TestUpdateFallbackWhenEndpointUnavailable(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "update", "--check"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "update_endpoint: unavailable") {
		t.Fatalf("expected graceful fallback, got: %s", out.String())
	}
}

func TestUploadDryRun(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "upload", "--file", "bundle.zip", "--idempotency-key", "run-12345", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dry_run: true") {
		t.Fatalf("expected dry_run output, got: %s", out.String())
	}
}

func TestUploadRequiresIdempotencyKey(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	err := exec(t, root, "upload", "--file", "bundle.zip")
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
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	root.SetIn(strings.NewReader("zipbytes"))

	if err := exec(t, root, "upload", "--stdin", "--idempotency-key", "run-12345"); err != nil {
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
	seedProfile(t, rt, "p1", s.URL, "tok")
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
	seedProfile(t, rt, "p1", s.URL, "tok")
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

func TestUpdateReturnsServerPayload(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"latest": "0.2.0", "deprecated_commands": []string{"oldcmd"}})
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "update", "--check"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "deprecated_commands") {
		t.Fatalf("expected deprecation metadata in output, got: %s", out.String())
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

func TestAgentsLoginStoresContextAfterVerification(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agents/home" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"username":"noel-agent"}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	err := exec(t, root, "agents", "login", "--base-url", s.URL, "--agent", "noel-agent", "--token", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "login_ok: noel-agent-127-0-0-1") {
		t.Fatalf("expected login output, got: %s", out.String())
	}
}

func TestAgentsLoginSwitchesExistingContextByAgentUsername(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	cfg := config.File{
		ActiveProfile: "p1",
		Profiles: map[string]config.Profile{
			"p1": {Name: "p1", AgentUsername: "agent-a", BaseURL: "x", TokenRef: "profile:p1"},
			"p2": {Name: "p2", AgentUsername: "agent-b", BaseURL: "x", TokenRef: "profile:p2"},
		},
	}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "login", "--agent", "agent-b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "active_context: p2") {
		t.Fatalf("expected active context output, got: %s", out.String())
	}
}

func TestAgentsLoginSwitchRequiresAdditionalSelectorWhenMultipleContextsMatch(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	cfg := config.File{
		ActiveProfile: "p1",
		Profiles: map[string]config.Profile{
			"p1": {Name: "p1", AgentUsername: "agent-a", BaseURL: "https://prod.example.com", TokenRef: "profile:p1"},
			"p2": {Name: "p2", AgentUsername: "agent-a", BaseURL: "https://staging.example.com", TokenRef: "profile:p2"},
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
	if !strings.Contains(err.Error(), "multiple contexts match") {
		t.Fatalf("expected ambiguity guidance, got: %v", err)
	}
}

func TestAgentsLoginCanSelectContextByAgentAndBaseURL(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	cfg := config.File{
		ActiveProfile: "p1",
		Profiles: map[string]config.Profile{
			"p1": {Name: "p1", AgentUsername: "agent-a", BaseURL: "https://prod.example.com", TokenRef: "profile:p1"},
			"p2": {Name: "p2", AgentUsername: "agent-a", BaseURL: "https://staging.example.com", TokenRef: "profile:p2"},
		},
	}
	if err := rt.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "login", "--agent", "agent-a", "--base-url", "https://staging.example.com"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "active_context: p2") {
		t.Fatalf("expected context switch output, got: %s", out.String())
	}
}

func TestAgentsListFallsBackToLocalContextsForUnclaimedAgent(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agents":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"agent_unclaimed"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"contexts"`) {
		t.Fatalf("expected contexts in output, got: %s", out.String())
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
	if cfg.ActiveProfile != "agent-abc-profile" {
		t.Fatalf("expected active profile agent-abc-profile, got: %s", cfg.ActiveProfile)
	}
	p := cfg.Profiles["agent-abc-profile"]
	if p.AgentUsername != "agent-abc" {
		t.Fatalf("expected agent username to be saved, got: %+v", p)
	}
	token, err := rt.Secrets.Get("profile:agent-abc-profile")
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

func TestAgentsRegisterNoSaveDoesNotPersistProfile(t *testing.T) {
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
	if cfg.ActiveProfile != "" || len(cfg.Profiles) != 0 {
		t.Fatalf("expected no saved profile, got: %+v", cfg)
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
	seedProfile(t, rt, "existing", s.URL, "tok_existing")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "New Agent"); err != nil {
		t.Fatal(err)
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != "existing" {
		t.Fatalf("expected active profile to remain existing, got: %s", cfg.ActiveProfile)
	}
	if _, ok := cfg.Profiles["new-agent-profile"]; !ok {
		t.Fatalf("expected new profile for new-agent, got: %+v", cfg.Profiles)
	}
	if !strings.Contains(errOut.String(), "logged_in: false") {
		t.Fatalf("expected non-login summary, got: %s", errOut.String())
	}
}

func TestAgentsRegisterSwitchesActiveWhenCurrentProfileUnauthorized(t *testing.T) {
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
	seedProfile(t, rt, "existing", s.URL, "tok_existing")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "New Agent"); err != nil {
		t.Fatal(err)
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != "new-agent-profile" {
		t.Fatalf("expected active profile to switch to new-agent-profile, got: %s", cfg.ActiveProfile)
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
	seedProfile(t, rt, "existing", s.URL, "tok_existing")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "New Agent", "--log-in"); err != nil {
		t.Fatal(err)
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != "new-agent-profile" {
		t.Fatalf("expected active profile to switch to new-agent-profile, got: %s", cfg.ActiveProfile)
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
	seedProfile(t, rt, "p1", s.URL, "old-token")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "token", "rotate"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "token_rotated: true") {
		t.Fatalf("expected rotate output, got: %s", out.String())
	}
	got, err := rt.Secrets.Get("profile:p1")
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
	err := exec(t, root, "token", "revoke")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "refusing revoke without --yes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadShow(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/uploads/7" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload":{"id":7,"status":"applied"}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "upload", "show", "--id", "7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status": "applied"`) {
		t.Fatalf("expected upload payload, got: %s", out.String())
	}
}

func TestBooksPricing(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/books/42/pricing" {
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
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "books", "pricing", "--book-id", "42", "--pricing-type", "paid", "--price-cents", "1200"); err != nil {
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
		case "/api/books/42/publish":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for publish: %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"published":true},"published_revision_id":7}`))
		case "/api/books/42/unpublish":
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
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "publish", "--book-id", "42", "--revision-id", "7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"published": true`) {
		t.Fatalf("expected publish payload, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "books", "unpublish", "--book-id", "42"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"published": false`) {
		t.Fatalf("expected unpublish payload, got: %s", out.String())
	}
}

func TestAgentsClaim(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents/11/claim" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":11,"status":"unclaimed"},"claim":{"url":"http://localhost/claims/token","message":"Have a human owner open this URL, sign in, and complete claim before publishing."}}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "claim-link", "refresh", "--agent-id", "11"); err != nil {
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
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "token", "show"); err != nil {
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
		case "/api/books/42/revisions":
			_, _ = w.Write([]byte(`{"revisions":[{"id":7}]}`))
		case "/api/books/42/revisions/7":
			_, _ = w.Write([]byte(`{"revision":{"id":7}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "revisions", "--book-id", "42"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"revisions"`) {
		t.Fatalf("expected revisions payload, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "books", "revision", "--book-id", "42", "--revision-id", "7"); err != nil {
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
		case r.Method == http.MethodPatch && r.URL.Path == "/api/books/42":
			_, _ = w.Write([]byte(`{"book":{"id":42,"title":"Updated"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/books/42/cover":
			_, _ = w.Write([]byte(`{"book":{"id":42,"cover_attached":true}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "books", "create", "--title", "New"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"title": "New"`) {
		t.Fatalf("expected create payload, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "books", "update", "--book-id", "42", "--title", "Updated"); err != nil {
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
	if err := exec(t, root, "books", "cover", "--book-id", "42", "--file", tmp); err != nil {
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
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents/11/claim":
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/uploads/9":
			_, _ = w.Write([]byte(`{"upload":{"id":9,"status":"applied","book_id":42}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/books/42/revisions":
			_, _ = w.Write([]byte(`{"revisions":[{"id":7}],"current_draft_revision_id":7,"published_revision_id":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/books/42/publish":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("publish missing idempotency key")
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"published":true},"published_revision_id":7}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/books/42/unpublish":
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

	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--name", "Smoke Agent", "--profile-name", "smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "agents", "claim-link", "refresh", "--agent-id", "11", "--idempotency-key", "run-claim-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "create", "--title", "Smoke Book", "--idempotency-key", "run-book-create"); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(zipPath, []byte("zip-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "upload", "--file", zipPath, "--idempotency-key", "run-upload-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "upload", "show", "--id", "9"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "revisions", "--book-id", "42"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "publish", "--book-id", "42", "--revision-id", "7", "--idempotency-key", "run-publish-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "unpublish", "--book-id", "42", "--idempotency-key", "run-unpublish-smoke"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), `"published": true`) || !strings.Contains(out.String(), `"published": false`) {
		t.Fatalf("expected publish and unpublish output in smoke run, got: %s", out.String())
	}
}
