package remote

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	for _, raw := range []string{"ssh://git@host:23398/o/r.git", "git@host:o/r.git", "https://host/o/r", "https://host/o/r.git"} {
		t.Run(raw, func(t *testing.T) {
			repo, err := Parse(raw)
			require.NoError(t, err)
			require.Equal(t, Repository{"host", "o", "r"}, repo)
			require.Equal(t, "https://host", repo.APIBase())
		})
	}
	repo, err := Parse("https://host:8443/o/r.git")
	require.NoError(t, err)
	require.Equal(t, "host:8443", repo.Host)
	for _, raw := range []string{"", "/local/repo", "file:///o/r", "https://host/o", "https://host/o/r/extra", "git@host:o/", "https://host/o/r?x=y"} {
		_, err := Parse(raw)
		require.Error(t, err, raw)
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	cmd := exec.Command("git", "-C", dir, "remote", "add", "origin", "ssh://git@host:23398/o/r.git")
	require.NoError(t, cmd.Run())
	t.Setenv("FGH_HOST", "")
	repo, err := Resolve(context.Background(), dir, "other/repo")
	require.NoError(t, err)
	require.Equal(t, Repository{"host", "other", "repo"}, repo)
	t.Setenv("FGH_HOST", "override")
	repo, err = Resolve(context.Background(), dir, "")
	require.NoError(t, err)
	require.Equal(t, Repository{"host", "o", "r"}, repo)
	repo, err = Resolve(context.Background(), dir, "other/repo")
	require.NoError(t, err)
	require.Equal(t, Repository{"host", "other", "repo"}, repo)
	host, err := Host(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "host", host)
	empty := t.TempDir()
	_, err = Resolve(context.Background(), empty, "")
	require.ErrorContains(t, err, "no repository")
	host, err = Host(context.Background(), empty)
	require.NoError(t, err)
	require.Equal(t, "override", host)
	repo, err = Resolve(context.Background(), t.TempDir(), "a/b")
	require.NoError(t, err)
	require.Equal(t, Repository{"override", "a", "b"}, repo)
	_, err = Resolve(context.Background(), dir, "bad")
	require.Error(t, err)
}
