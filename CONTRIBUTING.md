# Contributing to loxicmd-inference-gateway

Thanks for your interest in improving loxicmd-inference-gateway. This document
describes how to set up a development environment and the conventions we follow.

## Getting started

1. Fork the repository and clone your fork.
2. Install Go 1.25+ and `make`. The CLI itself targets Linux (it uses netlink
   APIs for local network inspection); on macOS, cross-check with
   `GOOS=linux go build ./...`.
3. Build and run the tests:

   ```bash
   make build
   make test
   ```

For questions, check the existing issues, or reach the developers at
[loxilb-devel@netlox.io](mailto:loxilb-devel@netlox.io), the
[loxilb forum](https://www.loxilb.io/forum), or the community
[Slack](https://www.loxilb.io/members).

## Development workflow

- Create a topic branch off `main` (e.g. `fix/token-perms`, `feat/lb-flag`).
- Keep changes focused; unrelated cleanups belong in separate PRs.
- Ensure the following pass before opening a PR:

  ```bash
  gofmt -l .          # must print nothing — CI fails on any unformatted file
  go build ./...
  go vet ./...
  make test
  golangci-lint run   # config in .golangci.yml
  scripts/release-hygiene.sh   # repository hygiene gate (also blocking in CI)
  ```

- New commands and flags must trace to a real field in the gateway's
  `api/swagger.yml` / `api/swagger-extras.yml` or a `cicd/` scenario payload —
  no invented API surface. Where the gateway's control plane accepts
  configuration its data plane does not yet enforce, say so in the help text.
- Update or add tests for any behavior change.
- Update documentation (`README.md`, `docs/COMMANDS.md`, `docs/QUICKSTART.md`)
  when you change commands, flags, or defaults.

## Coding conventions

- Follow standard Go style; run `gofmt`/`goimports`.
- Prefer clear, idiomatic Go doc comments over verbose narration.
- Never commit secrets or internal hostnames. Use documentation IP ranges
  (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`) and
  `<your-...>` placeholders in examples.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/), e.g.:

```
feat(lb): add --kv-warmup flag for KV-cache-aware routing
fix(api): trim whitespace when reading the token file
docs(commands): document the snapshot/restore lifecycle
```

## Sign your commits (DCO)

We require a [Developer Certificate of Origin (DCO)](https://developercertificate.org/) sign-off on
every commit. The sign-off certifies that you wrote the change or otherwise have the right to submit it
under the project's license.

Add a `Signed-off-by` line to each commit — it must match the git author name and email:

```
Signed-off-by: Your Name <your.name@example.com>
```

Git adds it automatically with the `-s` flag:

```bash
git commit -s -m "feat(lb): add --kv-warmup flag"
```

If you forgot on an unpushed commit, amend it with `git commit --amend -s`. PRs whose commits are not
signed off will be blocked by the DCO check.

## Pull request policy

All changes land through pull requests — direct pushes to `main` are disabled.

### Opening a PR

- Push your topic branch to your fork and open a PR against
  `loxilb-io/loxicmd-inference-gateway` `main`.
- Fill in the PR template: describe the motivation and the change, and link any related issue
  (e.g. `Closes #123`).
- Give the PR a [Conventional Commits](https://www.conventionalcommits.org/) style title — it becomes
  the squash-merge commit message.
- Keep the PR focused and reasonably small; unrelated changes belong in separate PRs.

### Requirements to merge

- **At least one approving review from a maintainer is required.** Maintainers are the code
  owners in [.github/CODEOWNERS](.github/CODEOWNERS); GitHub requests their review automatically, and
  branch protection blocks the merge until a maintainer approves.
- All CI checks pass — build, `go vet`, `gofmt`, tests, `golangci-lint`, the hygiene gate, the
  secret scan, and the DCO check.
- All commits are signed off (DCO) — see [Sign your commits](#sign-your-commits-dco).
- New or changed behavior is covered by tests, and docs are updated where relevant.
- The branch is up to date with `main` and all review threads are resolved.

Project roles and how maintainer decisions are made are described in [GOVERNANCE.md](GOVERNANCE.md).

### Review process

- A maintainer will review as soon as they can and may request changes; please be responsive.
- Address feedback with follow-up commits. Avoid force-pushing once a review has started so reviewers
  can follow incremental changes — the PR is squash-merged at the end, so intermediate commits don't
  matter.
- Authors cannot approve or merge their own PRs. Once it has a maintainer approval and green CI, a
  maintainer merges it via squash.
- PRs with no author activity for an extended period may be marked stale; reopen when you're ready to
  continue.

## Code of Conduct

This project follows our [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to
uphold it.

## Security

Do **not** open public issues for security vulnerabilities. Follow the process in
[SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE).
