package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
)

func newPRMergeCmd() *cobra.Command {
	var squash, merge, rebase, deleteBranch, admin, auto bool
	var subject, body, head string
	cmd := &cobra.Command{
		Use:   "merge [<number> | <url> | <branch>]",
		Short: "Merge a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("admin") || cmd.Flags().Changed("auto") {
				return errors.New("--admin and --auto are not implemented")
			}
			method := ""
			for name, enabled := range map[string]bool{"squash": squash, "merge": merge, "rebase": rebase} {
				if enabled {
					if method != "" {
						return errors.New("only one merge method may be selected")
					}
					method = name
				}
			}
			if method == "" {
				return errors.New("one of --merge, --rebase, or --squash is required")
			}
			override, err := cmd.Flags().GetString("repo")
			if err != nil {
				return err
			}
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			repository, number, branch, err := resolvePRArg(cmd, arg, override)
			if err != nil {
				return err
			}
			credential, err := auth.Token(cmd.Context(), repository.Host)
			if err != nil {
				return &authError{err}
			}
			client := forgejo.New(repository.APIBase(), credential.Token)
			if number == 0 {
				number, err = findByBranch(cmd, client, repository, branch)
				if err != nil {
					return wrapAuth(err)
				}
			}
			opts := forgejo.MergeOptions{Method: method, HeadSHA: head, DeleteBranch: deleteBranch}
			if cmd.Flags().Changed("subject") {
				opts.Title = &subject
			}
			if cmd.Flags().Changed("body") {
				opts.Message = &body
			}
			noBranch, err := client.Pulls.Merge(cmd.Context(), repository.Owner, repository.Repo, number, opts)
			if err != nil {
				return wrapAuth(err)
			}
			if noBranch {
				_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Note: AGit pull requests have no branch to delete.")
			}
			return err
		},
	}
	cmd.Flags().BoolVarP(&squash, "squash", "s", false, "Squash the commits into one commit")
	cmd.Flags().BoolVarP(&merge, "merge", "m", false, "Merge the commits with a merge commit")
	cmd.Flags().BoolVarP(&rebase, "rebase", "r", false, "Rebase the commits onto the base branch")
	cmd.Flags().StringVarP(&subject, "subject", "t", "", "Subject for the merge commit")
	cmd.Flags().StringVarP(&body, "body", "b", "", "Body for the merge commit")
	cmd.Flags().StringVar(&head, "match-head-commit", "", "Commit SHA that the pull request head must match")
	cmd.Flags().BoolVarP(&deleteBranch, "delete-branch", "d", false, "Delete the branch after merge")
	cmd.Flags().BoolVar(&admin, "admin", false, "Not implemented")
	cmd.Flags().BoolVar(&auto, "auto", false, "Not implemented")
	return cmd
}
