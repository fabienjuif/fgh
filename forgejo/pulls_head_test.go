package forgejo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Once the merge skill deletes the branch, Forgejo swaps head.ref for refs/pull/N/head; a script
// reading headRefName after the merge must still get the branch name.
func TestHeadBranch(t *testing.T) {
	for name, tc := range map[string]struct {
		flow       int
		label, ref string
		want       string
	}{
		"live branch":           {0, "feat", "feat", "feat"},
		"deleted branch":        {0, "woof/01a1", "refs/pull/13/head", "woof/01a1"},
		"deleted fork branch":   {0, "dave:fix", "refs/pull/9/head", "fix"},
		"agit topic":            {FlowAGit, "o/woof/01a1", "refs/pull/12/head", "woof/01a1"},
		"agit topic, no prefix": {FlowAGit, "topic", "refs/pull/3/head", "topic"},
	} {
		t.Run(name, func(t *testing.T) {
			pr := &PullRequest{Flow: tc.flow, Head: &Branch{Label: tc.label, Ref: tc.ref}}
			require.Equal(t, tc.want, pr.HeadBranch())
		})
	}
}
