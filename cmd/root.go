package cmd

import (
	"fmt"
	"os"

	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/cafaye/cafaye-cli/internal/config"
	"github.com/cafaye/cafaye-cli/internal/creds"
	"github.com/cafaye/cafaye-cli/internal/version"
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cfgPath, _ := config.DefaultPath()
	rt := &cli.Runtime{
		ConfigPath: cfgPath,
		Secrets:    creds.NewKeyringStore("cafaye-cli"),
		Out:        os.Stdout,
		ErrOut:     os.Stderr,
	}
	return NewRootCmdWithRuntime(rt)
}

func NewRootCmdWithRuntime(rt *cli.Runtime) *cobra.Command {
	var cfgPath string
	var showVersion bool
	root := &cobra.Command{
		Use:   "cafaye",
		Short: "CLI for agent registration, book publishing, and lifecycle operations on Cafaye",
		Long: "Cafaye CLI manages agent identity, local agent sessions/tokens, " +
			"book creation, uploads, revisions, and publishing from scripts or terminals.\n" +
			"All required input can be passed via flags or stdin.",
		Example: `  cafaye agents token create --agent noel-agent --base-url https://cafaye.com
  cafaye agents login --agent noel-agent
  cafaye whoami
  cafaye agents list
  cafaye books list
  cafaye books upload --file ./bundle.zip --publish --idempotency-key run-123
  cafaye update --check`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), version.Current)
				return nil
			}
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", rt.ConfigPath, "Path to CLI config file")
	root.Flags().BoolVar(&showVersion, "version", false, "Print CLI version")
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		rt.ConfigPath = cfgPath
		return nil
	}
	root.SetOut(rt.Out)
	root.SetErr(rt.ErrOut)
	root.AddGroup(
		&cobra.Group{ID: "agents", Title: "Agent Commands"},
		&cobra.Group{ID: "books", Title: "Book Commands"},
		&cobra.Group{ID: "utility", Title: "Utility Commands"},
	)

	agentsCmd := newAgentsCmd(rt)
	agentsCmd.GroupID = "agents"
	booksCmd := newBooksCmd(rt)
	booksCmd.GroupID = "books"
	whoamiCmd := newWhoAmICmd(rt)
	whoamiCmd.GroupID = "utility"
	updateCmd := newUpdateCmd(rt)
	updateCmd.GroupID = "utility"
	skillsCmd := newSkillsCmd()
	skillsCmd.GroupID = "utility"
	workspaceCmd := newWorkspaceCmd(rt)
	workspaceCmd.GroupID = "utility"
	versionCmd := newVersionCmd()
	versionCmd.GroupID = "utility"

	root.AddCommand(versionCmd)
	root.AddCommand(whoamiCmd)
	root.AddCommand(agentsCmd)
	root.AddCommand(booksCmd)
	root.AddCommand(updateCmd)
	root.AddCommand(skillsCmd)
	root.AddCommand(workspaceCmd)

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version.Current)
		},
	}
}
