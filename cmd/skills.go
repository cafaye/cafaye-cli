package cmd

import (
	"fmt"

	"github.com/cafaye/cafaye-cli/internal/skills"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Install version-matched Cafaye agent skill files",
	}
	cmd.AddCommand(newSkillsInstallCmd())
	return cmd
}

func newSkillsInstallCmd() *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install/update Cafaye skill into a workspace or bundle root",
		Example: `  cafaye skills install
  cafaye skills install --root ~/Cafaye/books
  cafaye skills install --root /tmp/source-bundle`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var (
				res skills.InstallResult
				err error
			)
			if root == "" {
				res, err = skills.EnsureDefaultInstalled()
			} else {
				res, err = skills.InstallForRoot(root)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "skill_path: %s\n", res.Path)
			fmt.Fprintf(cmd.OutOrStdout(), "updated: %t\n", res.Updated)
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Workspace/source bundle root (defaults to CAFAYE_BOOKS_DIR or ~/Cafaye/books)")
	return cmd
}
