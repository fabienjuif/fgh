package ghjson

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fabienjuif/fgh/forgejo"
)

func load(t *testing.T, name string) *forgejo.PullRequest {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	var pr forgejo.PullRequest
	require.NoError(t, json.Unmarshal(data, &pr))
	return &pr
}

func TestEveryFieldOfBranchPR(t *testing.T) {
	got, err := PR(load(t, "pr_branch.json"), PRFields())
	require.NoError(t, err)
	data, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"number":7,"title":"Add feature","body":"Does a thing.","url":"https://forge.test/o/r/pulls/7",
		"state":"OPEN","isDraft":false,"headRefName":"feat","headRefOid":"aaa111","baseRefName":"main","baseRefOid":"bbb222",
		"isCrossRepository":false,"mergeable":"MERGEABLE","mergedAt":null,"closedAt":null,
		"createdAt":"2026-10-01T10:00:00Z","updatedAt":"2026-10-02T09:30:00Z",
		"author":{"login":"alice"},"assignees":[{"login":"bob"}],"labels":[{"name":"bug"},{"name":"p1"}],
		"reviewRequests":[{"login":"carol"}],"additions":10,"deletions":2,"changedFiles":3,"mergeCommit":null}`, string(data))
	require.Len(t, PRFields(), 24)
}

func TestAGitPR(t *testing.T) {
	got, err := PR(load(t, "pr_agit.json"), []string{"headRefName", "mergeable", "isDraft", "assignees", "reviewRequests"})
	require.NoError(t, err)
	require.Equal(t, "my-topic", got["headRefName"])
	require.Equal(t, "UNKNOWN", got["mergeable"])
	require.Equal(t, true, got["isDraft"])
	require.Empty(t, got["assignees"])
	require.NotNil(t, got["assignees"]) // [] not null, like gh
}

func TestMergedPR(t *testing.T) {
	got, err := PR(load(t, "pr_merged.json"), []string{"state", "mergedAt", "closedAt", "mergeCommit", "isCrossRepository", "mergeable", "headRefName"})
	require.NoError(t, err)
	require.Equal(t, "MERGED", got["state"])
	require.Equal(t, "2026-10-04T09:00:00Z", got["mergedAt"])
	require.Equal(t, map[string]any{"oid": "ddd444"}, got["mergeCommit"])
	require.Equal(t, true, got["isCrossRepository"])
	require.Equal(t, "CONFLICTING", got["mergeable"])
	require.Equal(t, "fix", got["headRefName"])
}

func TestClosedUnmergedState(t *testing.T) {
	pr := load(t, "pr_branch.json")
	pr.State = "closed"
	require.Equal(t, "CLOSED", State(pr))
}

func TestUnknownFieldListsSupported(t *testing.T) {
	err := ParseFields([]string{"number", "nope"})
	require.ErrorContains(t, err, `Unknown JSON field: "nope"`)
	require.ErrorContains(t, err, "Available fields:")
	require.ErrorContains(t, err, "  headRefName")
	require.ErrorContains(t, ParseFields([]string{"reviews"}), "not supported on Forgejo")
}

func TestJq(t *testing.T) {
	got, err := PR(load(t, "pr_branch.json"), []string{"title", "labels"})
	require.NoError(t, err)
	lines, err := Jq(t.Context(), ".title, (.labels | map(.name))", got)
	require.NoError(t, err)
	require.Equal(t, []string{"Add feature", `["bug","p1"]`}, lines)
	_, err = Jq(t.Context(), "{{", got)
	require.Error(t, err)
}
