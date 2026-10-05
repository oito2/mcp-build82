# Documentation — build82 🎓

🌐 [Português](../pt-br/index.md) | **English** | 🏠 [Back to README](../../README.md)

---

**build82** is an MCP server (Model Context Protocol) designed to expose Moodle's internal structure to AI assistants, enabling faster, safer and more standardized plugin development. It's distributed as a single static binary.

Use this index to navigate through the entire documentation.

---

## 🚀 Getting Started

Set up your environment and get the server running in minutes.

- [Installation](./getting-started/installation.md) — System requirements and installation methods (release binary or `go install`).
- [Quickstart](./getting-started/quickstart.md) — Your first command and how to synchronize the AI with your Moodle.
- [Creating your First Plugin](./getting-started/first-plugin.md) — Using the scaffold to start a project from scratch.
- [Uninstallation](./getting-started/uninstallation.md) — Removing client registrations, generated files, config and the binary.

---

## 🧠 Concepts

Understand what MCP is, why this server exists and how it works internally.

- [What is MCP?](./concepts/what-is-mcp.md) — Introduction to the Model Context Protocol.
- [Why build82?](./concepts/why-build82.md) — The problem the server solves and when to use it.
- [How the server works](./concepts/how-build82-works.md) — The flow between Extractors, Generators and your AI.
- [Architecture](./concepts/architecture.md) — Overview of components and data flow.
- [Glossary](./concepts/glossary.md) — Terms and concepts used in this documentation.

---

## 📖 Guides

### MCP Clients

Configure the server in your preferred AI assistant.

- [Claude Code](./guides/clients/claude-code.md) — CLI registration with local, project and user scopes.
- [Claude Desktop](./guides/clients/claude-desktop.md) — `build82 install claude-desktop` or `claude_desktop_config.json` setup.
- [Antigravity](./guides/clients/antigravity.md) — IDE and CLI, via the shared `mcp_config.json`.
- [OpenAI Codex](./guides/clients/codex.md) — `codex mcp add` or `~/.codex/config.toml`.
- [OpenCode](./guides/clients/opencode.md) — OpenCode configuration.
- [Cursor](./guides/clients/cursor.md) — Cursor MCP configuration.
- [Zed](./guides/clients/zed.md) — Zed context server configuration.
- [Cline](./guides/clients/cline.md) — Cline (VS Code extension and CLI) MCP configuration.

### Environments

- [Using Docker](./guides/environments/docker.md) — Running build82 alongside a containerized Moodle.

### Workflows

- [Usage examples](./guides/workflows/examples.md) — Real use cases and development workflows.

---

## 💬 Example Prompts

- [Example Prompts](./prompts.md) — Natural-language requests to type to your AI agent, by category, with the parameters each needs and the expected result.

---

## 🛠️ Technical Reference

Consult the resources and commands available on the server.

- [Tools](./reference/tools.md) — Actions the AI can execute (e.g. `get_plugin_info`, `search_api`, `explain_plugin`).
- [Resources](./reference/resources.md) — Context that the AI reads passively (e.g. API indexes, plugins, database and events).
- [Prompts](./reference/prompts.md) — Pre-configured prompt templates for common tasks.
- [Generated Files](./reference/generated-files.md) — The `.md` files created under `.build82/` in your Moodle installation.
- [CLI](./reference/cli.md) — Every subcommand and flag, and the stdio / HTTP transports.
- [Configuration](./reference/configuration.md) — The `~/.build82` file and every `BUILD82_*` environment variable.

---

## 🔬 Internal Architecture

For those who want to understand or contribute to the server code.

- [Extractors](./architecture/extractors.md) — How the server reads and analyzes the Moodle installation, including the regex vs. tree-sitter backend split.
- [Generators](./architecture/generators.md) — How context content is generated for the AI.
- [Cache System](./architecture/cache-system.md) — The persisted mtime-cache and invalidation strategy.

---

## 🆘 Troubleshooting

- [Common Issues](./troubleshooting/common-issues.md) — Connection errors, permission errors, and stale cache.

---

> 💡 **Tip:** If you are using Claude Code or another MCP-aware assistant, try asking directly: _"What tools does build82 provide?"_ — the AI will query the server in real time and respond with the updated list.
