package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRCreateAndRepoView(t *testing.T) {
	var posted map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/o/r", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"r","full_name":"o/r","default_branch":"trunk","html_url":"https://h/o/r","ssh_url":"ssh://git@h/o/r.git","private":true,"owner":{"login":"o"}}`))
	})
	mux.HandleFunc("/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		data, _ := io.ReadAll(r.Body)
		assert.NoError(t, json.Unmarshal(data, &posted))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":3,"html_url":"https://h/o/r/pulls/3"}`))
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	host := strings.TrimPrefix(server.URL, "https://")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FGH_TOKEN", "tok")
	t.Setenv("FGH_HOST", "")
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "feat", dir}, {"-C", dir, "remote", "add", "origin", "https://" + host + "/o/r.git"}} {
		require.NoError(t, exec.Command("git", args...).Run())
	}
	t.Chdir(dir)

	out, err := run(t, "pr", "create", "--assignee", "bob", "--title", "T", "--body", "B")
	require.NoError(t, err)
	require.Equal(t, "https://h/o/r/pulls/3\n", out)
	require.Equal(t, map[string]any{"title": "T", "body": "B", "base": "trunk", "head": "feat", "assignees": []any{"bob"}}, posted)

	_, err = run(t, "pr", "create", "--title", "T")
	require.ErrorContains(t, err, "--body")

	out, err = run(t, "repo", "view", "--json", "owner,name", "--jq", `.owner.login+"/"+.name`)
	require.NoError(t, err)
	require.Equal(t, "o/r\n", out)

	out, err = run(t, "repo", "view", "o/r", "--json", "defaultBranchRef,isPrivate,nameWithOwner,url,sshUrl")
	require.NoError(t, err)
	require.JSONEq(t, `{"defaultBranchRef":{"name":"trunk"},"isPrivate":true,"nameWithOwner":"o/r","url":"https://h/o/r","sshUrl":"ssh://git@h/o/r.git"}`, out)

	_, err = run(t, "repo", "view", "--json", "bogus")
	require.ErrorContains(t, err, "Available fields:")
}
