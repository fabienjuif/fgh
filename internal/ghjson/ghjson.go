// Package ghjson maps Forgejo types to gh's JSON field names.
package ghjson

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/itchyny/gojq"

	"github.com/fabienjuif/fgh/forgejo"
)

// prFields maps each supported `gh pr view --json` field to its value.
var prFields = map[string]func(*forgejo.PullRequest) any{
	"number":      func(p *forgejo.PullRequest) any { return p.Number },
	"title":       func(p *forgejo.PullRequest) any { return p.Title },
	"body":        func(p *forgejo.PullRequest) any { return p.Body },
	"url":         func(p *forgejo.PullRequest) any { return p.HTMLURL },
	"state":       func(p *forgejo.PullRequest) any { return State(p) },
	"isDraft":     func(p *forgejo.PullRequest) any { return p.Draft },
	"headRefName": func(p *forgejo.PullRequest) any { return p.HeadBranch() },
	"headRefOid":  func(p *forgejo.PullRequest) any { return branch(p.Head).SHA },
	"baseRefName": func(p *forgejo.PullRequest) any { return branch(p.Base).Ref },
	"baseRefOid":  func(p *forgejo.PullRequest) any { return branch(p.Base).SHA },
	"isCrossRepository": func(p *forgejo.PullRequest) any {
		return branch(p.Head).RepoID != branch(p.Base).RepoID
	},
	"mergeable": func(p *forgejo.PullRequest) any {
		switch {
		case p.Mergeable == nil:
			return "UNKNOWN"
		case *p.Mergeable:
			return "MERGEABLE"
		}
		return "CONFLICTING"
	},
	"mergedAt":  func(p *forgejo.PullRequest) any { return timeValue(p.MergedAt) },
	"closedAt":  func(p *forgejo.PullRequest) any { return timeValue(p.ClosedAt) },
	"createdAt": func(p *forgejo.PullRequest) any { return timeValue(p.CreatedAt) },
	"updatedAt": func(p *forgejo.PullRequest) any { return timeValue(p.UpdatedAt) },
	"author": func(p *forgejo.PullRequest) any {
		if p.User == nil {
			return nil
		}
		return map[string]any{"login": p.User.Login}
	},
	"assignees": func(p *forgejo.PullRequest) any { return logins(p.Assignees) },
	"labels": func(p *forgejo.PullRequest) any {
		out := []map[string]any{}
		for _, l := range p.Labels {
			out = append(out, map[string]any{"name": l.Name})
		}
		return out
	},
	"reviewRequests": func(p *forgejo.PullRequest) any { return logins(p.RequestedReviewers) },
	"additions":      func(p *forgejo.PullRequest) any { return p.Additions },
	"deletions":      func(p *forgejo.PullRequest) any { return p.Deletions },
	"changedFiles":   func(p *forgejo.PullRequest) any { return p.ChangedFiles },
	"mergeCommit": func(p *forgejo.PullRequest) any {
		if p.MergeCommitSHA == nil || *p.MergeCommitSHA == "" {
			return nil
		}
		return map[string]any{"oid": *p.MergeCommitSHA}
	},
}

// ghOnly are gh fields Forgejo cannot answer in v1: told apart from a typo.
var ghOnly = map[string]bool{
	"autoMergeRequest": true, "comments": true, "commits": true,
	"files": true, "fullDatabaseId": true, "id": true, "latestReviews": true, "maintainerCanModify": true,
	"mergeStateStatus": true, "mergedBy": true, "milestone": true, "potentialMergeCommit": true,
	"projectCards": true, "projectItems": true, "reactionGroups": true, "reviewDecision": true,
	"reviews": true, "statusCheckRollup": true, "closingIssuesReferences": true,
	"headRepository": true, "headRepositoryOwner": true, "isInMergeQueue": true,
	"isMergeQueueEnabled": true, "labelsCount": true,
}

// PRFields lists the supported field names, sorted.
func PRFields() []string {
	names := make([]string, 0, len(prFields))
	for n := range prFields {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseFields validates a --json value (comma separated, already split by the caller).
func ParseFields(fields []string) error {
	for _, f := range fields {
		if _, ok := prFields[f]; ok {
			continue
		}
		if ghOnly[f] {
			return fmt.Errorf("field %q is not supported on Forgejo", f)
		}
		return fmt.Errorf("Unknown JSON field: %q\nAvailable fields:\n  %s", f, strings.Join(PRFields(), "\n  ")) //nolint:staticcheck // gh's wording
	}
	return nil
}

// PR returns the requested fields of a pull request under gh's names.
func PR(p *forgejo.PullRequest, fields []string) (map[string]any, error) {
	if err := ParseFields(fields); err != nil {
		return nil, err
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[f] = prFields[f](p)
	}
	return out, nil
}

// State is gh's OPEN, CLOSED or MERGED.
func State(p *forgejo.PullRequest) string {
	switch {
	case p.Merged:
		return "MERGED"
	case p.State == "open":
		return "OPEN"
	}
	return "CLOSED"
}

// Jq runs a jq expression over a value and returns gh's rendering: strings raw, the rest compact JSON.
func Jq(ctx context.Context, expr string, value any) ([]string, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}
	// gojq only walks plain JSON types, so round-trip the value.
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var plain any
	if err = json.Unmarshal(data, &plain); err != nil {
		return nil, err
	}
	var lines []string
	iter := query.RunWithContext(ctx, plain)
	for {
		v, ok := iter.Next()
		if !ok {
			return lines, nil
		}
		if err, isErr := v.(error); isErr {
			return lines, err
		}
		if s, isStr := v.(string); isStr {
			lines = append(lines, s)
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return lines, err
		}
		lines = append(lines, string(b))
	}
}

func branch(b *forgejo.Branch) forgejo.Branch {
	if b == nil {
		return forgejo.Branch{}
	}
	return *b
}

func logins(users []forgejo.User) []map[string]any {
	out := []map[string]any{}
	for _, u := range users {
		out = append(out, map[string]any{"login": u.Login})
	}
	return out
}

func timeValue(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
