package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
