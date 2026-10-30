package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/remote"
)

func newPRCreateCmd() *cobra.Command {
	var title, body, bodyFile, base, head string
	var assignees []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			if flags.Changed("body") && flags.Changed("body-file") {
				return errors.New("specify only one of `--body` or `--body-file`")
			}
			if title == "" || (!flags.Changed("body") && !flags.Changed("body-file")) {
				return errors.New("must provide `--title` and `--body` (or `--body-file`) when not running interactively")
			}
			if flags.Changed("body-file") {
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
				body = string(data)
			}
			override, err := flags.GetString("repo")
			if err != nil {
				return err
			}
			repository, err := remote.Resolve(cmd.Context(), "", override)
			if err != nil {
				return err
			}
			if head == "" {
				out, err := exec.CommandContext(cmd.Context(), "git", "symbolic-ref", "--short", "HEAD").Output()
				head = strings.TrimSpace(string(out))
				if err != nil || head == "" {
					return errors.New("could not determine the current branch; pass `--head`")
				}
			}
			credential, err := auth.Token(cmd.Context(), repository.Host)
			if err != nil {
				return &authError{err}
			}
			client := forgejo.New(repository.APIBase(), credential.Token)
			if base == "" {
				repo, err := client.Repos.Get(cmd.Context(), repository.Owner, repository.Repo)
				if err != nil {
					return wrapAuth(err)
				}
				base = repo.DefaultBranch
			}
			pr, err := client.Pulls.Create(cmd.Context(), repository.Owner, repository.Repo, forgejo.CreatePullRequest{
				Title: title, Body: body, Base: base, Head: head, Assignees: assignees,
			})
			if err != nil {
				var httpErr *forgejo.Error
				if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusUnprocessableEntity) {
					return fmt.Errorf("%w: is branch %q pushed to the repository, and is there a difference with %q?", err, head, base)
				}
				return wrapAuth(err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), pr.HTMLURL)
			return err
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Title for the pull request")
	cmd.Flags().StringVarP(&body, "body", "b", "", "Body for the pull request")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringVarP(&base, "base", "B", "", "The `branch` into which you want your code merged")
	cmd.Flags().StringVarP(&head, "head", "H", "", "The `branch` that contains commits for your pull request (default: current branch)")
	cmd.Flags().StringSliceVarP(&assignees, "assignee", "a", nil, "Assign people by their `login`")
	return cmd
}
