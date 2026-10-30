package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func listServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	host := strings.TrimPrefix(server.URL, "https://")
	t.Setenv("FGH_TOKEN", "tok")
	t.Setenv("FGH_HOST", host)
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	return host
}

func TestPRListCatchUp(t *testing.T) {
	listServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/repos/o/r/pulls", r.URL.Path)
		assert.Equal(t, "open", r.URL.Query().Get("state"))
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `</api/v1/repos/o/r/pulls?state=open&page=2>; rel="next"`)
			_, _ = fmt.Fprint(w, `[{"number":1,"user":{"login":"bob"},"labels":[{"name":"bug"}]}]`)
		} else {
			_, _ = fmt.Fprint(w, `[{"number":2,"title":"Fix","html_url":"https://example/o/r/pulls/2","user":{"login":"alice"},"updated_at":"2026-10-30T00:00:00Z","draft":true,"requested_reviewers":[{"login":"bob"}],"labels":[{"name":"bug"}]}]`)
		}
	})
	out, err := run(t, "pr", "list", "--repo", "o/r", "--state", "open", "--author", "alice", "--label", "bug", "--limit", "1", "--json", "number,title,author,url,updatedAt,isDraft,reviewRequests")
	require.NoError(t, err)
	require.JSONEq(t, `[{"number":2,"title":"Fix","author":{"login":"alice"},"url":"https://example/o/r/pulls/2","updatedAt":"2026-10-30T00:00:00Z","isDraft":true,"reviewRequests":[{"login":"bob"}]}]`, out)
	out, err = run(t, "pr", "list", "-R", "o/r", "--json", "number", "--jq", ".[].number")
	require.NoError(t, err)
	require.Equal(t, "1\n2\n", out)
}

func TestPRListStates(t *testing.T) {
	listServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, []string{"closed", "all"}, r.URL.Query().Get("state"))
		_, _ = fmt.Fprint(w, `[{"number":1,"state":"closed","merged":false},{"number":2,"state":"closed","merged":true}]`)
	})
	for _, tc := range []struct{ state, want string }{{"closed", `[{"number":1},{"number":2}]`}, {"merged", `[{"number":2}]`}, {"all", `[{"number":1},{"number":2}]`}} {
		out, err := run(t, "pr", "list", "-R", "o/r", "--state", tc.state, "--json", "number")
		require.NoError(t, err)
		require.JSONEq(t, tc.want, out)
	}
}

func TestSearchCatchUp(t *testing.T) {
	filters := []string{}
	listServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/repos/issues/search", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, "pulls", q.Get("type"))
		assert.Equal(t, "open", q.Get("state"))
		filter := ""
		for _, f := range []string{"created", "assigned", "mentioned", "review_requested"} {
			if q.Get(f) == "true" {
				assert.Empty(t, filter)
				filter = f
			}
		}
		assert.NotEmpty(t, filter)
		filters = append(filters, filter)
		if q.Get("page") == "" {
			next := q
			next.Set("page", "2")
			w.Header().Set("Link", `</api/v1/repos/issues/search?`+next.Encode()+`>; rel="next"`)
			_, _ = fmt.Fprint(w, `[{"number":7,"title":"Fix","repository":{"name":"r","full_name":"o/r"},"html_url":"https://example/o/r/pulls/7","updated_at":"2026-10-30T00:00:00Z","user":{"login":"alice"},"comments":3,"pull_request":{"draft":true},"state":"open"}]`)
		} else {
			_, _ = fmt.Fprint(w, `[{"number":7,"repository":{"name":"other","full_name":"o/other"},"state":"open"}]`)
		}
	})
	out, err := run(t, "search", "prs", "--state", "open", "--involves", "@me", "--json", "number,title,repository,url,updatedAt")
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got, 2)
	require.Equal(t, map[string]any{"name": "r", "nameWithOwner": "o/r"}, got[0]["repository"])
	require.Equal(t, "Fix", got[0]["title"])
	require.Equal(t, "2026-10-30T00:00:00Z", got[0]["updatedAt"])
	require.Equal(t, []string{"created", "created", "assigned", "assigned", "mentioned", "mentioned", "review_requested", "review_requested"}, filters)
	filters = nil
	out, err = run(t, "search", "prs", "--state", "open", "--review-requested", "@me", "--author", "alice", "--json", "number,author,commentsCount,isDraft,state", "--limit", "1")
	require.NoError(t, err)
	require.JSONEq(t, `[{"number":7,"author":{"login":"alice"},"commentsCount":3,"isDraft":true,"state":"open"}]`, out)
	require.Equal(t, []string{"review_requested", "review_requested"}, filters)
	filters = nil
	_, err = run(t, "search", "prs", "--state", "open", "--review-requested", "@me", "--involves", "@me", "--json", "number")
	require.NoError(t, err)
	require.Equal(t, []string{"review_requested", "review_requested"}, filters)
}

func TestListValidationAndErrors(t *testing.T) {
	for _, args := range [][]string{
		{"pr", "list", "--json", "bogus"},
		{"search", "prs", "--json", "reviewRequests"},
		{"pr", "list", "--jq", "."},
		{"search", "prs", "--json", ""},
		{"pr", "list", "--state", "bogus"},
		{"search", "prs", "--limit", "0"},
		{"search", "prs", "--review-requested", "alice"},
	} {
		_, err := run(t, args...)
		require.Error(t, err)
	}
	listServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	for _, args := range [][]string{{"pr", "list", "-R", "o/r"}, {"search", "prs", "--involves", "@me"}} {
		_, err := run(t, args...)
		require.Error(t, err)
		require.Equal(t, 4, exitCode(err))
	}
}

func TestListsEmpty(t *testing.T) {
	listServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "[]") })
	for _, args := range [][]string{{"pr", "list", "-R", "o/r", "--json", "number"}, {"search", "prs", "--json", "number"}} {
		out, err := run(t, args...)
		require.NoError(t, err)
		require.JSONEq(t, "[]", out)
	}
}
