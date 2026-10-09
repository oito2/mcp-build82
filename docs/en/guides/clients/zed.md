🌐 [Português](../../../pt-br/guides/clients/zed.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Zed

**Zed** registers MCP servers as **context servers** in `settings.json`, under the `context_servers` key (not `mcpServers`).

---

## 🛠️ MCP Server Configuration

### Automatic setup

```bash
build82 install zed
```

What it does, exactly:

1. Checks that Zed is detected (the `~/.config/zed` directory exists; on Windows, `%APPDATA%\Zed`); otherwise it prints `Skipped: Zed not detected.` and changes nothing.
2. Asks for the Moodle root (offers the current directory if it looks like a Moodle root).
3. Merges a `build82` entry into `~/.config/zed/settings.json` (Windows: `%APPDATA%\Zed\settings.json`) (top-level `context_servers` object), keeping every other entry, the order of the keys and each value as written (only the indentation is normalized to 2 spaces), the file's permissions, and a symbolic link at that path (the file it points to is written). A new `build82` entry goes at the end of the object. If the file does not exist it is created.
4. Registers the absolute path of the running `build82` binary as the command (stdio) and sets a single environment variable, `BUILD82_MOODLE_PATH`.

If the file contains comments or trailing commas (JSONC), build82 reads it but never rewrites it: if it already holds the same `build82` entry, the tool counts as installed; otherwise it prints `Zed... manual step needed: ...` with the exact `build82` snippet to paste by hand into its `context_servers` object, the file stays unchanged, and `install` exits with code 1. A UTF-8 byte order mark is tolerated. A file that is not valid JSON at all, or whose `context_servers` key is not an object, is reported as `failed` and left unchanged.

Resulting entry:

```json
{
  "context_servers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle" }
    }
  }
}
```

Zed's `settings.json` commonly contains comments and trailing commas. In that case the installer leaves it untouched and prints the snippet to paste into the `context_servers` object yourself. The project file `.zed/settings.json` is never written.

### Manual setup

Open the file with the `zed: open settings file` command (global) or use `.zed/settings.json` at the project root (project scope):

```json
{
  "context_servers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle" }
    }
  }
}
```

Alternatively use **Settings → AI → MCP Servers → Add Server**.

### Verify

In **Settings → AI → MCP Servers** (or the Agent Panel settings) the `build82` entry must show a **green dot** ("Server is active"). Hover other states for the error message.

### Remove

```bash
build82 uninstall zed
```

This deletes only the `build82` key from `~/.config/zed/settings.json` (Windows: `%APPDATA%\Zed\settings.json`). Other entries are untouched and a missing file is a no-op. It prints `Zed... removed.`, or `Zed... not registered.` (exit code 0) when there was nothing to remove. A file with comments or trailing commas (JSONC) is never rewritten: the command prints `Zed... manual step needed: ...` asking you to remove the entry by hand, and exits with code 1. A file that cannot be read or parsed is reported as `Zed... failed: ...`, never as not registered. Run `build82 uninstall` without a target to find and remove every build82 registration at once (it asks for confirmation).

Remove entries from a project `.zed/settings.json` by hand.

---

## ⚠️ Troubleshooting

- **Indicator not green:** hover it for the reason; check that `command` is an absolute path.
- **Wrong Moodle path:** `BUILD82_MOODLE_PATH` must be the directory containing `version.php`.

Official reference: [Model Context Protocol in Zed](https://zed.dev/docs/ai/mcp).

---

## ➡️ Next Steps

- [Claude Desktop](./claude-desktop.md) — Anthropic desktop app (macOS, Windows and Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — Anthropic CLI, `build82 install claude`
- [OpenAI Codex](./codex.md) — OpenAI CLI with TOML configuration
- [Antigravity (IDE and CLI)](./antigravity.md) — Google IDE and terminal agent
- [OpenCode](./opencode.md) — open-source agent with TUI interface
- [Cursor](./cursor.md) — Cursor editor
- [Cline](./cline.md) — Cline VS Code extension and CLI
- [Workflow Examples](../workflows/examples.md) — real-world use cases and ready-to-use prompts
- [Tools Reference](../../reference/tools.md) — complete parameters for all tools
- [Common Issues](../../troubleshooting/common-issues.md) — detailed troubleshooting
- [Back to Index](../../index.md)
