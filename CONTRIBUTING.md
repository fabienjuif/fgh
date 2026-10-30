# Contributing

Thanks for helping. `fgh` ports `gh`'s interface to Forgejo, so most changes are either a new
command that scripts already call, or a fix where Forgejo behaves differently from GitHub.

## Scope

- Copy `gh`'s interface: command names, flags, `--json` field names, `--jq`. Port a command only when
  something real calls it.
- A command that is not ported must fail with a non-zero exit code, never half-work.
- `fgh` is Forgejo-only. Choosing between `gh` and `fgh` from the git remote belongs outside this repo.
- Open an issue before a large change, especially a new command.

## Setup

Go 1.26 or later. Linters are pinned and run through Go, so nothing else is needed.

```sh
make ci                # lint and unit tests; a pull request must pass it
make fix               # format and autofix
make test-integration  # a real Forgejo 16 in podman, seeded through its API; skipped without podman
```

Merge pinning, AGit pull requests and the mergeability rechecks only show up against a real Forgejo,
so run `make test-integration` when you touch `Pulls.Merge`, the client, or anything that talks to
the forge in a new way.

## Code

- `forgejo`, `remote` and `auth` import only the standard library. cobra, gojq and gh's JSON shape
  stay in `cmd/fgh` and `internal/ghjson`.
- Logging uses `slog` with the `err` key. Tests use testify `require`.
- In a file, the most-used exported symbols come first, a constructor next to its type, unexported
  helpers last.
- Comments say why (a decision, a gotcha, something that was tried), never what the code does.
- Unit tests use an `httptest` server answering recorded Forgejo JSON. A test should fail when the
  behaviour it protects changes, not only when the code does.

## Commits and pull requests

- Commit subjects look like `:emoji: theme / what`, one line under 60 characters, no body.
- Pull requests are squash-merged and the PR title becomes the commit, so it follows the same format.
- Keep a pull request to one change. Describe what you ran: `make ci`, and `make test-integration`
  when it applies.
