🌐 [Português](../../../pt-br/guides/clients/antigravity.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Antigravity (IDE and CLI)

**Google Antigravity** ships as an IDE and as a terminal agent, **Antigravity CLI** (`agy`, the successor to Gemini CLI). Both read MCP servers from the same `mcp_config.json` format, with a global and a workspace file.

---

## 🛠️ MCP Server Configuration

### Automatic setup

```bash
build82 install antigravity
```

What it does, exactly:

1. Checks that Antigravity (IDE / CLI) is detected (the `agy` command is on `PATH`, or the `~/.gemini/config` or `~/.gemini/antigravity` directory exists); otherwise it prints `Skipped: Antigravity (IDE / CLI) not detected.` and changes nothing.
2. Asks for the Moodle root (offers the current directory if it looks like a Moodle root).
3. Merges a `build82` entry into `~/.gemini/config/mcp_config.json` (top-level `mcpServers` object), keeping every other entry and the file's permissions. If the file does not exist it is created.
4. Registers the absolute path of the running `build82` binary as the command (stdio) and sets a single environment variable, `BUILD82_MOODLE_PATH`.

If the file contains comments or trailing commas (JSONC), build82 reads it but never rewrites it: the command fails with `Antigravity (IDE / CLI)... failed: ...`, the file stays unchanged, and the message includes the exact `build82` snippet to paste by hand into its `mcpServers` object. A file that is not valid JSON at all also aborts without writing.

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

This is the global file both the IDE and the CLI read (see the manual setup below), so IDE-only users are detected too. The workspace file `.agents/mcp_config.json` is never written by the installer.

### Manual setup

Both the IDE and the CLI use the same file format and locations:

| Scope | File |
|-------|------|
| Global (all workspaces) | `~/.gemini/config/mcp_config.json` (Windows: `%USERPROFILE%\.gemini\config\mcp_config.json`) |
| Workspace | `.agents/mcp_config.json` in the workspace root |

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

Use an **absolute** path for `command`: relative paths are not resolved, and the client may not inherit your shell's `PATH` (`which build82`).

You can also add servers without editing files: in the **CLI** type `/mcp` to open the interactive MCP manager; in the **IDE** open the agent side panel menu → **MCP Servers** → **Manage MCP Servers** (raw config file).

> **Known issue (CLI, workspace scope):** [google-antigravity/antigravity-cli#60](https://github.com/google-antigravity/antigravity-cli/issues/60) reports that a project-local MCP config file is discovered at startup but its `mcpServers` are not started; only the home-level config actually spawns servers. At the time of writing (checked 2026-09-29) the issue is **open**, assigned, with no fix version. Until it is fixed, prefer the **global** file for the CLI. The IDE is not reported as affected.

### Verify

- **CLI:** restart the session (`Ctrl+C`, then `agy`) and type `/mcp`; `build82` must be listed.
- **IDE:** reopen the MCP Servers panel; `build82` must be listed with its tools.

Then ask the agent to run the build82 `doctor` tool.

### Remove

```bash
build82 uninstall antigravity
```

This deletes only the `build82` key from `~/.gemini/config/mcp_config.json` and from the legacy `~/.gemini/antigravity/mcp_config.json`. Other entries are untouched and a missing file is a no-op. It prints `Antigravity (IDE / CLI)... removed.`, or `Antigravity (IDE / CLI)... not registered.` (exit code 0) when there was nothing to remove. A file with comments or trailing commas (JSONC) is never rewritten: the command fails with `Antigravity (IDE / CLI)... failed: ...` asking you to remove the entry manually. Run `build82 uninstall` without a target to find and remove every build82 registration at once (it asks for confirmation).

Removing the entry from the config file may not be enough: the IDE and CLI keep cached copies under `~/.gemini/antigravity-ide/mcp/` and `~/.gemini/antigravity-cli/mcp/` and can keep showing a removed server. If build82 still appears, delete the matching cache folder (if any) and restart. An entry you added by hand to the workspace file `.agents/mcp_config.json` is not touched by the uninstall; remove it yourself.

Official references: [Antigravity MCP docs](https://antigravity.google/docs/mcp/), [Where does Antigravity look for MCP servers?](https://atamel.dev/posts/2026/07-10_where_agy_mcp_servers/).

---

## 💡 Recommended Workflows

### Starting a development session

At the beginning of each session, load the plugin context:

```
I'm working on local_myplugin. Load the full context.
```

Antigravity CLI will call `get_plugin_info` and gain knowledge of the plugin's architecture, database, functions, and coding patterns.

### Querying the core API

```
Which core API functions should I use to check if a user
is enrolled in a course? Prefer public, non-deprecated functions.
```

Antigravity CLI will use `search_api` and return functions with signatures and source files.

### Creating new plugins with a slash command

Use the slash command directly:

```
/scaffold_plugin type="local" name="web_service_test" description="Web service test plugin" features="web services, capabilities"
```

After creating the files, generate context:

```
Generate the AI context for local_web_service_test.
```

### Pre-commit review

```
/review_plugin plugin="local/myplugin" focus="security"
```

---

## ⚠️ Troubleshooting

### First step: verify the connection

Type `/mcp` in the CLI (or open the MCP Servers panel in the IDE). If `build82` does not appear, the problem is the configuration, not your prompt.

### Server does not appear after configuring

- Confirm you edited the file the client really reads: `~/.gemini/config/mcp_config.json` (global) or `.agents/mcp_config.json` (workspace). `build82 install` writes the global one.
- Validate the JSON and make sure `command` is an absolute path.
- Workspace file ignored in the CLI? See the known issue above and use the global file.
- Restart the `agy` session or the IDE after fixing.

### Relative paths don't work

`mcp_config.json` requires **absolute paths**.

### Incorrect BUILD82_MOODLE_PATH

`BUILD82_MOODLE_PATH` must point to the directory containing `version.php`. The `doctor` tool reports an error if the installation cannot be validated.

### Stale context after changes

- **New plugin installed:** _"Regenerate all global Moodle indexes."_ → `update_indexes`
- **Changes to a plugin:** _"Regenerate the context for local_myplugin."_ → `generate_plugin_context`

---

## ➡️ Next Steps

- [Claude Desktop](./claude-desktop.md) — Anthropic desktop app (macOS, Windows and Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — Anthropic CLI, `build82 install claude`
- [OpenAI Codex](./codex.md) — OpenAI CLI with TOML configuration
- [OpenCode](./opencode.md) — open-source agent with TUI interface
- [Cursor](./cursor.md) — Cursor editor
- [Zed](./zed.md) — Zed editor
- [Cline](./cline.md) — Cline VS Code extension and CLI
- [Workflow Examples](../workflows/examples.md) — real-world use cases and ready-to-use prompts
- [Tools Reference](../../reference/tools.md) — complete parameters for all tools
- [Common Issues](../../troubleshooting/common-issues.md) — detailed troubleshooting
- [Back to Index](../../index.md)
