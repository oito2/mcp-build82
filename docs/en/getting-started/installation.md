🌐 [Português](../../pt-br/getting-started/installation.md) | **English** | 🏠 [Index](../index.md)

---

# Installation Guide

**build82** ships as a single static binary — no runtime, no dependency manager, no `node_modules`. Download a release, or build it from source if you have Go installed.

---

## 📋 Prerequisites

| Component         | Minimum version | Notes                                     |
| :----------------- | :--------------- | :----------------------------------------- |
| Go (source builds only) | 1.26            | Not needed if you download a release binary |
| Moodle             | 4.1              | Hook API requires Moodle 4.3+              |
| Operating system   | Linux, macOS, Windows | Prebuilt binaries: Linux and macOS on amd64/arm64, Windows on amd64 only |

You will also need a **compatible MCP client** to interact with the server. build82's `install` command auto-configures any of these it finds:

- [Claude Code](../guides/clients/claude-code.md)
- [Claude Desktop](../guides/clients/claude-desktop.md) (macOS, Windows and Linux beta)
- [Antigravity (IDE and CLI)](../guides/clients/antigravity.md)
- [OpenAI Codex](../guides/clients/codex.md)
- [OpenCode](../guides/clients/opencode.md)
- [Cursor](../guides/clients/cursor.md), [Zed](../guides/clients/zed.md) and [Cline](../guides/clients/cline.md) (VS Code extension and CLI)

---

## 🚀 Option 1: Download a Release Binary (Recommended)

No Go toolchain required. Each release publishes one asset per platform, named `build82_<os>_<arch>` (with `.exe` on Windows):

| Asset | Platform |
| :--- | :--- |
| `build82_linux_amd64` | Linux, x86-64 |
| `build82_linux_arm64` | Linux, ARM64 |
| `build82_darwin_amd64` | macOS, Intel |
| `build82_darwin_arm64` | macOS, Apple Silicon |
| `build82_windows_amd64.exe` | Windows, x86-64 |
| `build82.mcpb` | Claude Desktop extension bundle: macOS (universal), Windows x86-64, Linux x86-64 and arm64 — see [Claude Desktop](../guides/clients/claude-desktop.md#desktop-extension-mcpb) |

Every release also ships a `checksums.txt` (SHA-256, one `<hash>  <asset name>` line per asset) and
a `server.json` MCP Registry descriptor. Always verify a download against `checksums.txt` before
trusting it. Pick your operating system below.

### 🐧 Linux

```bash
# amd64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_amd64

# arm64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_arm64
```

Verify the checksum **before** installing — `checksums.txt` lists the release file name, so run this in the download folder while the file still has it (it must print `OK` for your file):

```bash
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

Only if it printed `OK`, install the binary you downloaded:

```bash
# amd64
chmod +x build82_linux_amd64
sudo mv build82_linux_amd64 /usr/local/bin/build82

# arm64
chmod +x build82_linux_arm64
sudo mv build82_linux_arm64 /usr/local/bin/build82
```

`/usr/local/bin` is on `PATH` by default on virtually every distribution, so confirm the install
worked:

```bash
build82 --version
```

If you'd rather not use `sudo`, move the binary to any directory already on your user `PATH`
instead (e.g. `~/.local/bin`, `~/bin`).

### 🍎 macOS

Binaries are published separately for Apple Silicon and Intel — check your chip with `uname -m`
(`arm64` = Apple Silicon M1/M2/M3/M4, `x86_64` = Intel):

```bash
# Apple Silicon (M1/M2/M3/M4)
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_darwin_arm64

# Intel
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_darwin_amd64
```

Verify the checksum **before** installing (macOS ships `shasum` rather than `sha256sum`) — `checksums.txt` lists the release file name, so run this in the download folder while the file still has it (it must print `OK` for your file):

```bash
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
```

Only if it printed `OK`, install the binary you downloaded:

```bash
# Apple Silicon (M1/M2/M3/M4)
chmod +x build82_darwin_arm64
sudo mv build82_darwin_arm64 /usr/local/bin/build82

# Intel
chmod +x build82_darwin_amd64
sudo mv build82_darwin_amd64 /usr/local/bin/build82
```

> **Gatekeeper warning:** release binaries aren't notarized by Apple, so macOS will refuse to run
> the binary the first time, reporting it "cannot be opened because the developer cannot be
> verified." Clear the quarantine attribute before running it:
> ```bash
> xattr -d com.apple.quarantine /usr/local/bin/build82
> ```
> If that's blocked by your organization's MDM policy, you can instead approve it once under
> **System Settings → Privacy & Security → Security**, under the message about the blocked app.

Confirm the install worked:

```bash
build82 --version
```

### 🪟 Windows

Download the `amd64` binary with PowerShell:

```powershell
Invoke-WebRequest -Uri "https://github.com/oito2/mcp-build82/releases/latest/download/build82_windows_amd64.exe" -OutFile "build82_windows_amd64.exe"
```

Verify the checksum **before** installing — PowerShell's built-in `Get-FileHash` replaces `sha256sum`:

```powershell
Invoke-WebRequest -Uri "https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt" -OutFile "checksums.txt"
Get-FileHash .\build82_windows_amd64.exe -Algorithm SHA256
```

Compare the printed `Hash` value against the matching line for `build82_windows_amd64.exe` in
`checksums.txt`; continue only if they match.

> **SmartScreen warning:** release binaries aren't code-signed, so Windows Defender SmartScreen
> may block the first run with "Windows protected your PC." Click **More info → Run anyway**, or
> unblock the file beforehand so the prompt never appears:
> ```powershell
> Unblock-File .\build82_windows_amd64.exe
> ```

Move it somewhere permanent and add that folder to your user `PATH` (no admin rights required):

```powershell
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\build82"
Move-Item -Force .\build82_windows_amd64.exe "$env:LOCALAPPDATA\build82\build82.exe"
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$env:LOCALAPPDATA\build82", "User")
```

Close and reopen your terminal (so the updated `PATH` takes effect), then confirm the install
worked:

```powershell
build82 --version
```

> **WSL users:** if you're running Moodle inside WSL, follow the **Linux** instructions above
> inside your WSL distribution instead — a Windows `.exe` won't run there.

---

## 🛠️ Option 2: Build From Source (Go 1.26+)

Use this method if you intend to contribute to the project or want the latest unreleased code. It requires Go 1.26 or newer (the `go` directive in `go.mod`). The module path is `github.com/oito2/mcp-build82` and the entry point is `cmd/build82`.

```bash
go install github.com/oito2/mcp-build82/cmd/build82@latest
```

This installs `build82` into `$(go env GOPATH)/bin` — make sure that directory is on your `PATH`.

Or clone and build manually:

```bash
git clone https://github.com/oito2/mcp-build82.git
cd mcp-build82
go build -o build82 ./cmd/build82
```

`go build` produces the final, directly runnable binary (on Windows, use `-o build82.exe`). Run the test suite with:

```bash
go test ./...
```

See [Contributing](../../../CONTRIBUTING.md) for the full development workflow.

---

## ⚙️ Configuring the Moodle Path

build82 persists its configuration to a small file at `~/.build82` once you run `init_moodle_context` — you don't need to set anything by hand for day-to-day use. Environment variables remain available and take precedence, which is useful for CI or one-off overrides. The three variables below cover the Moodle location; the full list (including `BUILD82_EXTRACTOR_BACKEND` and `BUILD82_TOKEN`) is in the [Configuration Reference](../reference/configuration.md):

| Variable                    | Required | Description                                                                             |
| :--------------------------- | :------: | :---------------------------------------------------------------------------------------- |
| `BUILD82_MOODLE_PATH`       |  ❌ No   | Absolute path to the Moodle root. Overrides the saved config file when set.               |
| `BUILD82_MOODLE_VERSION`    |  ❌ No   | Moodle version (e.g. `4.5`), read only with `BUILD82_MOODLE_PATH`. See note below.       |
| `BUILD82_MOODLE_FULLVERSION`|  ❌ No   | Numeric Moodle build number: the `$version` of `version.php` (e.g. `2024100700`).         |

### About `BUILD82_MOODLE_VERSION`

`init_moodle_context` and `update_indexes` always detect the version from `version.php` for the indexes they generate. The other tools, resources and prompts read the version from the resolved configuration, and that configuration comes from **one source only**: when `BUILD82_MOODLE_PATH` is set (`build82 install` always sets it in the client entry), path, version and full version all come from the environment — `BUILD82_MOODLE_VERSION` and `BUILD82_MOODLE_FULLVERSION` are empty when unset, and the values in `~/.build82` are never merged in. Set them only if you want those tools to know the Moodle version while running with `BUILD82_MOODLE_PATH`. See [Configuration Reference](../reference/configuration.md#precedence).

---

## 🔌 Registering the MCP Server

build82 can configure your MCP client for you:

```bash
build82 install [target]
```

Run it without a target to auto-detect every supported client found on your machine (it lists them and asks for confirmation), or pass one explicitly (`claude`, `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`); an explicit target that is not detected is skipped. It prompts for the Moodle path and writes the client-specific config itself: for `claude` and `codex` it runs the tool's own `mcp add` command, for the others it merges a `build82` entry into the client's JSON configuration file (a file with comments or trailing commas is never rewritten; the command prints the snippet to paste by hand). Each client prints `<Label>... configured.`, or `<Label>... updated.` when build82 was already registered there: running `build82 install` again replaces the registration, updating the binary path and `BUILD82_MOODLE_PATH`. Exact locations and details per client are on the [client guides](../guides/clients/claude-code.md), which also cover manual configuration.

To remove the registration later:

```bash
build82 uninstall [target]
```

This removes only the `build82` registration from the client. Passing `--purge` additionally deletes the `~/.build82` config file, the global generated files, and the `PLUGIN_*.md` files and `.indevelopment` marker of marked development plugins, after its own separate confirmation. It leaves `.build82/tags` and `.build82/.cache.json` in place. The binary itself is not removed. See [Uninstallation](./uninstallation.md) for the complete list and manual cleanup.

---

## 🔍 Verifying the Installation

After installing and registering the MCP client, verify the server works by asking your AI assistant:

```
Run the build82 doctor
```

The AI will call the `doctor` tool via MCP and return a report: detected Moodle version, configured path, index freshness, legacy files pending migration, and cache stats.

> **Note:** `doctor` is an MCP tool, not a CLI command — it only runs inside an active session with a compatible client.

---

## ⬆️ Keeping build82 Up To Date

```bash
build82 self-update --check   # report whether a newer release exists, without installing it
build82 self-update           # download, verify, and install the latest release
build82 self-update --rollback # restore the previous binary if the new version turns out to be broken
```

Downloads are checksum-verified against the release's `checksums.txt` and smoke-tested before the running binary is ever replaced; the previous binary is kept alongside it as `<path>.bak`.

If a new version passes that smoke test but turns out to be broken in real use afterward, `build82 self-update --rollback` promotes the `.bak` backup back into place with an atomic rename, then runs its own smoke test against the restored binary purely as a diagnostic confirmation (its result doesn't undo the rollback either way). If no `.bak` backup exists, it fails with a clear error and leaves the current binary untouched.

---

## ➡️ Next steps

With the server installed, configure your MCP client:

- [Configure Claude Code](../guides/clients/claude-code.md)
- [Configure Claude Desktop](../guides/clients/claude-desktop.md)
- [Configure Antigravity (IDE and CLI)](../guides/clients/antigravity.md)
- [Configure OpenAI Codex](../guides/clients/codex.md)
- [Configure OpenCode](../guides/clients/opencode.md)

Or jump straight to usage:

- [Quickstart](./quickstart.md)
- [Back to Index](../index.md)
