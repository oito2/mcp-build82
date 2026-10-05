🌐 [Português](../../../pt-br/guides/clients/claude-code.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Claude Code

**Claude Code** is Anthropic's CLI for AI-assisted development. It is one of the most efficient MCP clients for Moodle development, enabling a fully terminal-based workflow with native MCP protocol support via stdio.

---

## 🛠️ Initial Setup

### 1. Add the server

**Automatic** — let build82 configure it:

```bash
build82 install claude
```

This requires the `claude` command on `PATH` (otherwise it prints `Skipped: Claude Code not detected.` and changes nothing), asks for the Moodle root, and then runs exactly:

```bash
claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=<moodle-root> -- <absolute-path-to-build82>
```

The server is registered in Claude Code's **user** scope: available in all your projects and stored privately in `~/.claude.json`. Use the manual commands below for the `local` or `project` scope. This target configures Claude Code only; the Claude Desktop app has its own target (see [Claude Desktop](./claude-desktop.md)).

Running the command again replaces the existing registration, so it also updates the binary path and `BUILD82_MOODLE_PATH`. Before `claude mcp add`, the installer runs `claude mcp get build82` and removes whatever that reports:

| Registration found | What the installer does |
|--------------------|-------------------------|
| none | Adds it and prints `Claude Code... configured.` |
| `user` or `local` scope | Removes it with `claude mcp remove --scope <scope> build82`, checks again, adds the user-scope registration and prints `Claude Code... updated.` |
| `project` scope | Leaves `.mcp.json` unchanged (it is shared with the project), removes a `user` registration if there is one, adds the user-scope registration and prints a warning with the manual command `claude mcp remove --scope project build82` |

`claude mcp get` reports only the registration that takes precedence (local, then project, then user), and the `local` and `project` scopes belong to a directory, so they are detected only for the directory you run the installer from. If the scope in the output cannot be recognized, the installer changes nothing and prints `Claude Code... failed: ...`.

**Manual** — pick the scope you need (adjust paths to your machine):

| Scope | Available in | Shared | Stored in |
|-------|--------------|--------|-----------|
| `local` (default) | current project only | no | `~/.claude.json` |
| `project` | current project only | yes, via version control | `.mcp.json` at the project root |
| `user` | all your projects | no | `~/.claude.json` |

```bash
# local (default): this project, private
claude mcp add build82 \
  -e BUILD82_MOODLE_PATH=/home/user/workspace/www/html/moodle \
  -- /usr/local/bin/build82

# user: every project
claude mcp add --scope user build82 \
  -e BUILD82_MOODLE_PATH=/home/user/workspace/www/html/moodle \
  -- /usr/local/bin/build82

# project: shared through a committed .mcp.json
claude mcp add --scope project build82 \
  -e BUILD82_MOODLE_PATH=/home/user/workspace/www/html/moodle \
  -- /usr/local/bin/build82
```

The project scope creates `.mcp.json` at the project root:

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

Claude Code asks each user to approve a project-scoped server the first time. Avoid committing a machine-specific `BUILD82_MOODLE_PATH`. When the same server name exists in several scopes, local overrides project, which overrides user.

### 2. Verify the connection

```bash
claude mcp list
```

`build82` must appear as connected (`claude mcp get build82` shows its details). Inside a session, run `/mcp` to see it with all 13 tools available. If it does not appear, see [Troubleshooting](#️-troubleshooting).

### 3. Remove

```bash
build82 uninstall claude
```

This runs `claude mcp get build82`, reads the `Scope:` line of its output, and removes that registration with `claude mcp remove --scope <scope> build82`, repeating until nothing is left in the `user` and `local` scopes. It prints `Claude Code... removed.`, or `Claude Code... not registered.` (exit code 0) when nothing is registered, and prints `Skipped: Claude Code not detected.` if `claude` is not on `PATH`.

A registration in the `project` scope (`.mcp.json`, shared with everyone on the project) is never modified. The uninstall still removes the `user` registration, if any, and prints a warning with the command to run yourself from the project directory:

```bash
claude mcp remove --scope project build82
```

When the `project` registration was the only one, it prints `Claude Code... nothing removed.` followed by that warning, and exits with code 0. The `local` and `project` scopes are per directory: run `build82 uninstall claude` from the project directory to find them.

### 4. Initialize the Moodle context

In the first session after setup, ask Claude:

```
Initialize the build82 context for my Moodle installation.
```

Claude will call `init_moodle_context`, detect the Moodle version, and generate all global indexes. This step only needs to be done once per installation.

Official reference: [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp).

---

## 📄 Enhancing with CLAUDE.md

Claude Code automatically reads the `CLAUDE.md` file at the project root when starting each session — eliminating the need to re-explain the environment every time.

Create the file at the root of your Moodle installation:

```bash
touch /home/user/workspace/www/html/moodle/CLAUDE.md
```

**Recommended template:**

```markdown
# Moodle Plugin Development Context

## Environment
- Moodle version: 4.4 (adjust to match your installation)
- Path: /home/user/workspace/www/html/moodle
- Stack: Docker with Nginx + PHP-FPM + MariaDB

## build82
The build82 MCP server is connected and indexes have been generated.
Available tools: init_moodle_context, generate_plugin_context, plugin_batch,
update_indexes, watch_plugins, search_plugins, search_api, get_plugin_info,
list_dev_plugins, doctor, explain_plugin, release_plugin.

## Plugins under development
- local_myplugin — briefly describe the purpose

## Conventions
- Code standard: Moodle Coding Style (PSR-12 + Frankenstyle)
- All database access via $DB — never direct SQL
- All output via $OUTPUT or renderers — never echo directly
- Capabilities always checked with require_capability() or has_capability()

## Workflow
1. Before working on a plugin, load its context with get_plugin_info.
2. Use search_api before suggesting core functions — prefer documented APIs.
3. After adding new plugins to the installation, run update_indexes.
4. After significant changes to a plugin, run generate_plugin_context.
```

---

## 💡 Recommended Workflows

### Starting a development session

At the beginning of each session, load the context of the plugin you will work on:

```
I'm working on the local_myplugin plugin. Load the full context.
```

Claude will call `get_plugin_info` and gain knowledge of the plugin's architecture, database schema, functions, events, and coding patterns.

### Searching the core API

Instead of opening a browser to check the official documentation:

```
Which core API functions should I use to handle grade persistence?
Prefer public, non-deprecated functions.
```

Claude will use `search_api` and return functions with signatures, source files, and version info (`@since`).

### Creating new plugins

Use the `scaffold_plugin` prompt to generate a complete plugin structure:

```
scaffold_plugin
  type="block"
  name="student_monitor"
  description="Displays a student activity summary for teachers"
  features="capabilities, caching"
```

After the files are created, generate context so Claude understands the new plugin:

```
Generate the AI context for block_student_monitor and explain
the generated class structure.
```

### Debugging errors

Paste the error directly into the chat:

```
I'm getting this error in Moodle:

[PASTE ERROR HERE]

Load the context for local_myplugin and help me identify
the root cause and fix.
```

### Enabling watch mode during development

To have the context update automatically as you code:

```
Start monitoring local_myplugin for file changes.
```

Claude will call `watch_plugins action="start"`. Any `db/*.php` or `version.php` file saved in the plugin triggers a background context update.

### Packaging a plugin for distribution

```
Package the local_myplugin plugin as a ZIP ready to distribute.
```

Claude will call `release_plugin`, excluding build82's own generated files from the archive.

---

## ⚠️ Troubleshooting

### First step: verify the connection

Before any other investigation, run `/mcp` inside the Claude session. If `build82` does not appear as connected, the problem is in the server configuration, not your prompt.

### `build82: command not found`

Claude Code does not inherit your shell's PATH in every environment. Find the absolute path and provide it explicitly:

```bash
# Find the correct path
which build82
# → /usr/local/bin/build82

# Re-add the server with the absolute path
claude mcp remove build82
claude mcp add build82 \
  -e BUILD82_MOODLE_PATH=/home/user/workspace/www/html/moodle \
  -- /usr/local/bin/build82
```

### Permission errors

If Claude Code reports it cannot run the server, verify the binary is executable:

```bash
ls -l $(which build82)
chmod +x $(which build82)
```

### Server connected but tools don't respond

Run the diagnostic by asking Claude:

```
Run the build82 doctor.
```

Claude will call the `doctor` tool and return a report with server status, Moodle version, index freshness, and cache stats.

### Stale context after changes

- **New plugin installed in Moodle:** _"Regenerate all global Moodle indexes."_ → `update_indexes`
- **Significant changes to a plugin:** _"Regenerate the context for local_myplugin."_ → `generate_plugin_context`

---

## ➡️ Next Steps

- [Claude Desktop](./claude-desktop.md) — Anthropic desktop app (macOS, Windows and Linux beta), `build82 install claude-desktop`
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
