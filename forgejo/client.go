// Package forgejo provides a standard-library client for Forgejo.
package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrRecheck  = errors.New("mergeability recheck")
)

// Client can be shared between goroutines after construction.
type Client struct {
	baseURL     string
	token       string
	http        *http.Client
	Users       *Users
	Pulls       *Pulls
	Repos       *Repos
	Comments    *Comments
	Attachments *Attachments
}

type Option func(*Client)

func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{baseURL: strings.TrimRight(baseURL, "/") + "/api/v1/", token: token, http: &http.Client{Timeout: 30 * time.Second}}
	for _, opt := range opts {
		opt(c)
	}
	c.Users = &Users{client: c}
	c.Pulls = &Pulls{client: c}
	c.Repos = &Repos{client: c}
	c.Comments = &Comments{client: c}
	c.Attachments = &Attachments{client: c}
	return c
}

func WithHTTPClient(client *http.Client) Option { return func(c *Client) { c.http = client } }

// Response preserves pagination metadata without exposing credentials.
type Response struct {
	Next       string
	TotalCount int
}

type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("Forgejo HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("Forgejo HTTP %d", e.StatusCode)
}

func (e *Error) Unwrap() error {
	switch e.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	case http.StatusMethodNotAllowed:
		return ErrRecheck
	}
	return nil
}

// Raw sends one request and returns the response whatever its status, for callers such as
// `fgh api` that relay the body of an error. The caller closes the body. header may override
// Accept and Content-Type but never Authorization.
func (c *Client) Raw(ctx context.Context, method, path string, header http.Header, body io.Reader) (*http.Response, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, base.Path) {
		path = strings.TrimPrefix(path, "/")
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	target := base.ResolveReference(ref)
	// Link headers are server-controlled; never send a token to a different origin.
	if target.Scheme != base.Scheme || target.Host != base.Host || !strings.HasPrefix(target.Path, base.Path) {
		return nil, errors.New("request URL is outside the API base")
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for name, values := range header {
		req.Header[http.CanonicalHeaderKey(name)] = values
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	} else {
		req.Header.Del("Authorization")
	}
	// Disable redirects, which could otherwise forward a token or change a mutation's method.
	client := *c.http
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forgejo request failed: %w", sanitizeTransportError(err))
	}
	return res, nil
}

// NextLink returns the rel="next" target of a Link header, or "".
func NextLink(h http.Header) string {
	for _, link := range strings.Split(h.Get("Link"), ",") {
		parts := strings.Split(strings.TrimSpace(link), ";")
		for _, param := range parts[1:] {
			if strings.TrimSpace(param) == `rel="next"` {
				return strings.Trim(strings.TrimSpace(parts[0]), "<>")
			}
		}
	}
	return ""
}

func (c *Client) Do(ctx context.Context, method, path string, body, out any) (*Response, error) {
	var reader io.Reader
	var header http.Header
	if body != nil {
		if r, ok := body.(io.Reader); ok {
			reader = r
		} else {
			data, marshalErr := json.Marshal(body)
			if marshalErr != nil {
				return nil, marshalErr
			}
			reader = bytes.NewReader(data)
		}
		header = http.Header{"Content-Type": {"application/json"}}
	}
	res, err := c.Raw(ctx, method, path, header, reader)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var detail struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&detail)
		return nil, &Error{StatusCode: res.StatusCode, Message: detail.Message}
	}
	meta := &Response{TotalCount: -1}
	if count, parseErr := strconv.Atoi(res.Header.Get("X-Total-Count")); parseErr == nil {
		meta.TotalCount = count
	}
	meta.Next = NextLink(res.Header)
	if out != nil {
		if writer, ok := out.(io.Writer); ok {
			_, err = io.Copy(writer, res.Body)
		} else {
			err = json.NewDecoder(res.Body).Decode(out)
			if errors.Is(err, io.EOF) {
				err = nil
			}
		}
	}
	return meta, err
}

// Paginate calls visit once per page, following Forgejo's next links.
func (c *Client) Paginate(ctx context.Context, path string, visit func(json.RawMessage, *Response) error) error {
	seen := map[string]bool{}
	for path != "" {
		if seen[path] {
			return errors.New("pagination link cycle")
		}
		seen[path] = true
		var page json.RawMessage
		meta, err := c.Do(ctx, http.MethodGet, path, nil, &page)
		if err != nil {
			return err
		}
		if err = visit(page, meta); err != nil {
			return err
		}
		path = meta.Next
	}
	return nil
}

func sanitizeTransportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return errors.New("transport error")
}
