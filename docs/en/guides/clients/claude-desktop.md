🌐 [Português](../../../pt-br/guides/clients/claude-desktop.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Claude Desktop

**Claude Desktop** is Anthropic's desktop app for macOS, Windows and Linux. The Linux version is in beta and supports Ubuntu 22.04+ and Debian 12+ (x86-64 and arm64); see [Claude Desktop on Linux](https://code.claude.com/docs/en/desktop-linux). It launches local MCP servers over **stdio** from a JSON file, `claude_desktop_config.json`.

---

## 🛠️ MCP Server Configuration

### Desktop extension (`.mcpb`)

Every release ships `build82.mcpb`, a desktop extension bundle with the `build82` binaries for macOS (universal: Intel and Apple Silicon), Windows (x86-64) and Linux (x86-64 and arm64, picked at startup by a small launcher script). It does not need a separately installed `build82` binary.

1. Download `build82.mcpb` from the [latest release](https://github.com/oito2/mcp-build82/releases/latest) and verify it against `checksums.txt` (see [Installation](../../getting-started/installation.md)).
2. In Claude Desktop, open **Settings > Extensions**, click **Advanced settings** and, in the **Extension Developer** section, click **Install Extension…**. Select `build82.mcpb`.
3. When prompted, choose the **Moodle root directory** (required). It is passed to the server as `BUILD82_MOODLE_PATH`.

Use either the extension or the setup below, not both: each one registers its own `build82` server. To update the extension, install the `build82.mcpb` of the newer release. The bundle is not signed.

### Automatic setup

```bash
build82 install claude-desktop
```

What it does, exactly:

1. Checks that Claude Desktop is detected (its configuration directory exists: `~/Library/Application Support/Claude` on macOS, `%APPDATA%\Claude` on Windows — or, for the MSIX package, `%LOCALAPPDATA%\Packages\Claude_<id>\LocalCache\Roaming\Claude` — and `$XDG_CONFIG_HOME/Claude` on Linux, defaulting to `~/.config/Claude`); otherwise it prints `Skipped: Claude Desktop not detected.` and changes nothing. Claude Desktop creates this directory the first time it starts, so launch it once before running the command.
2. Asks for the Moodle root (offers the current directory if it looks like a Moodle root).
3. Merges a `build82` entry into `claude_desktop_config.json` in each of those directories that exists (top-level `mcpServers` object), keeping every other entry, the order of the keys and each value as written (only the indentation is normalized to 2 spaces), the file's permissions, and a symbolic link at that path (the file it points to is written). A new `build82` entry goes at the end of the object. If the file does not exist it is created.
4. Registers the absolute path of the running `build82` binary as the command (stdio) and sets a single environment variable, `BUILD82_MOODLE_PATH`.

If the file contains comments or trailing commas (JSONC), build82 reads it but never rewrites it: if it already holds the same `build82` entry, the tool counts as installed; otherwise it prints `Claude Desktop... manual step needed: ...` with the exact `build82` snippet to paste by hand into its `mcpServers` object, the file stays unchanged, and `install` exits with code 1. A UTF-8 byte order mark is tolerated. A file that is not valid JSON at all, or whose `mcpServers` key is not an object, is reported as `failed` and left unchanged.

Resulting entry:

```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle" }
    }
  }
}
```

Restart Claude Desktop afterwards (see [Restart](#restart)). The `claude` target configures **Claude Code** only (see [Claude Code](./claude-code.md)).

### Manual setup (global)

Claude Desktop has a single, user-wide configuration file (there is no project scope). Open it via **Claude menu → Settings… → Developer → Edit Config**, or edit it directly:

| Operating system | Path |
|------------------|------|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |
| Windows, MSIX package (the claude.ai download and the Microsoft Store) | `%LOCALAPPDATA%\Packages\Claude_<id>\LocalCache\Roaming\Claude\claude_desktop_config.json` |
| Linux | `~/.config/Claude/claude_desktop_config.json` (`$XDG_CONFIG_HOME/Claude/` when `XDG_CONFIG_HOME` is set) |

On Windows, the MSIX package of Claude Desktop reads the virtualized copy under `%LOCALAPPDATA%\Packages\`, while **Edit Config** may open the `%APPDATA%` file instead ([anthropics/claude-code#26073](https://github.com/anthropics/claude-code/issues/26073)); `build82 install claude-desktop` writes to every one of these directories that exists. Close Claude Desktop before running `install` or editing the file: it rewrites the file while it runs.

Add `build82` under `mcpServers` (keep any servers already there):

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

On Windows, escape backslashes in paths (`"C:\\Tools\\build82.exe"`). Always use an **absolute path** for `command` — Claude Desktop does not inherit your shell's `PATH`. Run `which build82` (macOS and Linux) or `where build82` (Windows) to find it.

### Restart

Completely quit Claude Desktop and start it again. The configuration is only read at startup.

### Verify

After the restart, click the **+ / Add files, connectors, and more** button in the conversation input, hover **Connectors**, and confirm `build82` is listed with its tools. Then ask:

```
Run the build82 doctor.
```

### Remove

```bash
build82 uninstall claude-desktop
```

This deletes only the `build82` key from every `claude_desktop_config.json` listed above (other entries are untouched; a missing file is a no-op) and prints `Claude Desktop... removed.`, or `Claude Desktop... not registered.` (exit code 0) when there was nothing to remove. A file with comments or trailing commas (JSONC) is never rewritten: the command prints `Claude Desktop... manual step needed: ...` asking you to remove the entry by hand, and exits with code 1. A file that cannot be read or parsed is reported as `Claude Desktop... failed: ...`, never as not registered. Run `build82 uninstall` without a target to find and remove every build82 registration at once (it asks for confirmation).

Restart Claude Desktop afterwards.

---

## ⚠️ Troubleshooting

- **Server not listed:** validate the JSON syntax and confirm `command` is an absolute path to an executable file.
- **Logs:** macOS `~/Library/Logs/Claude`, Windows `%APPDATA%\Claude\logs`, Linux `~/.config/Claude/logs`. `mcp.log` covers connection problems; `mcp-server-build82.log` holds build82's stderr.
- **Wrong Moodle path:** `BUILD82_MOODLE_PATH` must point to the directory containing `version.php`; the `doctor` tool reports validation errors.
- **Transport:** Claude Desktop launches build82 over stdio (the default). The `--http` mode is not used here.

Official reference: [Connect to local MCP servers](https://modelcontextprotocol.io/docs/develop/connect-local-servers).

---

## ➡️ Next Steps

- [Claude Code](./claude-code.md) — Anthropic CLI, `build82 install claude`
- [OpenAI Codex](./codex.md) — OpenAI CLI with TOML configuration
- [Antigravity (IDE and CLI)](./antigravity.md) — Google IDE and terminal agent
- [OpenCode](./opencode.md) — open-source agent with TUI interface
- [Cursor](./cursor.md) — Cursor editor
- [Zed](./zed.md) — Zed editor
- [Cline](./cline.md) — Cline VS Code extension and CLI
- [Workflow Examples](../workflows/examples.md) — real-world use cases and ready-to-use prompts
- [Tools Reference](../../reference/tools.md) — complete parameters for all tools
- [Common Issues](../../troubleshooting/common-issues.md) — detailed troubleshooting
- [Back to Index](../../index.md)
