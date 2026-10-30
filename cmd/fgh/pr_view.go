package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/internal/ghjson"
	"github.com/fabienjuif/fgh/remote"
)

var prURLPath = regexp.MustCompile(`^/([^/]+)/([^/]+)/pulls?/(\d+)(?:/.*)?$`)

func newPRViewCmd() *cobra.Command {
	var fields []string
	var jq string
	cmd := &cobra.Command{
		Use:   "view [<number> | <url> | <branch>]",
		Short: "View a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("json") && len(fields) == 0 {
				return fmt.Errorf("Specify one or more comma-separated fields for `--json`:\n  %s", joinLines(ghjson.PRFields())) //nolint:staticcheck // gh's wording
			}
			if jq != "" && len(fields) == 0 {
				return errors.New("cannot use `--jq` without specifying `--json`")
			}
			if err := ghjson.ParseFields(fields); err != nil {
				return err
			}
			override, err := cmd.Flags().GetString("repo")
			if err != nil {
				return err
			}
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			repository, number, branch, err := resolvePRArg(cmd, arg, override)
			if err != nil {
				return err
			}
			credential, err := auth.Token(cmd.Context(), repository.Host)
			if err != nil {
				return &authError{err}
			}
			client := forgejo.New(repository.APIBase(), credential.Token)
			if number == 0 {
				number, err = findByBranch(cmd, client, repository, branch)
				if err != nil {
					return err
				}
			}
			pr, err := client.Pulls.Get(cmd.Context(), repository.Owner, repository.Repo, number)
			if err != nil {
				var httpErr *forgejo.Error
				if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden) {
					return &authError{err}
				}
				return err
			}
			out := cmd.OutOrStdout()
			if len(fields) == 0 {
				return printPR(out, pr)
			}
			value, err := ghjson.PR(pr, fields)
			if err != nil {
				return err
			}
			if jq != "" {
				lines, err := ghjson.Jq(cmd.Context(), jq, value)
				for _, l := range lines {
					_, _ = fmt.Fprintln(out, l)
				}
				return err
			}
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, string(data))
			return err
		},
	}
	cmd.Flags().StringSliceVar(&fields, "json", nil, "Output JSON with the specified `fields`")
	cmd.Flags().StringVarP(&jq, "jq", "q", "", "Filter JSON output using a jq `expression`")
	return cmd
}

// resolvePRArg returns the repository and either a number or a branch to look up.
func resolvePRArg(cmd *cobra.Command, arg, override string) (remote.Repository, int64, string, error) {
	if strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://") {
		u, err := url.Parse(arg)
		m := []string(nil)
		if err == nil {
			m = prURLPath.FindStringSubmatch(u.Path)
		}
		if m == nil {
			return remote.Repository{}, 0, "", fmt.Errorf("invalid pull request URL %q", arg)
		}
		n, _ := strconv.ParseInt(m[3], 10, 64)
		return remote.Repository{Host: u.Host, Owner: m[1], Repo: m[2]}, n, "", nil
	}
	repository, err := remote.Resolve(cmd.Context(), "", override)
	if err != nil {
		return repository, 0, "", err
	}
	if n, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64); err == nil && n > 0 {
		return repository, n, "", nil
	}
	if arg != "" {
		return repository, 0, arg, nil
	}
	out, err := exec.CommandContext(cmd.Context(), "git", "symbolic-ref", "--short", "HEAD").Output()
	branch := strings.TrimSpace(string(out))
	if err != nil || branch == "" || branch == "HEAD" {
		return repository, 0, "", errors.New("could not determine the current branch; pass a number, URL or branch")
	}
	return repository, 0, branch, nil
}

func findByBranch(cmd *cobra.Command, client *forgejo.Client, r remote.Repository, branch string) (int64, error) {
	prs, err := client.Pulls.List(cmd.Context(), r.Owner, r.Repo, "open")
	if err != nil {
		return 0, err
	}
	for i := range prs {
		if prs[i].HeadBranch() == branch {
			return prs[i].Number, nil
		}
	}
	return 0, fmt.Errorf("no pull requests found for branch %q", branch)
}

func printPR(out interface{ Write([]byte) (int, error) }, pr *forgejo.PullRequest) error {
	state := ghjson.State(pr)
	author := ""
	if pr.User != nil {
		author = pr.User.Login
	}
	base, head := "", pr.HeadBranch()
	if pr.Base != nil {
		base = pr.Base.Ref
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d\n", pr.Title, pr.Number)
	verb := "wants to merge"
	if state == "MERGED" {
		verb = "merged"
	}
	fmt.Fprintf(&b, "%s • %s %s into %s from %s\n", strings.ToUpper(state[:1])+strings.ToLower(state[1:]), author, verb, base, head)
	if pr.CreatedAt != nil {
		fmt.Fprintf(&b, "created %s\n", pr.CreatedAt.UTC().Format(time.RFC3339))
	}
	if strings.TrimSpace(pr.Body) != "" {
		fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(pr.Body))
	}
	fmt.Fprintf(&b, "\nView this pull request on Forgejo: %s\n", pr.HTMLURL)
	_, err := out.Write([]byte(b.String()))
	return err
}

func joinLines(s []string) string { return strings.Join(s, "\n  ") }
