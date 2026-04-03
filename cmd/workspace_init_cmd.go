package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cafaye/cafaye-cli/internal/cli"
	workspacepkg "github.com/cafaye/cafaye-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func newWorkspaceCmd(rt *cli.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Initialize and manage local writing workspaces",
	}
	cmd.AddCommand(newWorkspaceInitCmd(rt))
	return cmd
}

func newWorkspaceInitCmd(_ *cli.Runtime) *cobra.Command {
	var booksDir string
	var name string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create or refresh a starter source-bundle workspace",
		Example: `  cafaye workspace init
  cafaye workspace init --books-dir ~/Cafaye/books
  cafaye workspace init --books-dir /tmp/books --name starter-book`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveWorkspaceInitRoot(booksDir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				return err
			}
			workspaceName := resolveWorkspaceName(name)
			workspacePath := filepath.Join(root, workspaceName)
			if info, statErr := os.Stat(workspacePath); statErr == nil && info.IsDir() {
				result := map[string]any{
					"workspace_root":    root,
					"workspace_path":    workspacePath,
					"workspace_created": false,
					"starter_populated": false,
					"skipped":           true,
					"message":           "target workspace already exists; no files were changed",
					"notes": []string{
						"workspace init does not overwrite existing workspace directories",
						"Existing workspace left untouched",
					},
				}
				return printJSON(cmd.OutOrStdout(), result)
			}

			initRes, err := workspacepkg.EnsureStarterWorkspace(root, workspaceName)
			if err != nil {
				return err
			}

			result := map[string]any{
				"workspace_root":    root,
				"workspace_path":    initRes.WorkspacePath,
				"workspace_created": initRes.Created,
				"starter_populated": initRes.Populated,
				"message":           "starter workspace initialized successfully",
				"notes": []string{
					"Starter workspace includes book.yml, content/001-start-here.md, and assets/images/README.md",
					"workspace init does not install skills; install/update flows run skill sync separately",
				},
			}
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&booksDir, "books-dir", "", "Workspace root directory (defaults to ~/Cafaye/books)")
	cmd.Flags().StringVar(&name, "name", "", "Workspace folder name (defaults to starter-book)")
	_ = cmd.RegisterFlagCompletionFunc("name", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"starter-book"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(name) == "." || strings.TrimSpace(name) == ".." {
			return fmt.Errorf("invalid --name: %q", name)
		}
		return nil
	}
	return cmd
}

func resolveWorkspaceInitRoot(booksDir string) (string, error) {
	if strings.TrimSpace(booksDir) != "" {
		return booksDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Cafaye", "books"), nil
}

func resolveWorkspaceName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "starter-book"
	}
	return strings.TrimSpace(name)
}
