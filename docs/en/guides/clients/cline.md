🌐 [Português](../../../pt-br/guides/clients/cline.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Cline

**Cline** is a VS Code extension (`saoudrizwan.claude-dev`). Its MCP servers live in a single `cline_mcp_settings.json` file, using the `mcpServers` shape.

---

## 🛠️ MCP Server Configuration

### Automatic setup

```bash
build82 install cline
```

What it does, exactly:

1. Checks that Cline (VS Code extension / CLI) is detected (the Cline globalStorage directory of stable VS Code exists, or the Cline CLI directory exists: `~/.cline`, `$CLINE_DIR` when that variable holds an absolute path, or `$CLINE_DATA_DIR` when that variable is set); otherwise it prints `Skipped: Cline (VS Code extension / CLI) not detected.` and changes nothing.
2. Asks for the Moodle root (offers the current directory if it looks like a Moodle root).
3. Merges a `build82` entry into every Cline settings file whose parent location exists (see the table below), using the top-level `mcpServers` object, keeping every other entry, the order of the keys and each value as written (only the indentation is normalized to 2 spaces), the file's permissions, and a symbolic link at that path (the file it points to is written). A new `build82` entry goes at the end of the object. If the file does not exist it is created.
4. Registers the absolute path of the running `build82` binary as the command (stdio) and sets a single environment variable, `BUILD82_MOODLE_PATH`.

If the file contains comments or trailing commas (JSONC), build82 reads it but never rewrites it: if it already holds the same `build82` entry, the tool counts as installed; otherwise it prints `Cline (VS Code extension / CLI)... manual step needed: ...` with the exact `build82` snippet to paste by hand into its `mcpServers` object, the file stays unchanged, and `install` exits with code 1. A UTF-8 byte order mark is tolerated. A file that is not valid JSON at all, or whose `mcpServers` key is not an object, is reported as `failed` and left unchanged.

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

The installer writes to each location that exists, so a machine with both the VS Code extension and the Cline CLI gets both files:

| Location | Path |
|----------|------|
| VS Code extension, Linux | `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| VS Code extension, macOS | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| VS Code extension, Windows | `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json` |
| Cline CLI (all systems) | `~/.cline/data/settings/cline_mcp_settings.json` |
| Cline CLI with `CLINE_DIR` set (absolute path) | `$CLINE_DIR/data/settings/cline_mcp_settings.json` |
| Cline CLI with `CLINE_DATA_DIR` set | `$CLINE_DATA_DIR/settings/cline_mcp_settings.json` (takes precedence over `CLINE_DIR`) |

Only stable VS Code is detected (not Insiders, VSCodium or portable installs).

`CLINE_DIR` and `CLINE_DATA_DIR` are the Cline CLI's own variables: `CLINE_DIR` replaces the configuration directory `~/.cline`, and `CLINE_DATA_DIR` replaces the data directory `~/.cline/data` ([Cline CLI configuration](https://docs.cline.bot/cline-cli/configuration)). build82 uses an absolute `CLINE_DIR` in place of `~/.cline` (a relative value is ignored), and a non-empty `CLINE_DATA_DIR` in place of the data directory, for detection, install and uninstall — so run `build82` with the same values the Cline CLI uses. The installer prints `Cline (VS Code extension / CLI)... configured.`, or `... updated.` when a `build82` entry already existed in one of the files.

### Manual setup

Cline documents only a global file (no project scope). For the Cline CLI use `~/.cline/data/settings/cline_mcp_settings.json` (or `$CLINE_DIR/data/settings/cline_mcp_settings.json`, or `$CLINE_DATA_DIR/settings/cline_mcp_settings.json`) with the same content. In VS Code click the **MCP Servers** icon in Cline's top toolbar → **Configure** tab → **Configure MCP Servers**; this opens the file. Add:

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

### Verify

Open the **MCP Servers** panel in Cline and confirm `build82` shows as connected (green indicator) and lists its tools. Then ask:

```
Run the build82 doctor.
```

### Remove

```bash
build82 uninstall cline
```

This deletes only the `build82` key from `cline_mcp_settings.json` in both locations (VS Code globalStorage and `~/.cline/data/settings/`, `$CLINE_DIR/data/settings/` or `$CLINE_DATA_DIR/settings/` when set) (other entries are untouched; a missing file is a no-op) and prints `Cline (VS Code extension / CLI)... removed.`, or `Cline (VS Code extension / CLI)... not registered.` (exit code 0) when there was nothing to remove. A file with comments or trailing commas (JSONC) is never rewritten: the command prints `Cline (VS Code extension / CLI)... manual step needed: ...` asking you to remove the entry by hand, and exits with code 1. A file that cannot be read or parsed is reported as `Cline (VS Code extension / CLI)... failed: ...`, never as not registered. Run `build82 uninstall` without a target to find and remove every build82 registration at once (it asks for confirmation).

---

## ⚠️ Troubleshooting

- **Server not listed:** save the file, then reopen the MCP Servers panel; check the JSON syntax.
- **`build82: command not found`:** use the absolute path (`which build82`).
- **Wrong Moodle path:** `BUILD82_MOODLE_PATH` must be the directory containing `version.php`.

Official reference: [Configuring MCP servers (Cline)](https://docs.cline.bot/mcp/configuring-mcp-servers).

---

## ➡️ Next Steps

- [Claude Desktop](./claude-desktop.md) — Anthropic desktop app (macOS, Windows and Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — Anthropic CLI, `build82 install claude`
- [OpenAI Codex](./codex.md) — OpenAI CLI with TOML configuration
- [Antigravity (IDE and CLI)](./antigravity.md) — Google IDE and terminal agent
- [OpenCode](./opencode.md) — open-source agent with TUI interface
- [Cursor](./cursor.md) — Cursor editor
- [Zed](./zed.md) — Zed editor
- [Workflow Examples](../workflows/examples.md) — real-world use cases and ready-to-use prompts
- [Tools Reference](../../reference/tools.md) — complete parameters for all tools
- [Common Issues](../../troubleshooting/common-issues.md) — detailed troubleshooting
- [Back to Index](../../index.md)
