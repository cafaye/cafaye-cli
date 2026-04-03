package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/skills"
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
		Short: "Create or refresh a starter workspace and install Cafaye skill files",
		Example: `  cafaye workspace init
  cafaye workspace init --books-dir ~/Cafaye/books
  cafaye workspace init --books-dir /tmp/books --name starter-book`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveWorkspaceRoot(booksDir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				return err
			}
			initRes, err := workspacepkg.EnsureStarterWorkspace(root, name)
			if err != nil {
				return err
			}

			defaultSkillRes, err := skills.InstallForRoot(root)
			if err != nil {
				return err
			}
			workspaceSkillRes, err := skills.InstallForRoot(initRes.WorkspacePath)
			if err != nil {
				return err
			}

			result := map[string]any{
				"workspace_root":          root,
				"workspace_path":          initRes.WorkspacePath,
				"workspace_created":       initRes.Created,
				"starter_populated":       initRes.Populated,
				"default_skill_path":      defaultSkillRes.Path,
				"default_skill_updated":   defaultSkillRes.Updated,
				"workspace_skill_path":    workspaceSkillRes.Path,
				"workspace_skill_updated": workspaceSkillRes.Updated,
				"notes": []string{
					"Starter workspace includes book.yml, content/001-start-here.md, and assets/images/README.md",
					"Default skill location is ~/.agents/skills/cafaye/SKILL.md unless CAFAYE_BOOKS_DIR is set",
				},
			}
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&booksDir, "books-dir", "", "Workspace root directory (defaults to CAFAYE_BOOKS_DIR or ~)")
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
