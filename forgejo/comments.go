package forgejo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Comment is a comment on a pull request's conversation (Forgejo treats a pull request as an issue).
type Comment struct {
	ID        int64      `json:"id"`
	Body      string     `json:"body"`
	HTMLURL   string     `json:"html_url"`
	User      *User      `json:"user"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type Comments struct{ client *Client }

// Create adds a comment to the pull request or issue number.
func (c *Comments) Create(ctx context.Context, owner, repo string, number int64, body string) (*Comment, error) {
	var out Comment
	path := repoPath(owner, repo) + fmt.Sprintf("/issues/%d/comments", number)
	if _, err := c.client.Do(ctx, http.MethodPost, path, map[string]string{"body": body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Edit replaces a comment's body. Comment ids are repository-wide, so no number is needed.
func (c *Comments) Edit(ctx context.Context, owner, repo string, id int64, body string) (*Comment, error) {
	var out Comment
	path := repoPath(owner, repo) + fmt.Sprintf("/issues/comments/%d", id)
	if _, err := c.client.Do(ctx, http.MethodPatch, path, map[string]string{"body": body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every comment of the pull request or issue number, oldest first.
func (c *Comments) List(ctx context.Context, owner, repo string, number int64) ([]Comment, error) {
	var all []Comment
	path := repoPath(owner, repo) + fmt.Sprintf("/issues/%d/comments?limit=50", number)
	err := c.client.Paginate(ctx, path, func(page json.RawMessage, _ *Response) error {
		var comments []Comment
		if err := json.Unmarshal(page, &comments); err != nil {
			return err
		}
		all = append(all, comments...)
		return nil
	})
	return all, err
}
