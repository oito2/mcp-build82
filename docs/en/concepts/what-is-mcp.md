🌐 [Português](../../pt-br/concepts/what-is-mcp.md) | **English** | 🏠 [Index](../index.md)

---

# What is MCP?

**Model Context Protocol (MCP)** is an open standard created by Anthropic that defines how AI assistants communicate with external tools and data sources. Think of it as a universal adapter: any MCP-compatible AI client can connect to any MCP server using the same protocol.

---

## The problem MCP solves

Without MCP, each AI integration is built in a custom way:

- Cursor has its own plugin system
- VS Code has its own extension API
- Claude Desktop has its own tool format

With MCP, you build **a single server** and any compatible client can use it.

---

## How MCP works

```
AI Client (Claude Code, Gemini, Cursor...)
       │
       │  JSON-RPC via stdio or HTTP
       │
  MCP Server (build82)
       │
       │  reads from disk, parses PHP, writes context .md
       │
  Your Moodle installation
```

An MCP server exposes three types of capabilities:

| Capability    | Description                    | How the AI uses it                                                                    |
| ------------- | ------------------------------ | --------------------------------------------------------------------------------------- |
| **Tools**     | Functions that the AI can call | Explicitly invoked (`init_moodle_context`, `search_api`, `get_plugin_info`...)          |
| **Resources** | URIs that expose data          | Read passively — the client fetches them as context without explicit action             |
| **Prompts**   | Prebuilt prompt templates      | Invoked by name (`scaffold_plugin`, `review_plugin`...), automatically inject context   |

---

## MCP transports

MCP supports two transport mechanisms:

| Transport                  | How it works                                     | When to use                                                                 |
| --------------------------- | -------------------------------------------------- | ----------------------------------------------------------------------------- |
| **stdio**                  | The server runs as a subprocess of the AI client | Local use — Moodle on the same machine as your editor/CLI (the default)     |
| **HTTP (Streamable HTTP)** | The server runs as an independent HTTP service   | Remote use — Moodle on a separate server or isolated production environment |

For most developers running Moodle locally, the **stdio** transport is the default (`build82` with no flags) and does not require any additional configuration.

---

## Additional reading

- [Model Context Protocol specification](https://modelcontextprotocol.io)
- [Why build82?](./why-build82.md)
- [How build82 works](./how-build82-works.md)
- [Glossary](./glossary.md)

---

[🏠 Back to Index](../index.md)
