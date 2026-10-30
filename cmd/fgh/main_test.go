package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd("v1.2.3")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// The version line tells a script or the gh proxy which fgh answered.
func TestVersionPrintsTheBuildVersion(t *testing.T) {
	out, err := run(t, "--version")
	require.NoError(t, err)
	require.Equal(t, "fgh version v1.2.3\n", out)
}

// A gh command fgh does not port must fail, never pass for a success that did nothing.
func TestUnportedCommandFails(t *testing.T) {
	out, err := run(t, "release", "view", "1")
	require.EqualError(t, err, `unknown command "release" for "fgh"`)
	require.Contains(t, out, `unknown command "release" for "fgh"`)
}

// The same holds one level down: `pr checks` is a gh command the skills call and fgh does not port.
func TestUnportedSubcommandFails(t *testing.T) {
	for _, group := range []string{"auth", "pr", "repo", "search"} {
		_, err := run(t, group, "checks", "1")
		require.EqualError(t, err, `unknown command "checks" for "fgh `+group+`"`, group)
	}
}
