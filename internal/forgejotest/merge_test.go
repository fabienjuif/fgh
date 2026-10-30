//go:build integration

package forgejotest_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/internal/forgejotest"
)

func TestPinnedMerges(t *testing.T) {
	in := forgejotest.Start(t)
	owner, repo, _ := strings.Cut(in.Repo, "/")
	target, err := url.Parse(in.URL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	rechecks := 0
	// Inject one 405 before forwarding to the real forge, to make the recheck test deterministic.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/merge") && rechecks == 0 {
			rechecks++
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"message":"mergeability recheck"}`))
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer server.Close()
	client := forgejo.New(server.URL, in.Token)
	ctx := context.Background()
	pr, err := client.Pulls.Get(ctx, owner, repo, int64(in.BranchPR))
	require.NoError(t, err)
	require.NotNil(t, pr.Head)
	title, body := "pinned subject", ""

	// Move the branch after reading its head, then try the stale pin.
	dir := t.TempDir()
	cloneURL, err := url.Parse(in.CloneURL)
	require.NoError(t, err)
	cloneURL.User = url.UserPassword(in.User, in.Token)
	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	git("clone", "-q", cloneURL.String(), ".")
	git("checkout", "-q", in.BranchHead)
	git("-c", "user.name=fgh it", "-c", "user.email=it@example.invalid", "commit", "--allow-empty", "-m", "move head")
	git("push", "-q", "origin", in.BranchHead)
	_, err = client.Pulls.Merge(ctx, owner, repo, pr.Number, forgejo.MergeOptions{Method: "squash", HeadSHA: pr.Head.SHA})
	require.Error(t, err)
	current, err := client.Pulls.Get(ctx, owner, repo, pr.Number)
	require.NoError(t, err)
	require.False(t, current.Merged)
	_, err = client.Pulls.Merge(ctx, owner, repo, pr.Number, forgejo.MergeOptions{Method: "squash", Title: &title, Message: &body, HeadSHA: current.Head.SHA})
	require.NoError(t, err)
	require.Equal(t, 1, rechecks)
	merged, err := client.Pulls.Get(ctx, owner, repo, pr.Number)
	require.NoError(t, err)
	require.True(t, merged.Merged)
	require.NotNil(t, merged.MergeCommitSHA)
	var commit struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	_, err = client.Do(ctx, http.MethodGet, fmt.Sprintf("repos/%s/git/commits/%s", in.Repo, *merged.MergeCommitSHA), nil, &commit)
	require.NoError(t, err)
	require.Equal(t, title, strings.TrimRight(commit.Commit.Message, "\n"))
	agit, err := client.Pulls.Get(ctx, owner, repo, int64(in.AGitPR))
	require.NoError(t, err)
	require.Equal(t, forgejo.FlowAGit, agit.Flow)
	noBranch, err := client.Pulls.Merge(ctx, owner, repo, agit.Number, forgejo.MergeOptions{Method: "squash", DeleteBranch: true})
	require.NoError(t, err)
	require.True(t, noBranch)
	agit, err = client.Pulls.Get(ctx, owner, repo, agit.Number)
	require.NoError(t, err)
	require.True(t, agit.Merged)
}
