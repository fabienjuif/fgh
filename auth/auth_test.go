package auth

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	// A real git credential invocation exercises the protocol without contacting a host.
	require.NoError(t, exec.Command("git", "config", "--global", "credential.helper", "!f() { echo username=alice; echo password=helper; }; f").Run())
	t.Setenv("FGH_TOKEN", "fgh")
	t.Setenv("FORGEJO_TOKEN", "forgejo")
	require.NoError(t, Save("host", "stored"))
	for _, tc := range []struct{ source, token string }{{"FGH_TOKEN", "fgh"}, {"FORGEJO_TOKEN", "forgejo"}, {"stored", "stored"}, {"git credential", "helper"}} {
		credential, err := Token(context.Background(), "host")
		require.NoError(t, err)
		require.Equal(t, tc.token, credential.Token)
		if tc.source == "stored" {
			require.Equal(t, filepath.Join(home, ".config", "fgh", "hosts.json"), credential.Source)
			require.NoError(t, os.Remove(credential.Source))
		} else {
			require.Equal(t, tc.source, credential.Source)
			if tc.source != "git credential" {
				t.Setenv(tc.source, "")
			}
		}
	}
}

func TestSavePermissionsAndPreservation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, Save("one", "first"))
	require.NoError(t, Save("two", "second"))
	path, err := configPath()
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	hosts, err := readHosts(path)
	require.NoError(t, err)
	require.Equal(t, "first", hosts["one"].Token)
	require.Equal(t, "second", hosts["two"].Token)
	require.NoError(t, os.Chmod(path, 0o644))
	_, err = readHosts(path)
	require.Error(t, err)
	require.Error(t, Save("host", ""))
}

func TestMissingToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FGH_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("PATH", t.TempDir())
	_, err := Token(context.Background(), "host")
	require.ErrorIs(t, err, ErrNoToken)
}
