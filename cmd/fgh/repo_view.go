package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/internal/ghjson"
	"github.com/fabienjuif/fgh/remote"
)

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "repo <command>", Short: "Manage repositories", RunE: groupRunE}
	cmd.AddCommand(newRepoViewCmd())
	return cmd
}

func newRepoViewCmd() *cobra.Command {
	var fields []string
	var jq string
	cmd := &cobra.Command{
		Use:   "view [<repository>]",
		Short: "View a repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("json") && len(fields) == 0 {
				return fmt.Errorf("Specify one or more comma-separated fields for `--json`:\n  %s", joinLines(ghjson.RepoFields())) //nolint:staticcheck // gh's wording
			}
			if jq != "" && len(fields) == 0 {
				return errors.New("cannot use `--jq` without specifying `--json`")
			}
			override, err := cmd.Flags().GetString("repo")
			if err != nil {
				return err
			}
			if len(args) == 1 {
				override = args[0]
			}
			repository, err := remote.Resolve(cmd.Context(), "", override)
			if err != nil {
				return err
			}
			credential, err := auth.Token(cmd.Context(), repository.Host)
			if err != nil {
				return &authError{err}
			}
			client := forgejo.New(repository.APIBase(), credential.Token)
			repo, err := client.Repos.Get(cmd.Context(), repository.Owner, repository.Repo)
			if err != nil {
				var httpErr *forgejo.Error
				if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden) {
					return &authError{err}
				}
				return err
			}
			out := cmd.OutOrStdout()
			if len(fields) == 0 {
				_, err = fmt.Fprintf(out, "name:\t%s\nprivate:\t%t\ndefault branch:\t%s\nurl:\t%s\n", repo.FullName, repo.Private, repo.DefaultBranch, repo.HTMLURL)
				return err
			}
			value, err := ghjson.Repo(repo, fields)
			if err != nil {
				return err
			}
			if jq != "" {
				lines, err := ghjson.Jq(cmd.Context(), jq, value)
				for _, l := range lines {
					_, _ = fmt.Fprintln(out, l)
				}
				return err
			}
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, string(data))
			return err
		},
	}
	cmd.Flags().StringSliceVar(&fields, "json", nil, "Output JSON with the specified `fields`")
	cmd.Flags().StringVarP(&jq, "jq", "q", "", "Filter JSON output using a jq `expression`")
	return cmd
}
