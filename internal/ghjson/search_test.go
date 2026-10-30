package ghjson

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fabienjuif/fgh/forgejo"
)

func TestSearchMapping(t *testing.T) {
	var p forgejo.SearchResult
	require.NoError(t, json.Unmarshal([]byte(`{"number":7,"title":"Fix","body":"Body","state":"closed","user":{"login":"alice"},"assignees":[],"labels":[{"name":"bug"}],"comments":4,"html_url":"https://example/o/r/pulls/7","repository":{"name":"r","full_name":"o/r"},"pull_request":{"draft":true,"merged":true},"created_at":"2026-10-29T00:00:00Z","updated_at":"2026-10-30T00:00:00Z"}`), &p))
	got, err := Search(&p, SearchFields())
	require.NoError(t, err)
	require.Equal(t, "closed", got["state"])
	require.Equal(t, true, got["isDraft"])
	require.Equal(t, 4, got["commentsCount"])
	require.Equal(t, map[string]any{"login": "alice"}, got["author"])
	require.Equal(t, map[string]any{"name": "r", "nameWithOwner": "o/r"}, got["repository"])
	require.Equal(t, []map[string]any{}, got["assignees"])
	require.ErrorContains(t, ParseSearchFields([]string{"bogus"}), "Available fields:")
}
