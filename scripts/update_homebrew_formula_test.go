package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateHomebrewFormulaScriptUpdatesURLAndSHA(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	formulaPath := filepath.Join(tmp, "cafaye.rb")
	if err := os.WriteFile(formulaPath, []byte(`class Cafaye < Formula
  url "https://github.com/cafaye/cafaye-cli/archive/refs/tags/v0.3.11.tar.gz"
  sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
end
`), 0o644); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	curlPath := filepath.Join(binDir, "curl")
	if err := os.WriteFile(curlPath, []byte("#!/usr/bin/env bash\necho mock-tarball\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	shasumPath := filepath.Join(binDir, "shasum")
	if err := os.WriteFile(shasumPath, []byte("#!/usr/bin/env bash\ncat >/dev/null\necho 1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd  -\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join("update-homebrew-formula.sh")
	cmd := exec.Command("bash", script, "v0.3.13")
	cmd.Dir = filepath.Join("..", "scripts")
	cmd.Env = append(os.Environ(),
		"FORMULA_PATH="+formulaPath,
		"PATH="+binDir+":"+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}

	updated, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(updated)
	if !strings.Contains(body, `url "https://github.com/cafaye/cafaye-cli/archive/refs/tags/v0.3.13.tar.gz"`) {
		t.Fatalf("expected formula url to be updated, got:\n%s", body)
	}
	if !strings.Contains(body, `sha256 "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd"`) {
		t.Fatalf("expected formula sha to be updated, got:\n%s", body)
	}
}

func TestUpdateHomebrewFormulaScriptRejectsInvalidTag(t *testing.T) {
	t.Parallel()

	script := filepath.Join("update-homebrew-formula.sh")
	cmd := exec.Command("bash", script, "3.13")
	cmd.Dir = filepath.Join("..", "scripts")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected invalid tag to fail, output: %s", string(out))
	}
	if !strings.Contains(string(out), "invalid version tag") {
		t.Fatalf("expected invalid tag message, got: %s", string(out))
	}
}
