package forgejo

import (
	"context"
	"net/http"
)

// Repository is Forgejo's shape, with none of gh's names.
type Repository struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
	SSHURL        string `json:"ssh_url"`
	Private       bool   `json:"private"`
	Owner         *User  `json:"owner"`
}

type Repos struct{ client *Client }

func (r *Repos) Get(ctx context.Context, owner, repo string) (*Repository, error) {
	var out Repository
	if _, err := r.client.Do(ctx, http.MethodGet, repoPath(owner, repo), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
