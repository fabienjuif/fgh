package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/repos/issues/search", r.URL.Path)
		assert.Equal(t, "pulls", r.URL.Query().Get("type"))
		if r.URL.Query().Get("page") == "" {
			assert.Equal(t, "true", r.URL.Query().Get("review_requested"))
			w.Header().Set("Link", `</api/v1/repos/issues/search?type=pulls&page=2>; rel="next"`)
			_, _ = fmt.Fprint(w, `[{"number":1,"comments":4,"repository":{"name":"r","full_name":"o/r"},"pull_request":{"draft":true}}]`)
		} else {
			_, _ = fmt.Fprint(w, `[{"number":2}]`)
		}
	}))
	defer server.Close()
	q := url.Values{"review_requested": {"true"}}
	got, err := New(server.URL, "tok").Pulls.Search(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 4, got[0].Comments)
	require.True(t, got[0].Pull.Draft)
	require.Equal(t, "o/r", got[0].Repository.FullName)
	require.Equal(t, url.Values{"review_requested": {"true"}}, q)
}
