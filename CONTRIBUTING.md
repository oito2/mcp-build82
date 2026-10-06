# Contributing to build82

🌐 **Language:** English · [Português](docs/pt-br/contribuindo.md)

Thanks for considering a contribution. This document covers the practical steps: how to build and
test the project, what's expected of a pull request, and where to ask questions.

By participating in this project you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Before you start

For anything beyond a small fix (new tools, new CLI commands, extractor/generator changes,
dependency additions), open an issue first to discuss the approach. This project has a **minimal,
justified dependencies** philosophy — see [Concepts — Architecture](docs/en/concepts/architecture.md)
and the `require` block in [`go.mod`](go.mod) for the current baseline (`go-sdk`, `fsnotify`, and
the optional pure-Go `gotreesitter` backend — no cgo anywhere, which keeps `CGO_ENABLED=0`
cross-compilation in [`scripts/release`](scripts/release) working). A PR that adds a new dependency
without prior discussion is likely to be asked to remove it.

## Development setup

Requirements: a Go toolchain matching the version in [`go.mod`](go.mod) (Go 1.26+ — the project
targets the latest stable release; the MCP SDK's `Server.Sessions()` uses range-over-func
iterators, which require at least Go 1.23).

```bash
git clone https://github.com/oito2/mcp-build82
cd mcp-build82
go build -o build82 ./cmd/build82   # the binary; entry point is cmd/build82
go test ./...
```

The module path is `github.com/oito2/mcp-build82`.

Run the same checks CI runs before opening a PR:

```bash
gofmt -l .        # must print nothing
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
go build ./...
go test -race ./...
BUILD82_EXTRACTOR_BACKEND=treesitter go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

All seven must be clean (the test suite runs once per extractor backend) — [`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs them (on the newest Go 1.26.x patch, `ubuntu-24.04`) on every
push and pull request against `main` and will block merge otherwise. `golangci-lint` uses the
repository's [`.golangci.yml`](.golangci.yml) (default linters, with the standard exclusions for
unchecked `Close`/`fmt.Fprint*` errors) and must report `0 issues`; CI pins the same version. The `-race` flag needs cgo, i.e. a working C compiler on your machine; without one, use plain `go test ./...` locally and rely on CI for the race run. Tests exercise real behavior
wherever practical — real `fsnotify` events for the watcher, a real in-memory MCP client/server
transport pair for tools, real compiled binaries for install/self-update/CLI-layer checks — rather
than mocking the SDK or the filesystem.

## Project layout

- `cmd/build82/` — entry point; argv dispatch only.
- `internal/extractors/` — pure PHP/XML parsing, no MCP dependency. Includes the opt-in
  `tsbackend` (tree-sitter) subpackage alongside the default regex backend.
- `internal/generators/` — Markdown output, legacy-file migration, persistent-cache wiring.
- `internal/tools/`, `internal/resources/`, `internal/prompts/` — the MCP-facing layer.
- `internal/server/` — wires everything into one `*mcp.Server`.
- `internal/installer/` — `install`/`uninstall` target tables.
- `internal/selfupdate/` — `self-update`.
- `internal/config/`, `internal/cache/`, `internal/watcher/` — foundational, no MCP dependency.

## Making a change

1. Fork the repository and create a branch from `main`.
2. Keep the change focused — one logical change per PR. Unrelated cleanups make review slower, not
   faster.
3. Match existing code style and package layout (see [Project layout](#project-layout) above and
   [Concepts — Architecture](docs/en/concepts/architecture.md)).
4. Add or update tests for the behavior you changed. This project relies on `go test -race ./...`
   as the safety net — untested behavior is assumed broken.
5. Update the relevant docs under [`docs/en/`](docs/en/) and its [`docs/pt-br/`](docs/pt-br/)
   counterpart (see [Documentation](#documentation) below) if you changed a tool's parameters, a
   CLI command, or the architecture.
6. Run the checks in [Development setup](#development-setup) locally.

## Commit messages

Write a concise summary line explaining *why* the change was made, not just what changed — the
diff already shows what changed. Keep related changes in a single commit rather than a string of
"fix" follow-ups.

## Documentation

Root-level documents in this repository (README, CONTRIBUTING, CODE_OF_CONDUCT) ship in English
(canonical, at the root) and Portuguese (same name, lowercased and translated, inside
[`docs/pt-br/`](docs/pt-br/) — `leiame.md`, `contribuindo.md`, `codigo-de-conduta.md` — kept in
sync); the `docs/` reference site is split into parallel [`docs/en/`](docs/en/) and
[`docs/pt-br/`](docs/pt-br/) trees the same way. If your change affects behavior described in the
[Tools Reference](docs/en/reference/tools.md), [Generated Files Reference](docs/en/reference/generated-files.md),
[Installation Guide](docs/en/getting-started/installation.md), [Architecture](docs/en/concepts/architecture.md),
or the README, update both language versions in the same PR — a PR that updates only one will be
asked to add the other.

## Pull requests

- Describe what changed and why in the PR description; link the issue it addresses if one exists.
- Keep the PR scoped to the discussed change — large unsolicited refactors are likely to be
  declined even if the code itself is fine, per this project's preference for minimal, precise
  changes.
- A maintainer will review, request changes if needed, and merge once CI is green and the
  discussion is resolved.
- [Dependabot](.github/dependabot.yml) opens weekly pull requests that update the GitHub Actions
  used by the workflows; they go through the same CI checks.

## Reporting bugs and requesting features

Open a [GitHub issue](https://github.com/oito2/mcp-build82/issues) with:

- For bugs: what you ran (`build82` subcommand or MCP tool call), what you expected, what
  happened instead, and your `build82 --version` output.
- For features: the problem you're trying to solve, not just the solution you have in mind — see
  [Before you start](#before-you-start).

## Cutting a release

Releases are built with a small in-repo Go program rather than a third-party release tool, to
avoid an extra dependency for something this project's build is simple enough to do directly:

```bash
go run ./scripts/release vX.Y.Z
```

This cross-compiles `build82` for every supported `GOOS`/`GOARCH` (linux/amd64, linux/arm64,
darwin/amd64, darwin/arm64, windows/amd64) into `dist/`, with
`-ldflags "-s -w -X github.com/oito2/mcp-build82/internal/version.Current=vX.Y.Z"`: `-s -w`
strip the symbol table and DWARF debug info to shrink the binaries (panic stack traces are kept),
and `-X` makes the built binary report the right version (`build82 --version`) so `self-update`'s
semver comparison works correctly against it. It then packages `dist/build82.mcpb`, an [MCPB](https://github.com/modelcontextprotocol/mcpb)
desktop-extension bundle (manifest version 0.3) holding a universal macOS binary (the two darwin
builds merged into one Mach-O by the script itself, since MCPB picks a binary per OS but not per
CPU architecture), both Linux binaries behind a launcher script
([`scripts/release/build82-linux.sh`](scripts/release/build82-linux.sh), which execs the one
matching `uname -m`), the windows/amd64 binary, and the icons from `docs/img/icons/`. The
manifest's tool list is read from the server itself. Finally it writes `dist/checksums.txt` (standard `sha256sum` format, covering every binary
and the bundle) and `dist/server.json`, the [MCP Registry](https://modelcontextprotocol.io/registry/)
descriptor (schema `2025-12-11`) that points at the bundle. `server.json` embeds the bundle's
SHA-256, so it is not versioned in the repository (it is listed in `.gitignore`): the only valid
copy is the one generated next to the bundle it describes and published as a release asset.

Release assets are named `build82_<os>_<arch>` (`.exe` on Windows), plus `build82.mcpb`. Then either:

1. **Automatically (normal path):** push a `vX.Y.Z` tag (`git tag vX.Y.Z && git push --tags`).
   [`release.yml`](.github/workflows/release.yml) triggers on tags matching `v*.*.*` and runs two
   jobs:
   - `release`: sets up the newest Go 1.26.x patch and runs the same checks as CI (`gofmt`, `go vet`, `golangci-lint`, `go build`,
     `go test -race` with both the regex and the tree-sitter backend, `govulncheck`), runs
     `go run ./scripts/release <tag>`, validates `dist/server.json` with
     `mcp-publisher validate` (any check failing stops the release), and creates the GitHub
     release with `gh release create`, attaching everything in `dist/` (binaries,
     `build82.mcpb`, `checksums.txt` and `server.json`).
   - `publish-registry`: after `release` succeeds, downloads the `server.json` attached to the
     release and publishes it to the MCP Registry (see
     [Publishing to the MCP Registry](#publishing-to-the-mcp-registry)).
2. **Manually:** tag the release, then create the GitHub release yourself, uploading every file in
   `dist/` (including `build82.mcpb`, `checksums.txt` and `server.json`), and publish to the MCP
   Registry by hand as described below.

A binary built without the `-ldflags` above reports `"dev"` as its version — `self-update` treats
that as always-outdated, which is correct for a local dev build but means release binaries **must**
go through `scripts/release`, not a plain `go build`.

### Publishing to the MCP Registry

The `publish-registry` job of [`release.yml`](.github/workflows/release.yml) publishes every tagged
release automatically, with the official
[`mcp-publisher`](https://modelcontextprotocol.io/registry/github-actions) CLI (version pinned in
the workflow's `MCP_PUBLISHER_VERSION`) authenticated through GitHub Actions OIDC
(`mcp-publisher login github-oidc`, `id-token: write` permission, no secret needed). It runs only
after the release exists, because the registry entry points at the release's `build82.mcpb` asset
and at the tagged icons. If it fails, re-run just that job from the workflow run page.

To publish by hand instead (for example after a manual release), use the `oito2` GitHub account,
which the `io.github.oito2/` namespace requires:

```bash
gh release download vX.Y.Z --pattern server.json --dir /tmp/build82-vX.Y.Z
cd /tmp/build82-vX.Y.Z
mcp-publisher login github
mcp-publisher publish
```

`mcp-publisher publish` reads `server.json` from the current directory. Publish the copy attached
to the release, not a locally rebuilt one: a local build is not byte-identical to the CI build, so
its `fileSha256` would not match the published bundle. Check the result with
`curl "https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.oito2/mcp-build82"`.

## License

By contributing, you agree that your contributions will be licensed under the
[GNU General Public License v3.0](LICENSE), the same license that covers the rest of the project.
