package forgejo

import (
	"context"
	"encoding/json"
	"net/url"
)

// SearchResult is an issue returned by Forgejo's cross-repository search.
type SearchResult struct {
	PullRequest
	Repository RepositorySummary `json:"repository"`
	Comments   int               `json:"comments"`
	Pull       *struct {
		Draft  bool `json:"draft"`
		Merged bool `json:"merged"`
	} `json:"pull_request"`
}

type RepositorySummary struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}

// Search follows every page; filters use Forgejo's API parameter names.
func (p *Pulls) Search(ctx context.Context, filters url.Values) ([]SearchResult, error) {
	q := url.Values{}
	for k, v := range filters {
		q[k] = append([]string(nil), v...)
	}
	q.Set("type", "pulls")
	q.Set("limit", "50")
	out := []SearchResult{}
	err := p.client.Paginate(ctx, "repos/issues/search?"+q.Encode(), func(raw json.RawMessage, _ *Response) error {
		var page []SearchResult
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	})
	return out, err
}
