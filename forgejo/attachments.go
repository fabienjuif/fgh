package forgejo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

// Attachment is Forgejo's shape for a file attached to an issue or a pull request.
type Attachment struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Size               int64      `json:"size"`
	UUID               string     `json:"uuid"`
	BrowserDownloadURL string     `json:"browser_download_url"`
	CreatedAt          *time.Time `json:"created_at"`
}

type Attachments struct{ client *Client }

// Upload attaches r as name to issue or pull request index (they share one numbering). The body is
// streamed, so a large file is never held in memory. Forgejo checks the content against its allowed
// types and answers 422 otherwise.
func (a *Attachments) Upload(ctx context.Context, owner, repo string, index int64, name string, r io.Reader) (*Attachment, error) {
	if name == "" {
		return nil, errors.New("attachment name is empty")
	}
	pr, pw := io.Pipe()
	form := multipart.NewWriter(pw)
	go func() {
		part, err := form.CreateFormFile("attachment", name)
		if err == nil {
			_, err = io.Copy(part, r)
		}
		if err == nil {
			err = form.Close()
		}
		_ = pw.CloseWithError(err)
	}()
	// Unblocks the writer goroutine when the request stops reading early.
	defer func() { _ = pr.Close() }()

	path := repoPath(owner, repo) + fmt.Sprintf("/issues/%d/assets?name=%s", index, url.QueryEscape(name))
	res, err := a.client.Raw(ctx, http.MethodPost, path, http.Header{"Content-Type": {form.FormDataContentType()}}, pr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &Error{StatusCode: res.StatusCode}
	}
	var att Attachment
	if err := json.NewDecoder(res.Body).Decode(&att); err != nil {
		return nil, err
	}
	return &att, nil
}
