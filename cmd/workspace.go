package cmd

import (
	"strings"

	"github.com/cafaye/cafaye-cli/internal/skills"
)

func resolveWorkspaceRoot(booksDir string) (string, error) {
	if strings.TrimSpace(booksDir) != "" {
		return booksDir, nil
	}
	return skills.DefaultBooksDir()
}
