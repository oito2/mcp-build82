# build82

<p align="center">
  <img src="docs/img/github-header.png" alt="build82 by OITO2 Labs — The MCP for Moodle Developers" width="100%">
</p>

[![CI](https://github.com/oito2/mcp-build82/actions/workflows/ci.yml/badge.svg)](https://github.com/oito2/mcp-build82/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/oito2/mcp-build82.svg)](https://pkg.go.dev/github.com/oito2/mcp-build82)
[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/github/v/release/oito2/mcp-build82?sort=semver)](https://github.com/oito2/mcp-build82/releases)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Code: AI-Assisted](https://img.shields.io/badge/Code-AI--Assisted-blueviolet)](#ai-usage-in-this-project)

🌐 **Language:** English · [Português](docs/pt-br/leiame.md)

An MCP (Model Context Protocol) server that gives AI assistants structural understanding of a
Moodle installation and its plugins, so they can answer questions about the codebase and build
plugins that follow its real conventions. Distributed as a single static binary.

## Table of Contents

- [Overview](#overview)
- [Prerequisites & Quick Installation](#prerequisites--quick-installation)
- [Client Setup](#client-setup)
- [Update & Maintenance](#update--maintenance)
- [Documentation](#documentation)
- [AI Usage in This Project](#ai-usage-in-this-project)
- [License](#license)

## Overview

Point build82 at a Moodle installation and it scans the codebase (version.php, `db/*.php`, XMLDB
schemas, PHPDoc, hooks, tasks, capabilities, web services...) and generates a set of Markdown
context files describing the installation and any plugin you're developing. An AI assistant
connected to it can then answer questions about the codebase, scaffold new plugins consistently
with existing conventions, and keep the docs in sync as you work.

**Main features:**

- **13 global indexes** of the installation — API index, DB tables, events, tasks, capabilities,
  classes, web services, the plugin index and more.
- **12 per-plugin context files** — architecture, dependencies, runtime flow, settings, and an
  AI-ready summary.
- **13 MCP tools, 3 MCP prompts and MCP resources** — generate context, search the API and
  plugins, explain a plugin, diagnose the environment (`doctor`), package a plugin for release,
  and create a new plugin skeleton.
- **File watcher** that regenerates a dev plugin's context whenever its sources change.
- **Two PHP extraction backends** — fast regex (default) or pure-Go tree-sitter.
- **Clean output** — everything is written under `.build82/` (at the Moodle root and at each
  plugin's root), never loose in the Moodle or plugin root.
- **Native client registration** for 8 MCP clients, and a checksum-verified **self-update** with
  rollback.

## Prerequisites & Quick Installation

**Prerequisites:**

- A local Moodle installation (build82 reads its source tree).
- An MCP client — see [Client Setup](#client-setup).
- Optional: [`universal-ctags`](https://github.com/universal-ctags/ctags) on `PATH`, to also
  generate a `.build82/tags` file for editor navigation.
- Only to build from source: Go 1.26+.

**Download a release binary** (no Go toolchain required). Every release ships a `checksums.txt` —
verify it before trusting a downloaded binary. From v1.1.0 `checksums.txt` is also signed with
Sigstore and every binary carries a GitHub build provenance attestation
([how to verify](docs/en/getting-started/installation.md#-verifying-the-signature-and-provenance-optional)).
Full step-by-step guide with checksum verification and troubleshooting:
[Installation Guide](docs/en/getting-started/installation.md).

<details>
<summary><strong>🐧 Linux</strong></summary>

```bash
# amd64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_amd64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
sha256sum -c checksums.txt --ignore-missing \
  && chmod +x build82_linux_amd64 \
  && sudo mv build82_linux_amd64 /usr/local/bin/build82

# arm64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_arm64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
sha256sum -c checksums.txt --ignore-missing \
  && chmod +x build82_linux_arm64 \
  && sudo mv build82_linux_arm64 /usr/local/bin/build82

build82 --version
```

</details>

<details>
<summary><strong>🍎 macOS</strong></summary>

Check your chip with `uname -m` (`arm64` = Apple Silicon, `x86_64` = Intel):

```bash
# Apple Silicon (M1/M2/M3/M4)
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_darwin_arm64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing \
  && chmod +x build82_darwin_arm64 \
  && sudo mv build82_darwin_arm64 /usr/local/bin/build82

# Intel
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_darwin_amd64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing \
  && chmod +x build82_darwin_amd64 \
  && sudo mv build82_darwin_amd64 /usr/local/bin/build82
```

The binary isn't notarized by Apple, so Gatekeeper will refuse to run it the first time — clear
the quarantine flag: `xattr -d com.apple.quarantine /usr/local/bin/build82`.

</details>

<details>
<summary><strong>🪟 Windows</strong></summary>

On Windows on Arm, use `build82_windows_arm64.exe` in every command below.

```powershell
Invoke-WebRequest -Uri "https://github.com/oito2/mcp-build82/releases/latest/download/build82_windows_amd64.exe" -OutFile "build82_windows_amd64.exe"
Invoke-WebRequest -Uri "https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt" -OutFile "checksums.txt"
Get-FileHash .\build82_windows_amd64.exe -Algorithm SHA256
```

Compare the printed `Hash` with the `build82_windows_amd64.exe` line in `checksums.txt`; continue
only if they match:

```powershell
Unblock-File .\build82_windows_amd64.exe
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\build82"
Move-Item -Force .\build82_windows_amd64.exe "$env:LOCALAPPDATA\build82\build82.exe"
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$env:LOCALAPPDATA\build82", "User")
```

Restart your terminal and run `build82 --version` to confirm it's on your `PATH`.
The binary isn't code-signed; `Unblock-File` above keeps SmartScreen from blocking the first run
("Windows protected your PC") — if it still appears, click **More info → Run anyway**.

</details>

**Or build from source** (requires Go 1.26+):

```bash
go install github.com/oito2/mcp-build82/cmd/build82@latest
```

## Client Setup

Register build82 as an MCP server in your AI tool:

```bash
build82 install [target]
```

It detects which supported tools are installed and prompts for the Moodle path. Run it without a
target to configure every tool found, or pass one explicitly: `claude` (Claude Code),
`claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`. Close the client
first: `install` edits a configuration file the client also writes.

For example, with Claude Code you can also register it manually:

```bash
claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=/path/to/moodle -- /usr/local/bin/build82
```

**Claude Desktop** can also install build82 as a desktop extension: every release ships a
`build82.mcpb` bundle with the binaries included — see
[Claude Desktop](docs/en/guides/clients/claude-desktop.md#desktop-extension-mcpb). build82 is also
listed in the official MCP Registry as `io.github.oito2/mcp-build82`.

Per-client guides, including manual configuration: [MCP Clients](docs/en/guides/clients/).
To remove it: `build82 uninstall [target]` — see [Uninstallation](docs/en/getting-started/uninstallation.md).

## Update & Maintenance

```bash
build82 self-update --check    # report whether a newer release exists (exit 10 if so), without installing it
build82 self-update            # download, verify, and install the latest release
build82 self-update --require-signature # refuse to update unless cosign can verify the release signature
build82 self-update --rollback # restore the previous binary if the new version turns out to be broken
```

Downloads come only from this repository's release on GitHub. When [cosign](https://docs.sigstore.dev/cosign/system_config/installation/)
v3 or later is on `PATH`, the release's signature is verified first; without it, a warning is
printed and only the checksum is checked. Every download is checksum-verified against the
release's `checksums.txt` and smoke-tested before the running binary is replaced; the previous
binary is kept as `<path>.bak`, and `--rollback` swaps the two (so running it again undoes it). If
no backup exists, or the backup does not run, it fails with a clear error and nothing is changed. Details:
[CLI reference](docs/en/reference/cli.md#self-update).

## Documentation

The full documentation site lives in [`docs/en/index.md`](docs/en/index.md) (also in
[Portuguese](docs/pt-br/index.md)). Main entry points:

- [Quickstart](docs/en/getting-started/quickstart.md) — first run in a few minutes
- [Architecture](docs/en/concepts/architecture.md) — how build82 works, data flow and design decisions
- [Tools](docs/en/reference/tools.md), [Resources](docs/en/reference/resources.md) and
  [Prompts](docs/en/reference/prompts.md) reference — every MCP capability in detail
- [CLI](docs/en/reference/cli.md) and [Configuration](docs/en/reference/configuration.md)
  reference — every flag, environment variable and config key
- [Example Prompts](docs/en/prompts.md) — what to ask your AI agent
- [Guides](docs/en/guides/workflows/examples.md) and [Troubleshooting](docs/en/troubleshooting/common-issues.md)

Contributing: see [`CONTRIBUTING.md`](CONTRIBUTING.md) for the development workflow and release
process, and the [Code of Conduct](CODE_OF_CONDUCT.md).

## AI Usage in This Project

This project was developed with the assistance of generative AI tools:

- **Scope:** Generation of boilerplate, unit tests and refactoring of helper functions.

- **Oversight:** All generated code was manually reviewed, tested and validated before integration.

## License

GPL-3.0 — see [LICENSE](LICENSE).
