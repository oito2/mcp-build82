🌐 [Português](../../pt-br/getting-started/quickstart.md) | **English** | 🏠 [Index](../index.md)

---

# Quickstart

This guide will help you connect `build82` to your AI assistant and complete your first development task in under 5 minutes.

---

## 1. Connect Your Assistant

The fastest way: let build82 configure the client for you.

```bash
build82 install
```

It auto-detects every supported client installed on your machine (Claude Code, Claude Desktop, Antigravity, OpenAI Codex, OpenCode, Cursor, Zed, Cline), lists them, prompts for your Moodle path, asks for confirmation, and writes the config itself. Pass a target explicitly (e.g. `build82 install claude`) to configure just one.

### Manual configuration

If you'd rather configure a client by hand — or need to see exactly what `install` writes — here's the equivalent for Claude Code (`install claude` registers it in Claude Code's user scope, i.e. for all your projects):

```bash
claude mcp add --scope user build82 \
  -e BUILD82_MOODLE_PATH=/var/www/html/moodle \
  -- build82
```

Verify the server was registered:

```bash
claude mcp list
# build82 should appear in the list
```

> For the other clients — including exact config file paths and JSON/TOML snippets — see the full guides:
> [Claude Code](../guides/clients/claude-code.md) · [Claude Desktop](../guides/clients/claude-desktop.md) · [Antigravity](../guides/clients/antigravity.md) · [OpenAI Codex](../guides/clients/codex.md) · [OpenCode](../guides/clients/opencode.md) · [Cursor](../guides/clients/cursor.md) · [Zed](../guides/clients/zed.md) · [Cline](../guides/clients/cline.md)

---

## 2. Initialize the Context

With the server connected, open a chat with the AI and ask it to initialize the environment. This step maps the Moodle version and generates all 13 global index files.

> **Before continuing:** confirm the Moodle path points to the correct root — the directory containing `version.php`.

**If you registered the server with `build82 install`** (or set `BUILD82_MOODLE_PATH` yourself), the server already knows the Moodle path, so `init_moodle_context` would only reply "already initialized" and generate nothing. Ask for the indexes directly — **type in the chat:**

```
Generate the build82 indexes for my Moodle installation.
```

The AI will run the `update_indexes` tool, which detects the version from `version.php` and generates the global indexes, reporting the Moodle version and how many indexes were regenerated, cached or failed.

**If no Moodle path is configured yet** (manual setup without `BUILD82_MOODLE_PATH`), pass it in the prompt instead:

```
Initialize the build82 context for the installation at /var/www/html/moodle.
```

The AI will run the `init_moodle_context` tool, save the path to `~/.build82`, and confirm the version found (e.g. Moodle 4.5) and the indexes generated.

Either way, this takes anywhere from a few seconds to a couple of minutes depending on the number of installed plugins.

---

## 3. Your First Task: Exploring the API

Now that the AI has "eyes" inside your Moodle, ask it to search for something in the official API.

**Try this prompt:**

```
Search the core API for functions related to "enrollment" that are not deprecated.
```

The AI will use the `search_api` tool and list the functions with their signatures and the files where they are defined.

---

## 4. Analyzing an Existing Plugin

If you already have a plugin under development, ask the AI to deeply understand it.

**Try this prompt:**

```
Generate the AI context for my local_caedauth plugin and give me a summary of the database tables it uses.
```

The server will create `PLUGIN_*.md` files under `.build82/` inside the plugin directory, and the AI will explain the complete structure for you.

---

## 🎯 Next Steps

Now that you are connected, explore the full potential of the server:

- **Build from scratch:** Use the [My First Plugin](./first-plugin.md) guide to scaffold a new component.
- **Fix bugs:** Ask the AI to analyze an error using the `doctor` tool or the `debug_plugin` prompt.
- **Review code:** Before a commit, ask: _"Do a code review of my plugin focused on security and Moodle standards."_
- [Back to Index](../index.md)

---

> 💡 **Pro tip:** If the AI says it doesn't know the command, be explicit: _"Use the MCP tool `search_api` to find..."_.
