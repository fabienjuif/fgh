package forgejo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	for _, agit := range []bool{false, true} {
		t.Run(map[bool]string{false: "branch", true: "agit"}[agit], func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					flow := 0
					if agit {
						flow = FlowAGit
					}
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"flow": flow, "state": "open"}))
					return
				}
				assert.Equal(t, "/api/v1/repos/o/r/pulls/7/merge", r.URL.Path)
				var payload map[string]any
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				assert.Equal(t, "squash", payload["Do"])
				assert.Equal(t, "subject", payload["MergeTitleField"])
				assert.Contains(t, payload, "MergeMessageField")
				assert.Empty(t, payload["MergeMessageField"])
				assert.Equal(t, "pinned", payload["head_commit_id"])
				if agit {
					assert.NotContains(t, payload, "delete_branch_after_merge")
				} else {
					assert.Equal(t, true, payload["delete_branch_after_merge"])
				}
				posts++
				if posts == 1 {
					w.WriteHeader(http.StatusMethodNotAllowed)
					_, _ = w.Write([]byte(`{"message":"rechecking"}`))
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			subject, body := "subject", ""
			noBranch, err := New(server.URL, "").Pulls.Merge(context.Background(), "o", "r", 7, MergeOptions{Method: "squash", Title: &subject, Message: &body, HeadSHA: "pinned", DeleteBranch: true})
			require.NoError(t, err)
			require.Equal(t, agit, noBranch)
			require.Equal(t, 2, posts)
		})
	}
}

func TestMergeErrors(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusMethodNotAllowed, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"forge reason"}`))
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_, err := New(server.URL, "").Pulls.Merge(ctx, "o", "r", 1, MergeOptions{Method: "merge"})
			if status == http.StatusMethodNotAllowed {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				require.ErrorContains(t, err, "forge reason")
			}
			require.Equal(t, 1, posts)
		})
	}
}

// Forgejo answers 405 with an empty message for a pull request that no retry will merge. Waiting
// the whole window on a merged one made the merge skill hang for a minute on a stale PR number.
func TestMergeFailsFastOnAPullRequestThatCannotMerge(t *testing.T) {
	for name, tc := range map[string]struct {
		pr   string
		want string
	}{
		"merged": {`{"state":"closed","merged":true}`, "already merged"},
		"closed": {`{"state":"closed"}`, "is closed"},
		"draft":  {`{"state":"open","draft":true}`, "is a draft"},
	} {
		t.Run(name, func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(tc.pr))
					return
				}
				posts++
				w.WriteHeader(http.StatusMethodNotAllowed)
				_, _ = w.Write([]byte(`{"message":""}`))
			}))
			defer server.Close()
			start := time.Now()
			_, err := New(server.URL, "").Pulls.Merge(context.Background(), "o", "r", 4, MergeOptions{Method: "squash"})
			require.ErrorContains(t, err, tc.want)
			require.Equal(t, 1, posts)
			require.Less(t, time.Since(start), 500*time.Millisecond)
		})
	}
}

// An open pull request blocked for a reason the forge does not give still ends, with a hint.
func TestMergeExplainsAnEmptyBlock(t *testing.T) {
	old := recheckWindow
	recheckWindow = 100 * time.Millisecond
	defer func() { recheckWindow = old }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"state":"open"}`))
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":""}`))
	}))
	defer server.Close()
	_, err := New(server.URL, "").Pulls.Merge(context.Background(), "o", "r", 4, MergeOptions{Method: "squash"})
	require.ErrorIs(t, err, ErrRecheck)
	require.ErrorContains(t, err, "the forge gave no reason")
}
