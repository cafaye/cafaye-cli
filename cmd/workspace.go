package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/cafaye/cafaye-cli/internal/skills"
	"github.com/spf13/cobra"
)

func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Workspace setup and initialization commands",
	}
	cmd.AddCommand(newWorkspaceInitCmd())
	return cmd
}

func newWorkspaceInitCmd() *cobra.Command {
	var booksDir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize default Cafaye workspace root and install bundled skill",
		Example: `  cafaye workspace init
  cafaye workspace init --books-dir ~/Work/CafayeBooks`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveWorkspaceRoot(booksDir)
			if err != nil {
				return err
			}

			created := false
			if _, err := os.Stat(root); os.IsNotExist(err) {
				created = true
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				return err
			}

			res, err := skills.InstallForRoot(root)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "workspace_root: %s\n", root)
			fmt.Fprintf(cmd.OutOrStdout(), "workspace_created: %t\n", created)
			fmt.Fprintf(cmd.OutOrStdout(), "skill_path: %s\n", res.Path)
			fmt.Fprintf(cmd.OutOrStdout(), "skill_updated: %t\n", res.Updated)
			return nil
		},
	}

	cmd.Flags().StringVar(&booksDir, "books-dir", "", "Workspace books directory (defaults to CAFAYE_BOOKS_DIR or ~/Cafaye/books)")
	return cmd
}

func resolveWorkspaceRoot(booksDir string) (string, error) {
	if strings.TrimSpace(booksDir) != "" {
		return booksDir, nil
	}
	return skills.DefaultBooksDir()
}
