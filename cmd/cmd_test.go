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

func TestProfileAddUseList(t *testing.T) {
	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	if err := exec(t, root, "profile", "add", "--name", "p1", "--base-url", "https://x", "--agent", "a1", "--token", "tok"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "profile_saved: p1") {
		t.Fatalf("expected profile_saved output, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "profile", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "p1 (active)") {
		t.Fatalf("expected active profile, got: %s", out.String())
	}
}

func TestProfileAddMissingFlagsActionableError(t *testing.T) {
	rt, _, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)

	err := exec(t, root, "profile", "add", "--name", "p1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cafaye profile add --name <name>") {
		t.Fatalf("expected actionable invocation, got: %v", err)
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
	if !strings.Contains(out.String(), "cafaye profile add") {
		t.Fatalf("expected examples in help, got: %s", out.String())
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

func TestLoginStoresProfileAfterVerification(t *testing.T) {
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
	err := exec(t, root, "login", "--name", "p1", "--base-url", s.URL, "--agent", "noel-agent", "--token", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "login_ok: p1") {
		t.Fatalf("expected login output, got: %s", out.String())
	}
}

func TestAgentsUseSetsProfileByAgentUsername(t *testing.T) {
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
	if err := exec(t, root, "agents", "use", "--agent", "agent-b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "active_profile: p2") {
		t.Fatalf("expected active profile output, got: %s", out.String())
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
		_, _ = w.Write([]byte(`{"agent":{"id":1,"username":"agent-abc","name":"Agent ABC","status":"unclaimed"},"api_key":{"id":2,"token":"tok_new","scopes":["books:write"]},"claim_url":"http://localhost/claims/x"}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "register", "--base-url", s.URL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"claim_url"`) {
		t.Fatalf("expected claim_url in output, got: %s", out.String())
	}

	cfg, err := rt.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != "agent-abc" {
		t.Fatalf("expected active profile agent-abc, got: %s", cfg.ActiveProfile)
	}
	p := cfg.Profiles["agent-abc"]
	if p.AgentUsername != "agent-abc" {
		t.Fatalf("expected agent username to be saved, got: %+v", p)
	}
	token, err := rt.Secrets.Get("profile:agent-abc")
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok_new" {
		t.Fatalf("expected saved token tok_new, got: %s", token)
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
	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--no-save"); err != nil {
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
		_, _ = w.Write([]byte(`{"agent":{"id":11,"status":"unclaimed"},"claim_url":"http://localhost/claims/token"}`))
	}))
	defer s.Close()

	rt, out, _, _ := testRuntime(t)
	seedProfile(t, rt, "p1", s.URL, "tok")
	root := NewRootCmdWithRuntime(rt)
	if err := exec(t, root, "agents", "claim", "--agent-id", "11"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"claim_url"`) {
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
		case "/api/books/42/source":
			_, _ = w.Write([]byte(`{"source":{"upload_id":2}}`))
		case "/api/books/42/revisions/7/source":
			_, _ = w.Write([]byte(`{"source":{"upload_id":2}}`))
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

	if err := exec(t, root, "books", "source", "--book-id", "42"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"source"`) {
		t.Fatalf("expected source payload, got: %s", out.String())
	}
	out.Reset()

	if err := exec(t, root, "books", "revision-source", "--book-id", "42", "--revision-id", "7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"source"`) {
		t.Fatalf("expected revision source payload, got: %s", out.String())
	}
}

func TestBooksCreateUpdateAndCover(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/books":
			_, _ = w.Write([]byte(`{"book":{"id":42,"title":"New"}}`))
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

	if err := exec(t, root, "books", "create", "--title", "New", "--author", "A"); err != nil {
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
			_, _ = w.Write([]byte(`{"agent":{"id":11,"username":"smoke-agent","status":"unclaimed"},"api_key":{"id":3,"token":"tok_smoke","scopes":["books:write","books:publish"]},"claim_url":"http://localhost/claims/tok"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents/11/claim":
			if auth != "Bearer tok_smoke" {
				t.Fatalf("unexpected auth for claim: %q", auth)
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("claim missing idempotency key")
			}
			st.claimed = true
			_, _ = w.Write([]byte(`{"agent":{"id":11,"status":"claimed"},"claim_url":"http://localhost/claims/tok2"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/books":
			if auth != "Bearer tok_smoke" || !st.claimed {
				t.Fatalf("create requires claimed token")
			}
			_, _ = w.Write([]byte(`{"book":{"id":42,"title":"Smoke Book"}}`))
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

	if err := exec(t, root, "agents", "register", "--base-url", s.URL, "--profile-name", "smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "agents", "claim", "--agent-id", "11", "--idempotency-key", "run-claim-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, root, "books", "create", "--title", "Smoke Book", "--author", "Agent", "--idempotency-key", "run-book-create"); err != nil {
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
