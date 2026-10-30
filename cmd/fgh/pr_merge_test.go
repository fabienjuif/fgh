package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRMerge(t *testing.T) {
	for _, flow := range []int{0, 1} {
		t.Run(map[int]string{0: "branch", 1: "agit"}[flow], func(t *testing.T) {
			t.Chdir(t.TempDir())
			posts := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]int{"flow": flow}))
					return
				}
				posts++
				var payload map[string]any
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				assert.Equal(t, "squash", payload["Do"])
				assert.Equal(t, "chosen subject", payload["MergeTitleField"])
				assert.Contains(t, payload, "MergeMessageField")
				assert.Empty(t, payload["MergeMessageField"])
				assert.Equal(t, "abc", payload["head_commit_id"])
				if flow == 1 {
					assert.NotContains(t, payload, "delete_branch_after_merge")
				} else {
					assert.Equal(t, true, payload["delete_branch_after_merge"])
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			original := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			defer func() { http.DefaultTransport = original }()
			t.Setenv("FGH_TOKEN", "tok")
			t.Setenv("FGH_HOST", strings.TrimPrefix(server.URL, "https://"))
			_, err := run(t, "pr", "merge", "7", "-R", "o/r", "--squash", "--subject", "chosen subject", "--body", "", "--match-head-commit", "abc", "--delete-branch")
			require.NoError(t, err)
			require.Equal(t, 1, posts)
		})
	}
}

func TestPRMergeFlags(t *testing.T) {
	for _, flags := range [][]string{{"--admin"}, {"--auto"}, {"--squash", "--merge"}, {}} {
		_, err := run(t, append([]string{"pr", "merge", "7"}, flags...)...)
		require.Error(t, err)
	}
}
