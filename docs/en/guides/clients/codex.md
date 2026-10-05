🌐 [Português](../../../pt-br/guides/clients/codex.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with OpenAI Codex

**OpenAI Codex** is OpenAI's development agent, available as a CLI (`codex`) and as a VS Code extension. Unlike other clients, Codex uses **TOML** for configuration and an **`AGENTS.md`** file for persistent context — instead of JSON and `CLAUDE.md`/`GEMINI.md`.

> **Important:** The CLI and the VS Code extension share the same configuration file `~/.codex/config.toml`. Configuring it once applies to both.

---

## 🛠️ MCP Server Configuration

### Option 1 — Let build82 configure it

```bash
build82 install codex
```

Requires the `codex` command on `PATH` (otherwise it prints `Skipped: OpenAI Codex CLI not detected.` and changes nothing). It asks for the Moodle root and then runs exactly:

```bash
codex mcp add build82 --env BUILD82_MOODLE_PATH=<moodle-root> -- <absolute-path-to-build82>
```

So Codex itself writes the entry to `~/.codex/config.toml`, the same result as Option 2. It prints `OpenAI Codex CLI... configured.` Running the command again replaces the existing registration: when `codex mcp get build82` reports one, the installer first runs `codex mcp remove build82` and then adds it again with the current binary path and `BUILD82_MOODLE_PATH`, printing `OpenAI Codex CLI... updated.`

### Option 2 — Via the Codex CLI

```bash
codex mcp add build82 \
  --env BUILD82_MOODLE_PATH=/home/user/workspace/www/html/moodle \
  -- /usr/local/bin/build82
```

This writes the entry to `~/.codex/config.toml`. Verify it:

```bash
codex mcp list
```

You can also run `/mcp` inside the Codex TUI to see active servers.

### Option 3 — Editing config.toml directly

Global (all projects) — create or edit `~/.codex/config.toml`:

```toml
[mcp_servers.build82]
command = "/usr/local/bin/build82"
args    = []
env     = { BUILD82_MOODLE_PATH = "/home/user/workspace/www/html/moodle" }
```

Project scope — the same table in `.codex/config.toml` at the workspace root (only loaded for trusted projects).

> **Watch out for TOML syntax:** a syntax error in `config.toml` breaks **both** the CLI and the VS Code extension simultaneously. Check it with:
> ```bash
> python3 -c "import tomllib; tomllib.load(open('/home/user/.codex/config.toml', 'rb'))"
> ```

Use the absolute path for `command` if Codex does not inherit your shell's `PATH` (`which build82`).

### Remove

```bash
build82 uninstall codex
```

This runs `codex mcp get build82` to check that a registration exists and, only if it does, `codex mcp remove build82`. It prints `OpenAI Codex CLI... removed.`, or `OpenAI Codex CLI... not registered.` (exit code 0) when nothing is registered, and prints `Skipped: OpenAI Codex CLI not detected.` if `codex` is not on `PATH`. You can also run `codex mcp remove build82` yourself, or delete the `[mcp_servers.build82]` table from `config.toml`.

Official reference: [Codex MCP documentation](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

---

## 📄 Enhancing with AGENTS.md

`AGENTS.md` is the Codex equivalent of `CLAUDE.md` and `GEMINI.md`. It is read automatically at each session — from the most general scope to the most specific:

| Scope | Location |
|-------|----------|
| Global | `~/.codex/AGENTS.md` |
| Project root | `AGENTS.md` at the workspace root (Git root) |
| Subdirectory | `AGENTS.md` in any project subfolder |

Codex loads files in cascade — from global to most specific — and the closest to the current directory takes precedence.

Create the file at the root of your Moodle installation:

```bash
touch /home/user/workspace/www/html/moodle/AGENTS.md
```

**Recommended template:**

```markdown
# Moodle Plugin Development Context

## Environment
- Moodle version: 4.4 (adjust to match your installation)
- Path: /home/user/workspace/www/html/moodle
- Stack: Docker with Nginx + PHP-FPM + MariaDB

## build82
The build82 MCP server is configured.
Use get_plugin_info to load context before analyzing a plugin.
Use search_api to find core functions before suggesting alternatives.
After significant changes to a plugin, run generate_plugin_context.
After installing new plugins in Moodle, run update_indexes.

## Plugins under development
- local_myplugin — briefly describe the purpose

## Conventions
- Code standard: Moodle Coding Style (PSR-12 + Frankenstyle)
- All database access via $DB — never direct SQL
- All output via $OUTPUT or renderers — never echo directly
- Capabilities always checked with require_capability() or has_capability()
```

---

## 💡 Recommended Workflows

### Starting a session

```bash
# Navigate to the Moodle directory before starting Codex
cd /home/user/workspace/www/html/moodle
codex
```

Starting from the Moodle directory ensures the project `AGENTS.md` is loaded and Codex understands the workspace context.

In the first session after setup:

```
Initialize the build82 context for this Moodle installation.
```

### Loading plugin context

```
Load the context for local_myplugin and give me a summary
of the architecture, database, and main functions.
```

### Searching the core API

```
Use the search_api tool to find Moodle core API functions
related to enrollment that are not deprecated.
```

### Creating a new plugin

```
scaffold_plugin
  type="local"
  name="audit_log"
  description="Audit log for user actions"
  features="database tables, scheduled tasks, capabilities, event observers"
```

### Pre-commit review

```
/review_plugin plugin="local/myplugin" focus="security"
```

---

## ⚠️ Troubleshooting

### Server not appearing after adding

Codex reads `config.toml` at startup. After editing the file, restart the session:

```bash
exit
codex
```

In the VS Code extension, reload the window: `Ctrl+Shift+P` → **Developer: Reload Window**.

### TOML syntax error breaks CLI and VS Code simultaneously

This is a characteristic of the shared configuration. If both stop working after an edit, the problem is almost certainly invalid TOML syntax. Check:

- Strings must use double quotes: `"value"`, not `'value'`
- Arrays use brackets: `args = []`
- Section name must be exact: `[mcp_servers.build82]`

### SSE is not supported

`build82` runs over stdio by default, which is what all the options above register. The server's `--http` mode (Streamable HTTP + SSE) is a separate, opt-in mode and is not configured by any of the steps on this page.

### Stale context after changes

- **Changes to a plugin:** _"Regenerate the context for local_myplugin."_
- **New plugin installed:** _"Regenerate all global Moodle indexes."_

---

## ➡️ Next Steps

- [Claude Desktop](./claude-desktop.md) — Anthropic desktop app (macOS, Windows and Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — Anthropic CLI, `build82 install claude`
- [Antigravity (IDE and CLI)](./antigravity.md) — Google IDE and terminal agent
- [OpenCode](./opencode.md) — open-source agent with TUI interface
- [Cursor](./cursor.md) — Cursor editor
- [Zed](./zed.md) — Zed editor
- [Cline](./cline.md) — Cline VS Code extension and CLI
- [Workflow Examples](../workflows/examples.md) — real-world use cases and ready-to-use prompts
- [Tools Reference](../../reference/tools.md) — complete parameters for all tools
- [Common Issues](../../troubleshooting/common-issues.md) — detailed troubleshooting
- [Back to Index](../../index.md)
