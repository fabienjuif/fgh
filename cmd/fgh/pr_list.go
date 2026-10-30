package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/internal/ghjson"
	"github.com/fabienjuif/fgh/remote"
)

func newPRListCmd() *cobra.Command {
	var state, author, jq string
	var labels, fields []string
	var limit int
	cmd := &cobra.Command{Use: "list", Short: "List pull requests", Args: cobra.NoArgs}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := validateListJSON(cmd, fields, jq, false); err != nil {
			return err
		}
		if limit < 1 {
			return errors.New("invalid limit: must be positive")
		}
		if state != "open" && state != "closed" && state != "merged" && state != "all" {
			return fmt.Errorf("invalid state: %s", state)
		}
		override, _ := cmd.Flags().GetString("repo")
		r, err := remote.Resolve(cmd.Context(), "", override)
		if err != nil {
			return err
		}
		credential, err := auth.Token(cmd.Context(), r.Host)
		if err != nil {
			return &authError{err}
		}
		client := forgejo.New(r.APIBase(), credential.Token)
		if author == "@me" {
			u, e := client.Users.Me(cmd.Context())
			if e != nil {
				return listAPIError(e)
			}
			author = u.Login
		}
		apiState := state
		if state == "merged" {
			apiState = "closed"
		}
		prs, err := client.Pulls.List(cmd.Context(), r.Owner, r.Repo, apiState)
		if err != nil {
			return listAPIError(err)
		}
		values := []map[string]any{}
		count := 0
		for i := range prs {
			p := &prs[i]
			if state == "merged" && !p.Merged {
				continue
			}
			if author != "" && (p.User == nil || p.User.Login != author) {
				continue
			}
			matches := true
			for _, label := range labels {
				found := false
				for _, l := range p.Labels {
					if l.Name == label {
						found = true
					}
				}
				if !found {
					matches = false
				}
			}
			if !matches {
				continue
			}
			if len(fields) > 0 {
				v, e := ghjson.PR(p, fields)
				if e != nil {
					return e
				}
				values = append(values, v)
			} else {
				if _, e := fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\n", p.Number, p.Title, p.HeadBranch()); e != nil {
					return e
				}
			}
			count++
			if count == limit {
				break
			}
		}
		if len(fields) > 0 {
			return writeListJSON(cmd, values, jq)
		}
		return nil
	}
	cmd.Flags().StringVarP(&state, "state", "s", "open", "Filter by state: open, closed, merged, all")
	cmd.Flags().StringVarP(&author, "author", "A", "", "Filter by author")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Filter by label")
	cmd.Flags().IntVarP(&limit, "limit", "L", 30, "Maximum results")
	cmd.Flags().StringSliceVar(&fields, "json", nil, "Output JSON fields")
	cmd.Flags().StringVarP(&jq, "jq", "q", "", "Filter JSON with jq")
	return cmd
}

func validateListJSON(cmd *cobra.Command, fields []string, jq string, search bool) error {
	names := ghjson.PRFields()
	if search {
		names = ghjson.SearchFields()
	}
	if cmd.Flags().Changed("json") && len(fields) == 0 {
		return fmt.Errorf("specify one or more comma-separated fields for --json:\n  %s", strings.Join(names, "\n  "))
	}
	if jq != "" && len(fields) == 0 {
		return errors.New("cannot use `--jq` without specifying `--json`")
	}
	if search {
		return ghjson.ParseSearchFields(fields)
	}
	return ghjson.ParseFields(fields)
}

func writeListJSON(cmd *cobra.Command, values []map[string]any, jq string) error {
	if jq != "" {
		lines, err := ghjson.Jq(cmd.Context(), jq, values)
		for _, line := range lines {
			if _, e := fmt.Fprintln(cmd.OutOrStdout(), line); e != nil {
				return e
			}
		}
		return err
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(values)
}

func listAPIError(err error) error {
	var e *forgejo.Error
	if errors.As(err, &e) && (e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden) {
		return &authError{err}
	}
	return err
}
