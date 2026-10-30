package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prServer fakes Forgejo over TLS: the CLI always dials https://<host>.
func prServer(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../internal/ghjson/testdata")
	require.NoError(t, err)
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return data
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/o/r/pulls/7", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(read("pr_branch.json")) })
	mux.HandleFunc("/api/v1/repos/o/r/pulls/8", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(read("pr_agit.json")) })
	mux.HandleFunc("/api/v1/repos/o/r/pulls/9", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(read("pr_merged.json")) })
	mux.HandleFunc("/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "open", r.URL.Query().Get("state"))
		_, _ = w.Write([]byte("[" + string(read("pr_branch.json")) + "," + string(read("pr_agit.json")) + "]"))
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FGH_TOKEN", "tok")
	t.Setenv("FGH_HOST", "")
	return strings.TrimPrefix(server.URL, "https://")
}

func TestPRView(t *testing.T) {
	host := prServer(t)
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "feat", dir}, {"-C", dir, "remote", "add", "origin", "https://" + host + "/o/r.git"}} {
		require.NoError(t, exec.Command("git", args...).Run())
	}
	t.Chdir(dir)

	out, err := run(t, "pr", "view", "7", "--json", "number,state,headRefName", "-q", ".state")
	require.NoError(t, err)
	require.Equal(t, "OPEN\n", out)

	out, err = run(t, "pr", "view", "--json", "number,headRefName")
	require.NoError(t, err)
	require.JSONEq(t, `{"number":7,"headRefName":"feat"}`, out)

	out, err = run(t, "pr", "view", "my-topic", "--json", "number")
	require.NoError(t, err)
	require.JSONEq(t, `{"number":8}`, out)

	out, err = run(t, "pr", "view", "https://"+host+"/o/r/pulls/9", "--json", "state")
	require.NoError(t, err)
	require.JSONEq(t, `{"state":"MERGED"}`, out)

	out, err = run(t, "pr", "view", "7")
	require.NoError(t, err)
	require.Contains(t, out, "Add feature #7")
	require.Contains(t, out, "alice")
	require.Contains(t, out, "Does a thing.")

	_, err = run(t, "pr", "view", "7", "--json", "bogus")
	require.ErrorContains(t, err, "Available fields:")
	_, err = run(t, "pr", "view", "7", "-q", ".x")
	require.ErrorContains(t, err, "--jq")
	_, err = run(t, "pr", "view", "nosuchbranch")
	require.ErrorContains(t, err, "no pull requests found")
}

func TestPRViewRepoFlag(t *testing.T) {
	host := prServer(t)
	t.Setenv("FGH_HOST", host)
	t.Chdir(t.TempDir())
	out, err := run(t, "pr", "view", "8", "-R", "o/r", "--json", "headRefName,mergeable")
	require.NoError(t, err)
	require.JSONEq(t, `{"headRefName":"my-topic","mergeable":"UNKNOWN"}`, out)
}
