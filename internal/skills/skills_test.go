package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cafaye/cafaye-cli/internal/version"
)

func TestInstallForRootCreatesManagedSkill(t *testing.T) {
	root := t.TempDir()
	res, err := InstallForRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated {
		t.Fatal("expected first install to update target")
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatalf("expected skill file to exist: %v", err)
	}
	body, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "managed-by: cafaye-cli") {
		t.Fatalf("expected managed header, got: %q", s)
	}
	if !strings.Contains(s, "cli_version: "+version.Current) {
		t.Fatalf("expected version header, got: %q", s)
	}
}

func TestInstallForRootIsIdempotent(t *testing.T) {
	root := t.TempDir()
	first, err := InstallForRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := InstallForRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Updated {
		t.Fatal("expected first install to update")
	}
	if second.Updated {
		t.Fatal("expected second install to be no-op")
	}
}

func TestEnsureDefaultInstalledUsesEnvOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv(booksDirEnv, root)
	res, err := EnsureDefaultInstalled()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, skillRelativePath)
	if res.Path != want {
		t.Fatalf("expected %s, got %s", want, res.Path)
	}
}

func TestDefaultBooksDirFallsBackToHome(t *testing.T) {
	t.Setenv(booksDirEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultBooksDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != home {
		t.Fatalf("expected default dir %s, got %s", home, got)
	}
}

func TestInstallForRootReplacesOutdatedSkill(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, skillRelativePath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old-skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := InstallForRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated {
		t.Fatal("expected outdated skill to be replaced")
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "old-skill") {
		t.Fatalf("expected updated content, got: %q", string(body))
	}
}
