package ghjson

import (
	"fmt"
	"strings"

	"github.com/fabienjuif/fgh/forgejo"
)

func SearchFields() []string {
	return []string{"assignees", "author", "body", "closedAt", "commentsCount", "createdAt", "isDraft", "labels", "number", "repository", "state", "title", "updatedAt", "url"}
}

func ParseSearchFields(fields []string) error {
	for _, f := range fields {
		found := false
		for _, supported := range SearchFields() {
			if supported == f {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Unknown JSON field: %q\nAvailable fields:\n  %s", f, strings.Join(SearchFields(), "\n  ")) //nolint:staticcheck // gh's wording
		}
	}
	return nil
}

func Search(p *forgejo.SearchResult, fields []string) (map[string]any, error) {
	if err := ParseSearchFields(fields); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, f := range fields {
		switch f {
		case "repository":
			out[f] = map[string]any{"name": p.Repository.Name, "nameWithOwner": p.Repository.FullName}
		case "commentsCount":
			out[f] = p.Comments
		case "state":
			out[f] = strings.ToLower(State(&p.PullRequest))
			if out[f] == "merged" {
				out[f] = "closed"
			}
		case "isDraft":
			out[f] = p.Draft || (p.Pull != nil && p.Pull.Draft)
		default:
			out[f] = prFields[f](&p.PullRequest)
		}
	}
	return out, nil
}
