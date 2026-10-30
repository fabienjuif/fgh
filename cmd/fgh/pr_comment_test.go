package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPRDiffAndComment(t *testing.T) {
	const diff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-x\n+y\ndiff --git a/old.txt b/dir/new.txt\nrename from old.txt\n"
	comments := []map[string]any{
		{"id": 1, "body": "mine", "html_url": "https://h/c1", "user": map[string]any{"login": "bot"}},
		{"id": 2, "body": "other", "html_url": "https://h/c2", "user": map[string]any{"login": "alice"}},
	}
	var patched string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/o/r/pulls/7.diff", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(diff)) })
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"login":"bot"}`)) })
	mux.HandleFunc("/api/v1/repos/o/r/issues/7/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(comments)
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		comments = append(comments, map[string]any{"id": 3, "body": in["body"], "html_url": "https://h/c3", "user": map[string]any{"login": "bot"}})
		_ = json.NewEncoder(w).Encode(comments[len(comments)-1])
	})
	mux.HandleFunc("/api/v1/repos/o/r/issues/comments/3", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		patched = in["body"]
		_, _ = w.Write([]byte(`{"id":3,"html_url":"https://h/c3"}`))
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FGH_TOKEN", "tok")
	host := strings.TrimPrefix(server.URL, "https://")
	t.Chdir(t.TempDir())
	require.NoError(t, exec.Command("git", "init", "-q", ".").Run())
	t.Setenv("FGH_HOST", host)
	repo := "o/r"

	out, err := run(t, "pr", "diff", "7", "-R", repo)
	require.NoError(t, err)
	require.Equal(t, diff, out)

	out, err = run(t, "pr", "diff", "7", "-R", repo, "--name-only")
	require.NoError(t, err)
	require.Equal(t, "a.go\ndir/new.txt\n", out)

	out, err = run(t, "pr", "comment", "7", "-R", repo, "--body", "hi")
	require.NoError(t, err)
	require.Equal(t, "https://h/c3\n", out)
	require.Len(t, comments, 3)

	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	require.NoError(t, err)
	_, _ = stdin.WriteString("edited\nbody\n")
	_, err = stdin.Seek(0, 0)
	require.NoError(t, err)
	originalStdin := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = originalStdin })
	out, err = run(t, "pr", "comment", "7", "-R", repo, "--body-file", "-", "--edit-last")
	require.NoError(t, err)
	require.Equal(t, "https://h/c3\n", out)
	require.Len(t, comments, 3)
	require.Equal(t, "edited\nbody\n", patched)

	_, err = run(t, "pr", "comment", "7", "-R", repo)
	require.ErrorContains(t, err, "--body")
}
