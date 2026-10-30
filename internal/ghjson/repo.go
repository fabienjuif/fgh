package ghjson

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fabienjuif/fgh/forgejo"
)

var repoFields = []string{"defaultBranchRef", "isPrivate", "name", "nameWithOwner", "owner", "sshUrl", "url"}

// RepoFields lists the supported `repo view --json` fields.
func RepoFields() []string { return append([]string(nil), repoFields...) }

// Repo maps a Forgejo repository to gh's `repo view --json` shape.
func Repo(r *forgejo.Repository, fields []string) (map[string]any, error) {
	all := map[string]any{
		"defaultBranchRef": map[string]any{"name": r.DefaultBranch},
		"isPrivate":        r.Private,
		"name":             r.Name,
		"nameWithOwner":    r.FullName,
		"sshUrl":           r.SSHURL,
		"url":              r.HTMLURL,
	}
	login := ""
	if r.Owner != nil {
		login = r.Owner.Login
	}
	all["owner"] = map[string]any{"login": login}
	out := map[string]any{}
	for _, f := range fields {
		v, ok := all[f]
		if !ok {
			return nil, fmt.Errorf("Unknown JSON field: %q\nAvailable fields:\n  %s", f, strings.Join(sortedCopy(repoFields), "\n  ")) //nolint:staticcheck // gh's wording
		}
		out[f] = v
	}
	return out, nil
}

func sortedCopy(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}
