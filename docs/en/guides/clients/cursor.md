🌐 [Português](../../../pt-br/guides/clients/cursor.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Cursor

**Cursor** reads MCP servers from `mcp.json`, either globally or per project, using the `mcpServers` shape.

---

## 🛠️ MCP Server Configuration

### Automatic setup

```bash
build82 install cursor
```

What it does, exactly:

1. Checks that Cursor is detected (the `~/.cursor` directory exists); otherwise it prints `Skipped: Cursor not detected.` and changes nothing.
2. Asks for the Moodle root (offers the current directory if it looks like a Moodle root).
3. Merges a `build82` entry into `~/.cursor/mcp.json` (top-level `mcpServers` object), keeping every other entry, the order of the keys and each value as written (only the indentation is normalized to 2 spaces), the file's permissions, and a symbolic link at that path (the file it points to is written). A new `build82` entry goes at the end of the object. If the file does not exist it is created.
4. Registers the absolute path of the running `build82` binary as the command (stdio) and sets a single environment variable, `BUILD82_MOODLE_PATH`.

If the file contains comments or trailing commas (JSONC), build82 reads it but never rewrites it: if it already holds the same `build82` entry, the tool counts as installed; otherwise it prints `Cursor... manual step needed: ...` with the exact `build82` snippet to paste by hand into its `mcpServers` object, the file stays unchanged, and `install` exits with code 1. A UTF-8 byte order mark is tolerated. A file that is not valid JSON at all, or whose `mcpServers` key is not an object, is reported as `failed` and left unchanged.

Resulting entry:

```json
{
  "mcpServers": {
    "build82": {
      "type": "stdio",
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle" }
    }
  }
}
```

The installer writes the **global** file only; project files (`.cursor/mcp.json`) are yours to manage.

### Manual setup

| Scope | File |
|-------|------|
| Global (all projects) | `~/.cursor/mcp.json` |
| Project | `.cursor/mcp.json` at the project root |

Both use the same content:

```json
{
  "mcpServers": {
    "build82": {
      "type": "stdio",
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle" }
    }
  }
}
```

Cursor also supports `${env:NAME}`, `${workspaceFolder}` and `${userHome}` interpolation inside `mcp.json`, e.g. `"BUILD82_MOODLE_PATH": "${workspaceFolder}"` in a project file kept at the Moodle root.

### Verify

Open the **Customize** panel in Cursor's sidebar and confirm `build82` is listed and enabled (toggle it if needed). Then ask the agent:

```
Run the build82 doctor.
```

### Remove

```bash
build82 uninstall cursor
```

This deletes only the `build82` key from `~/.cursor/mcp.json`. Other entries are untouched and a missing file is a no-op. It prints `Cursor... removed.`, or `Cursor... not registered.` (exit code 0) when there was nothing to remove. A file with comments or trailing commas (JSONC) is never rewritten: the command prints `Cursor... manual step needed: ...` asking you to remove the entry by hand, and exits with code 1. A file that cannot be read or parsed is reported as `Cursor... failed: ...`, never as not registered. Run `build82 uninstall` without a target to find and remove every build82 registration at once (it asks for confirmation).

Remove a project-level `.cursor/mcp.json` entry by hand.

---

## ⚠️ Troubleshooting

- **Server not listed:** reload the window and check the JSON syntax.
- **`build82: command not found`:** use the absolute path (`which build82`).
- **Wrong Moodle path:** `BUILD82_MOODLE_PATH` must be the directory containing `version.php`.

Official reference: [Cursor MCP documentation](https://cursor.com/docs/context/mcp).

---

## ➡️ Next Steps

- [Claude Desktop](./claude-desktop.md) — Anthropic desktop app (macOS, Windows and Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — Anthropic CLI, `build82 install claude`
- [OpenAI Codex](./codex.md) — OpenAI CLI with TOML configuration
- [Antigravity (IDE and CLI)](./antigravity.md) — Google IDE and terminal agent
- [OpenCode](./opencode.md) — open-source agent with TUI interface
- [Zed](./zed.md) — Zed editor
- [Cline](./cline.md) — Cline VS Code extension and CLI
- [Workflow Examples](../workflows/examples.md) — real-world use cases and ready-to-use prompts
- [Tools Reference](../../reference/tools.md) — complete parameters for all tools
- [Common Issues](../../troubleshooting/common-issues.md) — detailed troubleshooting
- [Back to Index](../../index.md)
