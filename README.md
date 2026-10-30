# fgh

`gh`'s command interface for [Forgejo](https://forgejo.org): the same commands, flags, `--json` field
names and `--jq` filters, so scripts and tools written for `gh` run against a Forgejo instance
unchanged. It is a Go library first and a CLI on top.

```console
$ fgh pr view 42 --json number,state,headRefName --jq '.state + " " + .headRefName'
MERGED fix/login-redirect

$ fgh pr merge 42 --squash --match-head-commit "$sha" --subject "Fix login redirect" --body ""
```

## Why

Forgejo has no official CLI, and `gh` only speaks to GitHub. `fgh` ports the part of `gh` that
automation relies on and leaves the rest out on purpose: a command that is not ported fails with a
non-zero exit code, it never half-works.

## Install

Requires Go 1.26 or later.

```sh
go install github.com/fabienjuif/fgh/cmd/fgh@latest
```

Or from a checkout:

```sh
git clone https://github.com/fabienjuif/fgh && cd fgh
make build        # writes bin/fgh
```

## Authentication

The host and repository come from the `origin` remote, or from `-R owner/repo` (with `FGH_HOST` set
outside a git checkout). The token is looked up in this order:

1. `FGH_TOKEN`, then `FORGEJO_TOKEN` (environment)
2. `~/.config/fgh/hosts.json`, written by `fgh auth login --with-token`
3. `git credential fill`

```sh
echo "$TOKEN" | fgh auth login --with-token
fgh auth status
```

## Commands

| Command | Notes |
| --- | --- |
| `fgh api <endpoint>` | `-X`, `-f/-F` (including `@file` and `@-`), `-H`, `--input`, `--paginate`, `-q/--jq` |
| `fgh pr view \| list \| diff` | by number, URL or current branch; `--json` and `--jq` with gh's field names |
| `fgh pr create` | `--title`, `--body`, `--body-file`, `--base`, `--head`, `--assignee` |
| `fgh pr comment` | `--body`, `--body-file` (`-` for stdin), `--edit-last` |
| `fgh pr merge` | `--squash`, `--merge`, `--rebase`, `--subject`, `--body`, `--match-head-commit`, `--delete-branch` |
| `fgh search prs` | `--involves @me`, `--review-requested @me`, `--author`, `--state`, `-L` |
| `fgh repo view` | `--json` with gh's fields |
| `fgh auth login \| status` | |

Not ported (`pr checks`, `issue`, `--admin`, `--auto`, ...) exits with an error. Exit codes follow
`gh`: 1 for an error, 4 for an authentication failure.

## Where Forgejo differs from GitHub

- **Pinned merges.** `--match-head-commit` is sent as `head_commit_id`; Forgejo refuses the merge if
  the head moved since you read it.
- **AGit pull requests** (opened with `git push origin HEAD:refs/for/main`) have no branch.
  `pr merge --delete-branch` merges and prints a note rather than asking Forgejo to delete one,
  which would make it answer HTTP 500 after the merge has landed.
- **Mergeability rechecks.** Right after another merge, Forgejo answers 405 for a few seconds.
  `pr merge` retries for up to a minute, but fails at once on a pull request that is already merged,
  closed or a draft.
- **`headRefName`** stays the branch name after the branch is deleted, although Forgejo replaces
  `head.ref` with `refs/pull/N/head` at that point.
- **`--involves @me`** is the union of created, assigned, mentioned and review-requested, since
  Forgejo has no such filter.

## As a library

`forgejo`, `remote` and `auth` import only the standard library, so a Go program can embed them
without pulling in the CLI's dependencies:

```go
import "github.com/fabienjuif/fgh/forgejo"

client := forgejo.New("https://git.example.com", token)
pr, err := client.Pulls.Get(ctx, "owner", "repo", 42)
_, err = client.Pulls.Merge(ctx, "owner", "repo", 42, forgejo.MergeOptions{Method: "squash", HeadSHA: pr.Head.SHA})
```

gh's JSON shape lives only in `cmd/fgh` and `internal/ghjson`.

## Development

```sh
make ci                # lint and unit tests, what a pull request must pass
make test-integration  # a real Forgejo 16 in podman; skipped without podman
make fix               # format and autofix
```

Tools are pinned and run through Go: nothing to install but Go. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
