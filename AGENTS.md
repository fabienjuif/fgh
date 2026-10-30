# fgh

gh's command interface (`pr`, `api`, `--json`, `--jq`) over the Forgejo API: a Go library first,
a CLI on top.

## Where things are

- `cmd/fgh`: the CLI (cobra). `forgejo`, `remote` and `auth` are the library; `internal/ghjson` maps
  Forgejo to gh's JSON fields.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) has the scope and the workflow.

## Commands

- `make ci`: lint and unit tests, what a pull request must pass. Tools are pinned and run through Go:
  nothing to install but Go.
- `make fix`, `make test`, `make build` (`bin/fgh`, version from `git describe`),
  `make test-integration` (a real Forgejo in podman).

## Conventions

- Commits: `:emoji: <theme> / <what>`, one line under 60 characters, no body, no `Co-Authored-By`.
  Pull requests are squash-merged and the PR title is the commit, so it follows the same format.
- Go: `slog` with the `err` key; testify `require`; in a file, the most-used exported symbols first,
  a constructor next to its type, unexported helpers last.
- The library packages import only the standard library: cobra, gojq and gh's JSON shape stay in
  `cmd/fgh` and `internal/ghjson`.
- Comments say why (a decision, a gotcha, what was tried), never what the code does.
- Never write the em dash character (U+2014): code, comments, docs, commits, PRs.
