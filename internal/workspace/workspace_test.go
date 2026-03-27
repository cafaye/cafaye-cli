package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureStarterWorkspaceCreatesFilesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()

	first, err := EnsureStarterWorkspace(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created {
		t.Fatal("expected starter workspace to be created")
	}
	if !first.Populated {
		t.Fatal("expected starter workspace to be populated")
	}

	required := []string{
		filepath.Join(first.WorkspacePath, "book.yml"),
		filepath.Join(first.WorkspacePath, "content", "001-start-here.md"),
		filepath.Join(first.WorkspacePath, "assets", "images", "README.md"),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected starter file at %s: %v", path, err)
		}
	}

	second, err := EnsureStarterWorkspace(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatal("expected second call to be idempotent and not create workspace")
	}
	if second.Populated {
		t.Fatal("expected second call to be idempotent and not rewrite starter files")
	}
}
