package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/forgejo"
)

func newPRCommentCmd() *cobra.Command {
	var body, bodyFile string
	var editLast bool
	cmd := &cobra.Command{
		Use:   "comment [<number> | <url> | <branch>]",
		Short: "Add a comment to a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
				return errors.New("specify only one of `--body` or `--body-file`")
			}
			text := body
			if cmd.Flags().Changed("body-file") {
				var data []byte
				var err error
				if bodyFile == "-" {
					data, err = io.ReadAll(cmd.InOrStdin())
				} else {
					data, err = os.ReadFile(bodyFile)
				}
				if err != nil {
					return err
				}
				text = string(data)
			} else if !cmd.Flags().Changed("body") {
				return errors.New("flag `--body` or `--body-file` required when not running interactively")
			}
			client, repository, number, err := prTarget(cmd, args)
			if err != nil {
				return err
			}
			ctx, owner, repo := cmd.Context(), repository.Owner, repository.Repo
			var comment *forgejo.Comment
			if editLast {
				me, err := client.Users.Me(ctx)
				if err != nil {
					return wrapAuth(err)
				}
				comments, err := client.Comments.List(ctx, owner, repo, number)
				if err != nil {
					return wrapAuth(err)
				}
				var last *forgejo.Comment
				for i := range comments {
					if comments[i].User != nil && comments[i].User.Login == me.Login {
						last = &comments[i]
					}
				}
				if last == nil {
					return errors.New("no comments found for current user")
				}
				comment, err = client.Comments.Edit(ctx, owner, repo, last.ID, text)
				if err != nil {
					return wrapAuth(err)
				}
			} else if comment, err = client.Comments.Create(ctx, owner, repo, number, text); err != nil {
				return wrapAuth(err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), comment.HTMLURL)
			return err
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "The comment body `text`")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().BoolVar(&editLast, "edit-last", false, "Edit the last comment of the current user")
	return cmd
}
