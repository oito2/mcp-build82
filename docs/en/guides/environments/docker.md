🌐 [Português](../../../pt-br/guides/environments/docker.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Docker

`build82` is a single static binary, which makes it flexible in containerized environments. If your Moodle installation runs in Docker (Nginx + PHP-FPM + MariaDB, or any similar stack), run `build82` on the host, next to your AI client, and let it read the Moodle files from the mounted volume. build82 talks to the client over stdio, so it runs where the client runs; there is no network mode to put it in its own container (the former `--http` mode was removed in v1.2.0).

---

## 🚀 build82 on the Host

In this setup, `build82` runs directly on your host machine while Moodle runs inside containers, with Claude Code, Cursor, VS Code or any other local AI client.

### Why use it this way?

- **Performance:** File reading without Docker filesystem overhead (especially on macOS and Windows).
- **Simplicity:** The AI assistant running on the host can access the `build82` binary directly — no network configuration, no PATH-inside-a-container issues.
- **Persistence:** Context `.md` files are written to the host-mounted volume (under `.build82/`) and become immediately visible inside the Moodle container.

---

### How it works

If your Moodle directory lives at, say:

```
~/workspace/www/html/<project>/
```

and is bind-mounted into your PHP containers, `build82` running on the host reads these files directly as a normal local directory — Docker doesn't need to be involved at all.

---

#### AI Client Configuration (Claude Code)

```bash
build82 install claude
```

or manually:

```bash
claude mcp add build82 \
  -e BUILD82_MOODLE_PATH=/home/user/workspace/www/html/moodle \
  -- build82
```

#### AI Client Configuration (Antigravity CLI — `~/.gemini/config/mcp_config.json`)

> `build82 install antigravity` writes a different file than the one shown here; see the [Antigravity guide](../clients/antigravity.md) before relying on it. The other clients (Codex, Cursor, Zed, Cline) are covered in their own [guides](../clients/codex.md).

```json
{
    "mcpServers": {
        "build82": {
            "command": "build82",
            "args": [],
            "env": {
                "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle"
            }
        }
    }
}
```

#### AI Client Configuration (OpenCode — `opencode.json` at the Moodle root)

```json
{
    "$schema": "https://opencode.ai/config.json",
    "mcp": {
        "build82": {
            "type": "local",
            "command": ["build82"],
            "environment": {
                "BUILD82_MOODLE_PATH": "/home/user/workspace/www/html/moodle"
            }
        }
    }
}
```

---

#### Initializing the context

Once the client is configured, ask your AI assistant:

```
Initialize the build82 context for my Moodle installation.
```

The assistant will call `init_moodle_context`, detect the Moodle version, and generate all global indexes — without any additional terminal commands.

---

## 🛠️ Resolving Permission Conflicts

When running `build82` on the host while Moodle runs inside Docker containers, you may encounter write permission issues in plugin directories if containers create files (e.g. Moodle uploads) under a different UID than your host user.

### Fix

```bash
# Check plugin file ownership
ls -la ~/workspace/www/html/moodle/local/

# Adjust ownership if needed
sudo chown -R $USER:$USER ~/workspace/www/html/moodle/local/
```

---

## ➡️ Next Steps

- [Claude Code](../clients/claude-code.md) — detailed configuration
- [Antigravity CLI](../clients/antigravity.md) — detailed configuration
- [OpenAI Codex](../clients/codex.md) — detailed configuration
- [OpenCode](../clients/opencode.md) — detailed configuration
- [Common Issues](../../troubleshooting/common-issues.md) — permission and PATH errors
- [Back to Index](../../index.md)
