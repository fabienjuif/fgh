package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/remote"
)

func newPRDiffCmd() *cobra.Command {
	var nameOnly bool
	cmd := &cobra.Command{
		Use:   "diff [<number> | <url> | <branch>]",
		Short: "View changes in a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, repository, number, err := prTarget(cmd, args)
			if err != nil {
				return err
			}
			diff, err := client.Pulls.Diff(cmd.Context(), repository.Owner, repository.Repo, number)
			if err != nil {
				return wrapAuth(err)
			}
			out := cmd.OutOrStdout()
			if !nameOnly {
				_, err = fmt.Fprint(out, diff)
				return err
			}
			for _, name := range diffNames(diff) {
				_, _ = fmt.Fprintln(out, name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&nameOnly, "name-only", false, "Display only names of changed files")
	return cmd
}

// prTarget resolves the repository, the token and the pull request number from the arguments.
func prTarget(cmd *cobra.Command, args []string) (*forgejo.Client, remote.Repository, int64, error) {
	override, err := cmd.Flags().GetString("repo")
	if err != nil {
		return nil, remote.Repository{}, 0, err
	}
	arg := ""
	if len(args) == 1 {
		arg = args[0]
	}
	repository, number, branch, err := resolvePRArg(cmd, arg, override)
	if err != nil {
		return nil, repository, 0, err
	}
	credential, err := auth.Token(cmd.Context(), repository.Host)
	if err != nil {
		return nil, repository, 0, &authError{err}
	}
	client := forgejo.New(repository.APIBase(), credential.Token)
	if number == 0 {
		number, err = findByBranch(cmd, client, repository, branch)
		if err != nil {
			return nil, repository, 0, wrapAuth(err)
		}
	}
	return client, repository, number, nil
}

func wrapAuth(err error) error {
	var httpErr *forgejo.Error
	if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden) {
		return &authError{err}
	}
	return err
}

// diffNames lists the changed files from the "diff --git a/x b/y" headers, as gh does (the new name).
func diffNames(diff string) []string {
	var names []string
	for _, line := range strings.Split(diff, "\n") {
		rest, ok := strings.CutPrefix(line, "diff --git ")
		if !ok {
			continue
		}
		if i := strings.LastIndex(rest, " b/"); i >= 0 {
			names = append(names, rest[i+3:])
		}
	}
	return names
}
