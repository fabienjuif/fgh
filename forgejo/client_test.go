package forgejo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeAndDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "token secret", r.Header.Get("Authorization"))
		assert.Equal(t, "/api/v1/user", r.URL.Path)
		fmt.Fprint(w, `{"id":1,"login":"alice"}`)
	}))
	defer server.Close()
	user, err := New(server.URL, "secret").Users.Me(context.Background())
	require.NoError(t, err)
	require.Equal(t, "alice", user.Login)
}

func TestErrors(t *testing.T) {
	for code, expected := range map[int]error{404: ErrNotFound, 409: ErrConflict, 405: ErrRecheck} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code); fmt.Fprint(w, "secret") }))
			defer server.Close()
			_, err := New(server.URL, "secret").Do(context.Background(), http.MethodPost, "merge", map[string]string{"x": "y"}, nil)
			require.ErrorIs(t, err, expected)
			require.NotContains(t, err.Error(), "secret")
			var typed *Error
			require.ErrorAs(t, err, &typed)
			require.Equal(t, code, typed.StatusCode)
		})
	}
}

func TestPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/items", r.URL.Path)
		w.Header().Set("X-Total-Count", "2")
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `</api/v1/items?page=2>; rel="next", </api/v1/items?page=2>; rel="last"`)
		}
		fmt.Fprint(w, `[{"id":1}]`)
	}))
	defer server.Close()
	pages := 0
	err := New(server.URL, "").Paginate(context.Background(), "items", func(page json.RawMessage, meta *Response) error {
		pages++
		require.JSONEq(t, `[{"id":1}]`, string(page))
		require.Equal(t, 2, meta.TotalCount)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, pages)
}

func TestRejectForeignLinkAndRedirect(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("foreign host contacted") }))
	defer foreign.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, foreign.URL, http.StatusFound) }))
	defer server.Close()
	client := New(server.URL, "secret")
	_, err := client.Do(context.Background(), http.MethodGet, foreign.URL, nil, nil)
	require.Error(t, err)
	_, err = client.Do(context.Background(), http.MethodGet, "redirect", nil, nil)
	require.Error(t, err)
}

func TestBodyCancellationAndCycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]string
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "value", body["key"])
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Link", `</api/v1/items>; rel="next"`)
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()
	client := New(server.URL, "")
	_, err := client.Do(context.Background(), http.MethodPost, "items", map[string]string{"key": "value"}, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Do(ctx, http.MethodGet, "items", nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	err = client.Paginate(context.Background(), "items", func(json.RawMessage, *Response) error { return nil })
	require.ErrorContains(t, err, "cycle")
}
