🌐 [Português](../../pt-br/troubleshooting/common-issues.md) | **English** | 🏠 [Index](../index.md)

---

# Troubleshooting

Running into issues with `build82`? This page covers the most common errors with direct solutions.

---

## 🔍 Initial Diagnosis

Before investigating any specific problem, run these two steps — they resolve most cases:

**Step 1 — Check if the server is connected:**

In your AI client chat, run:
```
/mcp
```

If `build82` appears as connected with 13 tools, the server is working. The problem is in the context configuration, not the connection.

If it does not appear, the problem is in the server configuration — see the [Connection Errors](#-connection-and-path-errors) section below.

**Step 2 — Check environment health:**

```
Run the build82 doctor.
```

The AI will call the `doctor` tool and return a report with: config file location, Moodle path and detected version, index freshness, legacy files pending migration, cross-plugin consistency, and cache statistics. Any configuration problem will appear here.

---

## 🧭 Symptom Index

| Symptom | Section |
| :--- | :--- |
| Client says it cannot connect, or the server never leaves "Connecting..." | [The AI client cannot find `build82`](#the-ai-client-cannot-find-build82), [Server stuck at "Connecting..."](#server-stuck-at-connecting) |
| `❌ build82 is not initialized. Run init_moodle_context first.` | [build82 is not initialized](#build82-is-not-initialized) |
| `❌ Invalid Moodle path: ...` | [Moodle path not found](#moodle-path-not-found) |
| `command not found: build82` | [command not found after building from source](#command-not-found-build82-after-building-from-source) |
| macOS or Windows refuses to run the downloaded binary | [The operating system blocks the downloaded binary](#the-operating-system-blocks-the-downloaded-binary) |
| Permission errors when generating `.md` files | [Permission denied when creating `.md` files](#permission-denied-when-creating-md-files) |
| `build82 self-update` fails or the new version is broken | [self-update fails](#build82-self-update-fails) |
| The AI uses outdated fields or functions | [The AI suggests code from an older version of the plugin](#the-ai-suggests-code-from-an-older-version-of-the-plugin) |
| Old Moodle version reported after an upgrade | [Stale context after a Moodle upgrade](#stale-context-after-a-moodle-upgrade) |
| Automatic monitoring stopped | [Watch mode stops after restarting the server](#watch-mode-stops-after-restarting-the-server) |
| `.build82/` shows up in `git status` | [`.build82/` appearing in `git status`](#build82-appearing-in-git-status) |
| Server empty or missing when Moodle runs in Docker | [build82 on the host cannot see Moodle in Docker](#build82-on-the-host-cannot-see-moodle-in-docker) |
| HTTP `401 Unauthorized`, `403 Forbidden`, or a startup error about the token | [HTTP transport errors](#-http-transport-errors) |
| `scaffold_plugin` is missing from the tool list | [`scaffold_plugin` does not appear as a tool in `/mcp`](#scaffold_plugin-does-not-appear-as-a-tool-in-mcp) |
| The AI does not know `build82` | [The AI says it doesn't know `build82`](#the-ai-says-it-doesnt-know-build82) |

---

## 🚫 Connection and PATH Errors

### The AI client cannot find `build82`

**Symptom:** the AI client reports it cannot connect to the server, or the server appears as "Connecting..." and never completes.

**Cause:** unlike `npx`-launched servers, `build82` is a single binary — the client just needs its absolute path when it's not on the PATH the client inherits (IDEs and some CLIs don't always inherit your interactive shell's PATH).

**Solution:** find the absolute path and use it explicitly in the server configuration.

```bash
# Find the correct path
which build82
# → /usr/local/bin/build82
```

**Claude Code (`~/.claude.json`):**
```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "env": {
        "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle"
      }
    }
  }
}
```

**Antigravity CLI (`~/.gemini/config/mcp_config.json`, global):**
```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": {
        "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle"
      }
    }
  }
}
```

**OpenAI Codex (`~/.codex/config.toml`):**
```toml
[mcp_servers.build82]
command = "/usr/local/bin/build82"
env     = { BUILD82_MOODLE_PATH = "/home/user/workspace/www/html/moodle" }
```

**OpenCode (`opencode.json` at the Moodle root):**
```json
{
  "mcp": {
    "build82": {
      "type": "local",
      "command": ["/usr/local/bin/build82"],
      "environment": {
        "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle"
      }
    }
  }
}
```

> Easiest fix of all: run `build82 install [target]` again — it writes the absolute path itself. Note that for some clients (Codex, Antigravity, OpenCode) the file `install` writes differs from the one the client's official documentation lists; if the server still does not appear, configure the client by hand following its [guide](../guides/clients/claude-code.md) (each client has its own page in that folder).

---

### Server stuck at "Connecting..."

**Symptom:** The server appears but never leaves the "Connecting..." state. No tools are listed.

**In CLI clients (Antigravity CLI, OpenCode):**

If the server was configured but does not appear, restart the session:

```bash
Ctrl+C
agy   # or: opencode
```

Then check with `/mcp` inside the session.

---

### Moodle path not found

**Symptom:** `init_moodle_context` returns `❌ Invalid Moodle path: <reason>`, where the reason is one of `directory does not exist`, `version.php not found — this doesn't look like a Moodle root`, `lib/ directory not found`, or `neither config.php nor config-dist.php found`. `build82 install` reports a similar "does not look like a Moodle root" message.

**Cause:** `BUILD82_MOODLE_PATH` (or the path passed to `init_moodle_context`) points to a subdirectory, a directory that does not exist, or uses a relative path.

**Solution:**
- The path must point to the **Moodle root** — the directory containing `version.php`, `lib/`, and `config.php` (or `config-dist.php`)
- Use an **absolute path** (e.g. `/home/user/workspace/www/html/moodle`), never relative (e.g. `./moodle`)

### build82 is not initialized

**Symptom:** a tool returns `❌ build82 is not initialized. Run `init_moodle_context` first.`, or `doctor` reports the config as `not initialized`.

**Cause:** there is no `~/.build82` config file and `BUILD82_MOODLE_PATH` is not set in the server's environment.

**Solution:** ask the AI to run `init_moodle_context` with your Moodle root path, or set `BUILD82_MOODLE_PATH` in the client's server configuration (`build82 install` does this for you). See the [Configuration Reference](../reference/configuration.md).

---

## 🛠️ Build and Runtime Errors

### `command not found: build82` after building from source

**Symptom:** `go build -o build82 ./cmd/build82` succeeds, but running `build82` fails.

**Cause:** the binary was written to the current directory, which usually isn't on `PATH`.

**Solution:**
```bash
sudo mv build82 /usr/local/bin/
# or, if you used `go install`:
export PATH="$(go env GOPATH)/bin:$PATH"
```

---

### The operating system blocks the downloaded binary

**Symptom:** macOS says the binary "cannot be opened because the developer cannot be verified", or Windows SmartScreen shows "Windows protected your PC".

**Cause:** release binaries are neither notarized (macOS) nor code-signed (Windows).

**Solution:** clear the quarantine attribute (`xattr -d com.apple.quarantine /usr/local/bin/build82`) on macOS, or use **More info → Run anyway** / `Unblock-File` on Windows. Details in the [Installation guide](../getting-started/installation.md).

---

### `build82 self-update` fails

**Symptom:** the command exits with an error such as `checksum verification failed, refusing to replace the running binary`, `signature verification failed, refusing to replace the running binary`, `release <tag> has no asset named <asset>`, `refusing to download ...`, `--require-signature refuses to update ...`, `the previous version is still in use`, or `smoke test failed`.

**Cause and solution:** the update is signature-checked (when cosign is available), verified against the release's `checksums.txt` and smoke-tested before the running binary is replaced, so a failure leaves your current binary untouched.

| Error | What to do |
| :--- | :--- |
| `signature verification failed` | The release's `checksums.txt` is not signed by this repository's release workflow for that tag. Do not install it; report it on the issue tracker. cosign's own output follows the message. |
| `--require-signature refuses to update` / warning `cosign was not found on PATH` or `older than the v3.0.0` | Install [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 or later, or run without `--require-signature` to rely on the checksum alone. |
| `release <tag> has no asset named checksums.txt.sigstore.json` | The target release predates signed releases (before v1.1.0). Remove cosign from `PATH` for this update. |
| `refusing to download ...` / `unexpected host` | The release metadata or a redirect pointed outside GitHub's release hosts for this repository. Nothing was downloaded; retry later. |
| `the previous version is still in use` (Windows) | An MCP client is still running the previous `build82.bak`. Restart your MCP clients, then run `build82 self-update` again. |
| `checksum verification failed`, `smoke test failed` | Retry later, or download the asset manually and verify it as described in the [Installation guide](../getting-started/installation.md). |

If a new version installed fine but misbehaves, run `build82 self-update --rollback`; `no backup found at <path> — nothing to roll back` means there is no `.bak` file next to the binary.

---

### Permission denied when creating `.md` files

**Symptom:** The server connects, but when generating context it returns a permission error. The `.build82/PLUGIN_*.md` files are not created.

**Cause:** The user running `build82` does not have write permission on the Moodle directory or plugin directories.

**Solution on Linux (direct installation):**
```bash
sudo chown -R $USER:$USER /path/to/your/moodle/local/
```

**Solution with Docker (sidecar scenario):**

Make sure the volume is not mounted as `:ro` (read-only). The server needs to write the `.md` context files:

```yaml
volumes:
  - ./www/html/moodle:/var/www/moodle  # without :ro
```

---

## 🔄 Context and Cache Issues

### The AI suggests code from an older version of the plugin

**Symptom:** You modified `db/install.xml` or a PHP file, but the AI still references the previous version of the fields or functions.

**Cause:** The mtime cache may not have detected the change, or the plugin context was not regenerated after the changes.

**Solution — for a specific plugin:**
```
Regenerate the context for local_myplugin ignoring the cache.
```
(The AI will call `plugin_batch mode="list" plugins=["local_myplugin"] force=true` — `generate_plugin_context` itself always respects the cache and has no `force` parameter.)

**Solution — for all global indexes:**
```
Regenerate all Moodle indexes ignoring the cache.
```

The AI will call `update_indexes` with `force=true`.

---

### Stale context after a Moodle upgrade

**Symptom:** After upgrading Moodle to a new version, the AI still reports the old version and functions that were deprecated or removed.

**Solution:**
```
Reinitialize the build82 context for the Moodle installation.
```

The AI will call `init_moodle_context`, detect the new version, and regenerate all 13 global indexes.

---

### Watch mode stops after restarting the server

**Symptom:** Automatic plugin monitoring stops working after restarting VS Code, Claude Code, or the MCP server.

**Cause:** Watch mode is **in-memory** — it does not persist across server restarts (unlike the mtime cache itself, which is persisted to `.build82/.cache.json`).

**Solution:** Reactivate monitoring after each restart:
```
Start monitoring local_myplugin for file changes.
```

---

### `.build82/` appearing in `git status`

**Symptom:** `git status` shows a new, untracked `.build82/` directory in the Moodle installation or in a plugin.

**Solution:** Add one line to `.gitignore` at the Moodle root:

```gitignore
# build82 context files
.build82/
```

Since every generated file lives under this single directory, that's the whole exclusion list.

---

## 🐳 Docker and Virtual Environments

### build82 on the host cannot see Moodle in Docker

**Symptom:** The server reports the folder is empty or does not exist, even with Moodle running in containers.

**Cause:** `BUILD82_MOODLE_PATH` must be the path **on the host** (your machine), not the internal container path.

**Solution:** use the host-side path in your client config:

```json
"env": {
  "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle"
}
```

build82 running on the host reads directly from the mounted volume's files on disk — it never needs to reach into the container.

---

## 🌐 HTTP Transport Errors

Only relevant when running `build82 --http` (see [Docker](../guides/environments/docker.md) and the [CLI Reference](../reference/cli.md)).

### `401 Unauthorized`

**Symptom:** requests to `/mcp` or `/sse` return `Valid Bearer token required. Set Authorization: Bearer <token> header.`

**Solution:** send `Authorization: Bearer <token>` with exactly the token the server was started with (`--token` or `BUILD82_TOKEN`). `/health` never needs a token.

### `403 Forbidden`: Host not allowed

**Symptom:** the response says `Host "<name>" is not allowed. Pass --allowed-host to permit it.`

**Cause:** the server only accepts `localhost`, `127.0.0.1`, `::1`, the `--host` value, and values given with `--allowed-host`.

**Solution:** restart with `--allowed-host <name>` for every hostname or IP clients use to reach it.

### Startup fails with a token error

**Symptom:** exit code 1 with `--token was passed with an empty value; ...` or `BUILD82_TOKEN is set but empty; ...`.

**Solution:** unset the variable entirely (or pass a non-empty value) to choose between no authentication and a real token. See [Configuration Reference](../reference/configuration.md#build82_token-semantics).

### `listen on <host>:<port>` error

**Symptom:** the server exits with `Fatal error: listen on 127.0.0.1:3000: ...`.

**Solution:** the port is already in use or not allowed; pick another one with `--port <n>`.

---

## ❓ Common Questions

### `scaffold_plugin` does not appear as a tool in `/mcp`

**This is expected.** `scaffold_plugin` is an **MCP Prompt**, not a Tool. It does not appear in the `/mcp` tool list — it is invoked directly in chat as a prompt or slash command:

```
/scaffold_plugin type="local" name="mytools" description="..."
```

For the complete list of tools vs. prompts, see the [Tools Reference](../reference/tools.md) and the [Prompts Reference](../reference/prompts.md).

---

### The AI says it doesn't know `build82`

**Symptom:** The AI responds that it doesn't have access to the server or doesn't know what `build82` is.

**Solution:** Be more explicit in your instruction:

```
Use the get_plugin_info tool to load the context for local_myplugin.
```

```
Use the search_api tool to find functions related to enrollment.
```

Naming the tool explicitly ensures the AI uses it instead of responding with generic knowledge.

---

## 📝 Reporting a New Bug

If your problem is not listed here:

1. Ask your assistant: _"Run the build82 doctor"_ and copy the full output
2. Note which AI client you are using (Claude Code, Antigravity CLI, OpenAI Codex, OpenCode) and your OS/platform
3. Open an **Issue** on GitHub: [github.com/oito2/mcp-build82/issues](https://github.com/oito2/mcp-build82/issues)
4. Describe the steps to reproduce the error and include the `doctor` output

---

> **Tip:** Restarting the IDE or the AI client process resolves most MCP server hang issues — especially after editing configuration files like `~/.claude.json`, `~/.gemini/antigravity/mcp_config.json`, `~/.codex/config.toml`, or `opencode.json`.

---

[🏠 Back to Index](../index.md)
