package main

import "github.com/spf13/cobra"

func newPRCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "pr <command>", Short: "Manage pull requests", RunE: groupRunE}
	cmd.AddCommand(newPRViewCmd(), newPRListCmd(), newPRDiffCmd(), newPRCommentCmd(), newPRCreateCmd(), newPRMergeCmd())
	return cmd
}
