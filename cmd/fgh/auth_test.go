package main

import (
	"bytes"
	"fmt"
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

func TestAuthStatusAndLogin(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/user", r.URL.Path)
		assert.Equal(t, "token top-secret", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"login":"alice"}`)
	}))
	defer server.Close()
	// Exercise HTTPS host resolution with the fake's trusted transport.
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FGH_HOST", "")
	t.Setenv("FGH_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	host := strings.TrimPrefix(server.URL, "https://")
	require.NoError(t, exec.Command("git", "-C", dir, "remote", "add", "origin", "https://"+host+"/o/r.git").Run())
	t.Chdir(dir)
	cmd := newRootCmd("test")
	cmd.SetArgs([]string{"auth", "login", "--with-token"})
	cmd.SetIn(strings.NewReader("top-secret\n"))
	require.NoError(t, cmd.Execute())
	info, err := os.Stat(filepath.Join(home, ".config", "fgh", "hosts.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	out, err := run(t, "auth", "status")
	require.NoError(t, err)
	require.Contains(t, out, host)
	require.Contains(t, out, "alice")
	require.Contains(t, out, "hosts.json")
	require.NotContains(t, out, "top-secret")
	require.Contains(t, out, "Logged in to "+host+" account alice ("+filepath.Join(home, ".config", "fgh", "hosts.json")+")")
	t.Setenv("FGH_HOST", host)
	t.Chdir(t.TempDir())
	out, err = run(t, "auth", "status")
	require.NoError(t, err)
	require.Contains(t, out, "Logged in to "+host+" account alice ("+filepath.Join(home, ".config", "fgh", "hosts.json")+")")
	require.NotContains(t, out, "top-secret")
}

func TestAuthFailures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FGH_HOST", "example.invalid")
	t.Setenv("FGH_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("PATH", t.TempDir())
	_, err := run(t, "auth", "status")
	require.Error(t, err)
	require.Equal(t, 4, exitCode(err))
	cmd := newRootCmd("test")
	cmd.SetArgs([]string{"auth", "login", "--with-token"})
	cmd.SetIn(bytes.NewReader(nil))
	require.Error(t, cmd.Execute())
	_, err = run(t, "auth", "login")
	require.ErrorContains(t, err, "not implemented")
}

func TestAuthInvalidTokenIsPrivate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "top-secret")
	}))
	defer server.Close()
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("FGH_TOKEN", "top-secret")
	out, err := run(t, "auth", "status", "--hostname", strings.TrimPrefix(server.URL, "https://"))
	require.Error(t, err)
	require.Equal(t, 4, exitCode(err))
	require.NotContains(t, out, "top-secret")
	require.NotContains(t, err.Error(), "top-secret")
}
