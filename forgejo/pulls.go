package forgejo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FlowAGit is the PullRequest.Flow value of a pull request made by `git push origin HEAD:refs/for/...`.
const FlowAGit = 1

// PullRequest is Forgejo's shape, with none of gh's names.
type PullRequest struct {
	Number             int64      `json:"number"`
	Title              string     `json:"title"`
	Body               string     `json:"body"`
	HTMLURL            string     `json:"html_url"`
	State              string     `json:"state"`
	Draft              bool       `json:"draft"`
	Merged             bool       `json:"merged"`
	Mergeable          *bool      `json:"mergeable"` // null while Forgejo rechecks
	MergedAt           *time.Time `json:"merged_at"`
	ClosedAt           *time.Time `json:"closed_at"`
	CreatedAt          *time.Time `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at"`
	MergeCommitSHA     *string    `json:"merge_commit_sha"`
	Additions          int        `json:"additions"`
	Deletions          int        `json:"deletions"`
	ChangedFiles       int        `json:"changed_files"`
	Flow               int        `json:"flow"`
	User               *User      `json:"user"`
	Assignees          []User     `json:"assignees"`
	RequestedReviewers []User     `json:"requested_reviewers"`
	Labels             []Label    `json:"labels"`
	Head               *Branch    `json:"head"`
	Base               *Branch    `json:"base"`
}

type Label struct {
	Name string `json:"name"`
}

type Branch struct {
	Label  string `json:"label"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
	RepoID int64  `json:"repo_id"`
}

// HeadBranch is the branch name a user knows. head.ref is an internal refs/pull path for an AGit
// pull request, and for any pull request once its branch is deleted (the merge skill does that),
// so the name then comes from head.label: "owner/topic" for AGit, "branch" or "owner:branch" else.
func (p *PullRequest) HeadBranch() string {
	if p.Head == nil {
		return ""
	}
	if p.Flow == FlowAGit {
		_, topic, found := strings.Cut(p.Head.Label, "/")
		if found {
			return topic
		}
		return p.Head.Label
	}
	if strings.HasPrefix(p.Head.Ref, "refs/pull/") {
		_, branch, found := strings.Cut(p.Head.Label, ":")
		if found {
			return branch
		}
		return p.Head.Label
	}
	return p.Head.Ref
}

type Pulls struct{ client *Client }

func (p *Pulls) Get(ctx context.Context, owner, repo string, number int64) (*PullRequest, error) {
	var pr PullRequest
	if _, err := p.client.Do(ctx, http.MethodGet, repoPath(owner, repo)+fmt.Sprintf("/pulls/%d", number), nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// CreatePullRequest holds the fields of a new pull request. Head is a branch of the
// repository, or "owner:branch" for a fork.
type CreatePullRequest struct {
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Base      string   `json:"base"`
	Head      string   `json:"head"`
	Assignees []string `json:"assignees,omitempty"`
}

// Create opens a branch pull request. There is no AGit mode.
func (p *Pulls) Create(ctx context.Context, owner, repo string, in CreatePullRequest) (*PullRequest, error) {
	var pr PullRequest
	if _, err := p.client.Do(ctx, http.MethodPost, repoPath(owner, repo)+"/pulls", in, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// Diff returns the unified diff of a pull request.
func (p *Pulls) Diff(ctx context.Context, owner, repo string, number int64) (string, error) {
	var b strings.Builder
	if _, err := p.client.Do(ctx, http.MethodGet, repoPath(owner, repo)+fmt.Sprintf("/pulls/%d.diff", number), nil, &b); err != nil {
		return "", err
	}
	return b.String(), nil
}

// List returns the pull requests in a state ("open", "closed" or "all"), following every page.
func (p *Pulls) List(ctx context.Context, owner, repo, state string) ([]PullRequest, error) {
	var all []PullRequest
	path := repoPath(owner, repo) + "/pulls?limit=50&sort=latest&state=" + url.QueryEscape(state)
	err := p.client.Paginate(ctx, path, func(page json.RawMessage, _ *Response) error {
		var prs []PullRequest
		if err := json.Unmarshal(page, &prs); err != nil {
			return err
		}
		all = append(all, prs...)
		return nil
	})
	return all, err
}

func repoPath(owner, repo string) string {
	return "repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

// MergeOptions maps to Forgejo's merge endpoint. Title and Message are pointers so an
// explicit empty value is distinct from the forge's default.
type MergeOptions struct {
	Method       string
	Title        *string
	Message      *string
	HeadSHA      string
	DeleteBranch bool
}

// recheckWindow is how long Merge retries a 405; a variable so a test need not wait a minute.
var recheckWindow = time.Minute

// Merge retries only Forgejo's transient mergeability recheck response. An AGit pull request
// has no branch, and asking Forgejo to delete it makes the merge answer HTTP 500 after it
// landed, so DeleteBranch is dropped and noBranch reports it.
func (p *Pulls) Merge(ctx context.Context, owner, repo string, number int64, opts MergeOptions) (noBranch bool, err error) {
	switch opts.Method {
	case "merge", "squash", "rebase":
	default:
		return false, fmt.Errorf("unsupported merge method %q", opts.Method)
	}
	if opts.DeleteBranch {
		pr, err := p.Get(ctx, owner, repo, number)
		if err != nil {
			return false, err
		}
		if pr.Flow == FlowAGit {
			opts.DeleteBranch = false
			noBranch = true
		}
	}
	payload := struct {
		Method       string  `json:"Do"`
		Title        *string `json:"MergeTitleField,omitempty"`
		Message      *string `json:"MergeMessageField,omitempty"`
		HeadSHA      string  `json:"head_commit_id,omitempty"`
		DeleteBranch bool    `json:"delete_branch_after_merge,omitempty"`
	}{opts.Method, opts.Title, opts.Message, opts.HeadSHA, opts.DeleteBranch}
	retryCtx, cancel := context.WithTimeout(ctx, recheckWindow)
	defer cancel()
	var last error
	checked := false
	for {
		_, err := p.client.Do(retryCtx, http.MethodPost, repoPath(owner, repo)+fmt.Sprintf("/pulls/%d/merge", number), payload, nil)
		if !errors.Is(err, ErrRecheck) {
			if last != nil && retryCtx.Err() != nil && ctx.Err() == nil {
				return noBranch, explainRecheck(last, recheckWindow)
			}
			return noBranch, err
		}
		last = err
		if !checked {
			checked = true
			if blocked := p.notOpen(retryCtx, owner, repo, number); blocked != nil {
				return noBranch, blocked
			}
		}
		select {
		case <-retryCtx.Done():
			if ctx.Err() != nil {
				return noBranch, ctx.Err()
			}
			return noBranch, explainRecheck(last, recheckWindow)
		case <-time.After(time.Second):
		}
	}
}

// notOpen explains a 405 that no retry will cure. Forgejo answers 405 with an empty message for a
// merged, closed or draft pull request, and for a block such as a missing approval, so without this
// check `fgh pr merge` on an already merged pull request hangs for the whole window.
func (p *Pulls) notOpen(ctx context.Context, owner, repo string, number int64) error {
	// A failed read changes nothing: the merge is retried and its next answer says more.
	if pr, err := p.Get(ctx, owner, repo, number); err == nil {
		switch {
		case pr.Merged:
			return fmt.Errorf("pull request #%d is already merged", number)
		case pr.State != "open":
			return fmt.Errorf("pull request #%d is %s", number, pr.State)
		case pr.Draft:
			return fmt.Errorf("pull request #%d is a draft", number)
		}
	}
	return nil
}

func explainRecheck(last error, window time.Duration) error {
	var httpErr *Error
	if errors.As(last, &httpErr) && httpErr.Message == "" {
		return fmt.Errorf("%w: still not mergeable after %s and the forge gave no reason (a missing approval, branch protection or a failing check?)", last, window)
	}
	return last
}
