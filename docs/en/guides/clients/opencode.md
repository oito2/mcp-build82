🌐 [Português](../../../pt-br/guides/clients/opencode.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with OpenCode

**OpenCode** is an open-source terminal-based coding agent with native MCP server support. Its TUI (Text User Interface) enables a fully terminal-based workflow, similar to Claude Code.

---

## 🛠️ Installing OpenCode

### Check if already installed

Before installing, verify whether OpenCode is already available on your system:

```bash
which opencode && opencode --version
```

If a path and version are displayed, OpenCode is already installed — skip to the [MCP Server Configuration](#️-mcp-server-configuration) section.

### Install via npm (recommended)

```bash
npm install -g opencode-ai
```

Verify the installation:

```bash
opencode --version
```

### Install via official script

```bash
curl -fsSL https://opencode.ai/install | bash
```

---

## ⚙️ MCP Server Configuration

### Automatic setup

```bash
build82 install opencode
```

What it does, exactly:

1. Checks that OpenCode is detected (the `opencode` command is on `PATH`); otherwise it prints `Skipped: OpenCode not detected.` and changes nothing.
2. Asks for the Moodle root (offers the current directory if it looks like a Moodle root).
3. Merges a `build82` entry into `~/.config/opencode/opencode.json` (or the existing `~/.config/opencode/opencode.jsonc` when there is no `opencode.json`) (top-level `mcp` object), keeping every other entry and the file's permissions. If the file does not exist it is created.
4. Registers the absolute path of the running `build82` binary as the command (stdio) and sets a single environment variable, `BUILD82_MOODLE_PATH`.

If the file contains comments or trailing commas (JSONC), build82 reads it but never rewrites it: the command fails with `OpenCode... failed: ...`, the file stays unchanged, and the message includes the exact `build82` snippet to paste by hand into its `mcp` object. A file that is not valid JSON at all also aborts without writing.

Resulting entry:

```json
{
  "mcp": {
    "build82": {
      "type": "local",
      "command": ["/usr/local/bin/build82"],
      "environment": { "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle" },
      "enabled": true
    }
  }
}
```

Because `opencode.jsonc` files usually carry comments, the installer will refuse to rewrite them and print the snippet to paste instead. Project files (`opencode.json` at the project root) are never written.

### Manual setup

OpenCode accepts JSON or JSONC and merges configs: global first, then project (project wins).

| Scope | File |
|-------|------|
| Global | `~/.config/opencode/opencode.json` |
| Project | `opencode.json` at the project root |

```json
{
  "$schema": "https://opencode.ai/config.json",
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

> **Note:** `command` must be an **array** (not a string) and the variables key is `environment`. A separate `args` field is not supported; put extra arguments in the `command` array.

Use the absolute path of `build82` as the first element of `command` if OpenCode does not inherit your shell's `PATH` (`which build82`).

### Verify

```bash
opencode mcp list
```

`build82` must be listed. You can also start `opencode` in your Moodle directory and check the active MCP servers inside the session.

### Remove

```bash
build82 uninstall opencode
```

This deletes only the `build82` key from `~/.config/opencode/opencode.json`, `~/.config/opencode/opencode.jsonc` and the legacy `~/.config/opencode/config.json`. Other entries are untouched and a missing file is a no-op. It prints `OpenCode... removed.`, or `OpenCode... not registered.` (exit code 0) when there was nothing to remove. A file with comments or trailing commas (JSONC) is never rewritten: the command fails with `OpenCode... failed: ...` asking you to remove the entry manually. Run `build82 uninstall` without a target to find and remove every build82 registration at once (it asks for confirmation).

Entries you added by hand to a project `opencode.json` must be deleted manually.

Official references: [MCP servers](https://opencode.ai/docs/mcp-servers/), [Config](https://opencode.ai/docs/config/).

---

## 📄 Enhancing with AGENTS.md

OpenCode automatically reads `AGENTS.md` to load project context at each session.

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
# Navigate to the Moodle directory before starting OpenCode
cd /home/user/workspace/www/html/moodle
opencode
```

Starting from the Moodle directory ensures the project `AGENTS.md` is loaded.

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

### Server not appearing after configuration

OpenCode reads `opencode.json` at startup. After editing the file, restart the session.

Also verify the JSON is well-formed — a syntax error prevents the entire configuration from loading:

```bash
python3 -c "import json; json.load(open('opencode.json'))" && echo "Valid JSON"
```

### `build82` not found

If OpenCode does not inherit the shell PATH, use the absolute path in the `command` field (see the configuration section above).

```bash
# Find the correct path
which build82
# → /usr/local/bin/build82
```

### Stale context after changes

- **Changes to a plugin:** _"Regenerate the context for local_myplugin."_
- **New plugin installed:** _"Regenerate all global Moodle indexes."_

---

## ➡️ Next Steps

- [Claude Desktop](./claude-desktop.md) — Anthropic desktop app (macOS, Windows and Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — Anthropic CLI, `build82 install claude`
- [OpenAI Codex](./codex.md) — OpenAI CLI with TOML configuration
- [Antigravity (IDE and CLI)](./antigravity.md) — Google IDE and terminal agent
- [Cursor](./cursor.md) — Cursor editor
- [Zed](./zed.md) — Zed editor
- [Cline](./cline.md) — Cline VS Code extension and CLI
- [Workflow Examples](../workflows/examples.md) — real-world use cases and ready-to-use prompts
- [Tools Reference](../../reference/tools.md) — complete parameters for all tools
- [Common Issues](../../troubleshooting/common-issues.md) — detailed troubleshooting
- [Back to Index](../../index.md)
