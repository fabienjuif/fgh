//go:build integration

package forgejotest_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fabienjuif/fgh/internal/forgejotest"
)

func TestSeededInstance(t *testing.T) {
	in := forgejotest.Start(t)
	require.NotZero(t, in.BranchPR)
	require.NotZero(t, in.AGitPR)
	require.NotEqual(t, in.BranchPR, in.AGitPR)

	get := func(n int) map[string]any {
		req, _ := http.NewRequest(http.MethodGet, in.URL+"/api/v1/repos/"+in.Repo+"/pulls/"+strconv.Itoa(n), nil)
		req.Header.Set("Authorization", "token "+in.Token)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, 200, resp.StatusCode)
		var m map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
		return m
	}
	require.Equal(t, "open", get(in.BranchPR)["state"])
	agit := get(in.AGitPR)
	require.Equal(t, "open", agit["state"])
	require.Equal(t, forgejotest.AGitTitle, agit["title"])
	require.NotEmpty(t, agit["head"].(map[string]any)["sha"])
	// An AGit pull request has no branch on the server: its head is refs/pull/<n>/head.
	t.Logf("agit head: %v", agit["head"])
	require.NotEqual(t, in.BranchHead, agit["head"].(map[string]any)["ref"])
}
