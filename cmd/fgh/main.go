// Command fgh is gh's command interface over the Forgejo API.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if err := newRootCmd(version).Execute(); err != nil {
		os.Exit(exitCode(err))
	}
}

func newRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "fgh <command> <subcommand> [flags]",
		Short:        "gh's command interface for Forgejo",
		Version:      version,
		SilenceUsage: true,
		RunE:         groupRunE,
	}
	cmd.PersistentFlags().StringP("repo", "R", "", "Select repository owner/repo")
	cmd.AddCommand(newAuthCmd(), newAPICmd(), newPRCmd(), newRepoCmd(), newSearchCmd())
	cmd.SetVersionTemplate("fgh version {{.Version}}\n")
	return cmd
}

// groupRunE is the RunE of every command that only holds subcommands. Without one, cobra
// prints the help and exits 0 for any argument: a command fgh does not port (`pr checks`)
// must fail like gh's unknown command, or a script would read it as a success.
func groupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	return cmd.Help()
}
