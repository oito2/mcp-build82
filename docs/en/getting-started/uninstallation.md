🌐 [Português](../../pt-br/getting-started/uninstallation.md) | **English** | 🏠 [Index](../index.md)

---

# Uninstallation

Installing and using build82 leaves files in several places. This page lists all of them, what `build82 uninstall` removes automatically, and what you must remove by hand. Recommended order: unregister from MCP clients, optionally purge, then delete the binary.

| Item | Location | Removed by `build82 uninstall` | Removed by `--purge` |
| :--- | :--- | :---: | :---: |
| MCP client registrations | Per client, see [below](#mcp-client-registrations) | Yes | No |
| Config file | `~/.build82` | No | Yes |
| Global generated Markdown files (13) | `<moodle_root>/.build82/*.md` | No | Yes |
| Per-plugin Markdown files (12) and `.indevelopment` marker | `<plugin_root>/.build82/` of `.indevelopment`-marked plugins | No | Yes |
| `tags`, `.cache.json`, empty `.build82/` directories | `<moodle_root>/.build82/` | No | No |
| `.build82/` directories of plugins not marked `.indevelopment` | `<plugin_root>/.build82/` | No | No |
| Binary | See [Removing the binary](#removing-the-binary) | No | No |
| `.bak` backup from `self-update` | Next to the binary | No | No |

---

## `build82 uninstall`

```bash
build82 uninstall [target] [--purge]
```

| Argument | Description |
| :--- | :--- |
| `target` | Optional. The first argument that does not start with `-`. One of `claude`, `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`. Omitted: all registrations found. |
| `--purge` | Optional, may appear anywhere. After unregistering, also deletes generated files and the config file, with its own separate confirmation. |

Extra positional arguments after the first are ignored. An unknown `target` fails with `unknown target "<id>" — supported targets: <list>` and exit code 1.

### Without a target

1. build82 checks every supported client for an existing `build82` entry. It does not require the client to be detected as installed.
   - Claude Code and Codex: run `<tool> mcp get build82`; the tool counts as registered when the command succeeds (for Claude Code this also matches the `local` and `project` scopes of the current directory). If the tool is missing or the command fails, it is treated as not registered.
   - File-based clients: any of the configuration files listed [below](#what-is-removed-per-client) must exist, be readable as JSON (comments and trailing commas are tolerated for reading) and contain a `build82` key under the client's top-level key.
2. No entries found: prints `No build82 registrations found.`
3. Otherwise lists the clients and asks `Remove build82 from all N tool(s)? [y/N]`. Only `y` (any case) confirms; anything else, including an empty answer, exits without changes and **without running `--purge`**.
4. Removes each entry, printing `<Label>... removed.` or `<Label>... failed: <error>` (for Claude Code, `<Label>... nothing removed.` when only a `project` registration was found, followed by a warning; see [below](#claude-code-scopes)). A failure on one client does not stop the others and does not change the exit code.

### With a target

- No confirmation prompt.
- Claude Code and Codex: if the tool is not found in `PATH`, prints `Skipped: <Label> not detected.` (exit code 0). Otherwise it runs `<tool> mcp get build82`; if that reports no registration it prints `<Label>... not registered.` (exit code 0), else it runs the removal command (see the table below). A Claude Code `project` registration is never removed: it prints a warning and exits with code 0 (see [Claude Code scopes](#claude-code-scopes)). On failure it prints `<Label>... failed: <error>` and exits with code 1 (`--purge` does not run).
- File-based clients: no detection check. Removes the `build82` key from every file listed for the target. Prints `<Label>... removed.` when at least one entry was deleted, or `<Label>... not registered.` (exit code 0) when none was found. Invalid JSON in one of the files is an error (`<Label>... failed: <error>`, exit code 1).
- A file with comments or trailing commas (JSONC, common in Zed's `settings.json` and OpenCode's `opencode.jsonc`) is never rewritten: if it holds a `build82` entry the command fails asking you to remove that entry manually, and the file stays unchanged.

### What is removed per client

Only the `build82` key is deleted; the rest of the file is preserved (file permissions are kept). A file is never created if missing.

| Target (label) | Registration removed |
| :--- | :--- |
| `claude` (Claude Code) | `claude mcp remove --scope <scope> build82` for each `user` and `local` registration; a `project` registration is left unchanged with a warning |
| `claude-desktop` (Claude Desktop) | `mcpServers.build82` in `claude_desktop_config.json` (macOS: `~/Library/Application Support/Claude/`, Windows: `%APPDATA%\Claude\`, Linux: `~/.config/Claude/`) |
| `antigravity` (Antigravity (IDE / CLI)) | `mcpServers.build82` in `~/.gemini/config/mcp_config.json` and in the legacy `~/.gemini/antigravity/mcp_config.json` |
| `codex` (OpenAI Codex CLI) | `codex mcp remove build82` |
| `opencode` (OpenCode) | `mcp.build82` in `~/.config/opencode/opencode.json`, `~/.config/opencode/opencode.jsonc` and the legacy `~/.config/opencode/config.json` |
| `cursor` (Cursor) | `mcpServers.build82` in `~/.cursor/mcp.json` |
| `zed` (Zed) | `context_servers.build82` in `~/.config/zed/settings.json` (Windows: `%APPDATA%\Zed\settings.json`) |
| `cline` (Cline (VS Code extension / CLI)) | `mcpServers.build82` in `<globalStorage>/settings/cline_mcp_settings.json` and in `~/.cline/data/settings/cline_mcp_settings.json` (`$CLINE_DATA_DIR/settings/cline_mcp_settings.json` when `CLINE_DATA_DIR` is set) |

Cline `<globalStorage>` (stable VS Code):

| OS | Path |
| :--- | :--- |
| Linux | `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev` |
| macOS | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev` |
| Windows | `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev` (`%USERPROFILE%\AppData\Roaming` if `APPDATA` is unset) |

### Claude Code scopes

`claude mcp get build82` reports only the registration that takes precedence (`local`, then `project`, then `user`), in a line such as `Scope: User config (available in all your projects)`. The uninstall reads that scope, removes the registration with `claude mcp remove --scope <scope> build82`, and asks again until nothing is left in the `user` and `local` scopes. If the scope cannot be recognized, nothing is removed and the uninstall fails.

The `local` and `project` scopes belong to a directory, so they are only found when you run `build82 uninstall` from that project's directory.

A `project` registration lives in the project's `.mcp.json`, shared with everyone on the project, so build82 never modifies it. The uninstall still removes a `user` registration hidden behind it, then prints a warning with the command to run yourself from the project directory, and exits with code 0:

```bash
claude mcp remove --scope project build82
```

### `--purge`

Runs after the unregistration step (and only if it did not fail with an error). It loads the configuration ([precedence](../reference/configuration.md#precedence)) and prints what it will delete:

```text
--purge will delete:
  - config file: /home/you/.build82
  - 14 generated file(s) under .build82/
This cannot be undone. Proceed? [y/N]
```

The config-file line appears only if `~/.build82` exists. Only `y` (any case) proceeds; otherwise prints `Purge cancelled.` and exits 0.

Deleted:

| What | Details |
| :--- | :--- |
| `~/.build82` | The config file. |
| Global files | Each of the 13 files that exists in `<moodle_root>/.build82/`: `AI_CONTEXT.md`, `MOODLE_API_INDEX.md`, `MOODLE_EVENTS_INDEX.md`, `MOODLE_TASKS_INDEX.md`, `MOODLE_SERVICES_INDEX.md`, `MOODLE_DB_TABLES_INDEX.md`, `MOODLE_CLASSES_INDEX.md`, `MOODLE_CAPABILITIES_INDEX.md`, `MOODLE_PLUGIN_INDEX.md`, `MOODLE_DEV_RULES.md`, `MOODLE_PLUGIN_GUIDE.md`, `MOODLE_AI_WORKSPACE.md`, `MOODLE_AI_INDEX.md`. |
| Plugin files | For every plugin with a `.build82/.indevelopment` marker under the Moodle root: each of the 12 `PLUGIN_*.md` files that exists, plus the `.indevelopment` marker. See [Generated Files](../reference/generated-files.md). |

Not deleted: `.build82/tags`, `.build82/.cache.json`, the `.build82/` directories themselves, files of plugins without the `.indevelopment` marker, legacy files at the Moodle or plugin root (pre-`.build82/` layout), the binary, the `.bak` backup, and client registrations. If no configuration can be resolved, only the config file is considered. A file that fails to delete is reported (`failed to remove <path>: <error>`) and the rest continue; the exit code stays 0.

---

## Manual removal

### MCP client registrations

Use `build82 uninstall` where possible. Registrations outside the locations listed above (for example a project-scoped `.mcp.json`, a project-level `.cursor/mcp.json`, `.zed/settings.json` or `.agents/mcp_config.json`, or a hand-written entry in another file or format) are not found by it; remove those in the client. Claude Code `project` scope entries are reported with the command that removes them (see [Claude Code scopes](#claude-code-scopes)). Each entry created by `build82 install` sets `BUILD82_MOODLE_PATH` in its `env`, so deleting the entry also removes that variable. See the [client guides](../guides/clients/claude-code.md).

### Removing the binary

| OS | Steps |
| :--- | :--- |
| Linux, macOS | `sudo rm /usr/local/bin/build82` (or the directory you chose, e.g. `~/.local/bin/build82`). Find it with `command -v build82`. |
| Windows | `Remove-Item -Recurse "$env:LOCALAPPDATA\build82"`, then remove `%LOCALAPPDATA%\build82` from the user `Path` (below). |
| Built with `go install` | `rm "$(go env GOPATH)/bin/build82"` |
| Built with `go build` | Delete the binary you produced. |

Remove the `PATH` entry on Windows (PowerShell):

```powershell
$p = [Environment]::GetEnvironmentVariable("Path", "User") -split ';' | Where-Object { $_ -and $_ -ne "$env:LOCALAPPDATA\build82" }
[Environment]::SetEnvironmentVariable("Path", ($p -join ';'), "User")
```

### The `.bak` backup

`build82 self-update` renames the previous binary to `<binary path>.bak` (for example `/usr/local/bin/build82.bak`). Delete it together with the binary. Once deleted, `build82 self-update --rollback` is no longer possible.

### The config file

```bash
rm ~/.build82
```

Windows: `Remove-Item "$env:USERPROFILE\.build82"`. Written by the `init_moodle_context` tool; see [Configuration Reference](../reference/configuration.md).

### `.build82/` directories

Located at the Moodle root and in each plugin root. Generated content is regenerable, so deleting is safe. Nothing else in these directories belongs to your code.

```bash
# Moodle root: global files, tags, .cache.json
rm -rf /path/to/moodle/.build82

# Every plugin root
find /path/to/moodle -type d -name .build82 -prune -exec rm -rf {} +
```

`.indevelopment` markers live inside these directories (`<plugin_root>/.build82/.indevelopment`), so removing the directories also unmarks the plugins. Plugins from an earlier layout may still have `PLUGIN_*.md` files or `.indevelopment` directly at the plugin root, and global files or `tags` directly at the Moodle root; delete those by hand.

Files that build82 does not create for you and does not delete: a `.buildignore` you wrote in a plugin root, ZIPs produced by `release_plugin` (`<component>_<version>.zip`, in `output_dir` or the working directory), and plugins created by the scaffold tool.

### Editor `tags` setting

If you added `set tags=./.build82/tags;` to your editor configuration (see the README), remove it.

---

## Related

- [Installation](./installation.md)
- [Configuration Reference](../reference/configuration.md)
- [CLI Reference](../reference/cli.md)
