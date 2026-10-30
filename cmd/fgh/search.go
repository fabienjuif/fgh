package main

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/internal/ghjson"
)

func newSearchCmd() *cobra.Command {
	parent := &cobra.Command{Use: "search", Short: "Search Forgejo", RunE: groupRunE}
	var state, author, reviewer, involves, jq string
	var fields []string
	var limit int
	cmd := &cobra.Command{Use: "prs [query]", Short: "Search pull requests", Args: cobra.ArbitraryArgs}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateListJSON(cmd, fields, jq, true); err != nil {
			return err
		}
		if limit < 1 {
			return errors.New("invalid limit: must be positive")
		}
		if state != "" && state != "open" && state != "closed" {
			return fmt.Errorf("invalid state: %s", state)
		}
		if reviewer != "" && reviewer != "@me" {
			return errors.New("--review-requested supports only @me on Forgejo")
		}
		if involves != "" && involves != "@me" {
			return errors.New("--involves supports only @me on Forgejo")
		}
		host, err := authHost(cmd, "")
		if err != nil {
			return err
		}
		credential, err := auth.Token(cmd.Context(), host)
		if err != nil {
			return &authError{err}
		}
		client := forgejo.New("https://"+host, credential.Token)
		q := url.Values{"state": {"all"}, "sort": {"latest"}}
		if state != "" {
			q.Set("state", state)
		}
		if len(args) > 0 {
			q.Set("q", strings.Join(args, " "))
		}
		if author != "" {
			if author == "@me" {
				u, e := client.Users.Me(cmd.Context())
				if e != nil {
					return listAPIError(e)
				}
				author = u.Login
			}
		}
		if reviewer != "" {
			q.Set("review_requested", "true")
		}
		filters := []string{""}
		if involves != "" && reviewer == "" {
			filters = []string{"created", "assigned", "mentioned", "review_requested"}
		}
		results := []forgejo.SearchResult{}
		seen := map[string]bool{}
		for _, filter := range filters {
			if filter != "" {
				q.Set(filter, "true")
			}
			// Fetch all pages before local filtering and the union; unfiltered searches
			// can scan every pull request on the instance even with a small limit.
			page, e := client.Pulls.Search(cmd.Context(), q)
			if e != nil {
				return listAPIError(e)
			}
			if filter != "" && (filter != "review_requested" || reviewer == "") {
				q.Del(filter)
			}
			for _, p := range page {
				if author != "" && (p.User == nil || p.User.Login != author) {
					continue
				}
				key := p.Repository.FullName + "#" + fmt.Sprint(p.Number)
				if !seen[key] {
					seen[key] = true
					results = append(results, p)
				}
			}
		}
		sort.SliceStable(results, func(i, j int) bool {
			a, b := results[i].CreatedAt, results[j].CreatedAt
			if a == nil {
				return false
			}
			if b == nil {
				return true
			}
			return a.After(*b)
		})
		values := []map[string]any{}
		for i := range results {
			if i == limit {
				break
			}
			p := &results[i]
			if len(fields) > 0 {
				v, e := ghjson.Search(p, fields)
				if e != nil {
					return e
				}
				values = append(values, v)
			} else {
				if _, e := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%s\n", p.Repository.FullName, p.Number, p.Title); e != nil {
					return e
				}
			}
		}
		if len(fields) > 0 {
			return writeListJSON(cmd, values, jq)
		}
		return nil
	}
	cmd.Flags().StringVar(&state, "state", "", "Filter by state: open, closed")
	cmd.Flags().StringVar(&author, "author", "", "Filter by author")
	cmd.Flags().StringVar(&reviewer, "review-requested", "", "Filter by requested reviewer")
	cmd.Flags().StringVar(&involves, "involves", "", "Filter by involvement")
	cmd.Flags().IntVarP(&limit, "limit", "L", 30, "Maximum results")
	cmd.Flags().StringSliceVar(&fields, "json", nil, "Output JSON fields")
	cmd.Flags().StringVarP(&jq, "jq", "q", "", "Filter JSON with jq")
	parent.AddCommand(cmd)
	return parent
}
