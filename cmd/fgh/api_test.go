package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeForgejo points FGH_HOST at an httptest TLS server and returns the host.
func fakeForgejo(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	server := httptest.NewTLSServer(h)
	t.Cleanup(server.Close)
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FGH_HOST", strings.TrimPrefix(server.URL, "https://"))
	t.Setenv("FGH_TOKEN", "tok")
	t.Setenv("FORGEJO_TOKEN", "")
	t.Chdir(t.TempDir())
}

func runWith(stdin string, args ...string) (stdout, stderr string, err error) {
	cmd := newRootCmd("test")
	var o, e bytes.Buffer
	cmd.SetOut(&o)
	cmd.SetErr(&e)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err = cmd.Execute()
	return o.String(), e.String(), err
}

func TestAPIUserJQ(t *testing.T) {
	fakeForgejo(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/user", r.URL.Path)
		assert.Equal(t, "token tok", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"login":"alice","id":7}`)
	})
	out, _, err := runWith("", "api", "user", "-q", ".login")
	require.NoError(t, err)
	require.Equal(t, "alice\n", out)
	out, _, err = runWith("", "api", "user", "--jq", ".id")
	require.NoError(t, err)
	require.Equal(t, "7\n", out)
	out, _, err = runWith("", "api", "user")
	require.NoError(t, err)
	require.JSONEq(t, `{"login":"alice","id":7}`, out)
}

func TestAPIPatchBodyFromStdin(t *testing.T) {
	fakeForgejo(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v1/repos/o/r/issues/comments/1", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		data, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"body":"hello\nworld"}`, string(data))
		fmt.Fprint(w, `{}`)
	})
	_, _, err := runWith("hello\nworld", "api", "-X", "PATCH", "-R", "o/r", "repos/{owner}/{repo}/issues/comments/1", "-F", "body=@-")
	require.NoError(t, err)
}

func TestAPIFieldsTypesAndMethodDefault(t *testing.T) {
	fakeForgejo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			assert.Equal(t, "open", r.URL.Query().Get("state"))
			assert.Equal(t, "X", r.Header.Get("X-Test"))
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		data, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"n":3,"b":true,"s":"3","l":["a","b"],"o":{"k":"v"}}`, string(data))
	})
	_, _, err := runWith("", "api", "x", "-F", "n=3", "-F", "b=true", "-f", "s=3", "-f", "l[]=a", "-f", "l[]=b", "-f", "o[k]=v")
	require.NoError(t, err)
	_, _, err = runWith("", "api", "x", "-X", "GET", "-f", "state=open", "-H", "X-Test: X")
	require.NoError(t, err)
}

func TestAPIErrorStatus(t *testing.T) {
	fakeForgejo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"nope"}`)
	})
	out, errOut, err := runWith("", "api", "missing")
	require.EqualError(t, err, "HTTP 404")
	require.Equal(t, 1, exitCode(err))
	require.Empty(t, out)
	require.Contains(t, errOut, `{"message":"nope"}`)
}

func TestAPIUnauthorizedExitsOne(t *testing.T) {
	fakeForgejo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, _, err := runWith("", "api", "user")
	require.EqualError(t, err, "HTTP 401")
	require.Equal(t, 1, exitCode(err))
}

func TestAPIPaginate(t *testing.T) {
	fakeForgejo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"n":2}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<https://%s/api/v1/things?page=2>; rel="next"`, r.Host))
		fmt.Fprint(w, `[{"n":1}]`)
	})
	out, _, err := runWith("", "api", "things", "--paginate", "-q", ".[].n")
	require.NoError(t, err)
	require.Equal(t, "1\n2\n", out)
}

func TestAPIInput(t *testing.T) {
	fakeForgejo(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "1", r.URL.Query().Get("q"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		data, _ := io.ReadAll(r.Body)
		assert.Equal(t, `{"raw":1}`, string(data))
	})
	_, _, err := runWith(`{"raw":1}`, "api", "x", "--input", "-", "-f", "q=1")
	require.NoError(t, err)
}
