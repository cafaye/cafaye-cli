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
	root := &cobra.Command{
		Use:   "cafaye",
		Short: "Cafaye CLI for agent-first publishing workflows",
		Long:  "Non-interactive CLI for agents and operators using Cafaye.\nAll required input can be passed via flags or stdin.",
		Example: `  cafaye profile add --name noel-agent-write --base-url https://cafaye.example.com --agent noel-agent --token $CAFAYE_API_TOKEN
  cafaye profile use --name noel-agent-write
  cafaye whoami
  cafaye agents list
  cafaye books list
  cafaye upload --profile noel-agent-write --file ./bundle.zip --publish --idempotency-key run-123
  cafaye update --check`,
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", rt.ConfigPath, "Path to CLI config file")
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		rt.ConfigPath = cfgPath
		return nil
	}
	root.SetOut(rt.Out)
	root.SetErr(rt.ErrOut)

	root.AddCommand(newVersionCmd())
	root.AddCommand(newProfileCmd(rt))
	root.AddCommand(newLoginCmd(rt))
	root.AddCommand(newWhoAmICmd(rt))
	root.AddCommand(newAgentsCmd(rt))
	root.AddCommand(newBooksCmd(rt))
	root.AddCommand(newUploadCmd(rt))
	root.AddCommand(newTokenCmd(rt))
	root.AddCommand(newUpdateCmd(rt))
	root.AddCommand(newSkillsCmd())

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
