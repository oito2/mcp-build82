🌐 [Português](../../../pt-br/guides/environments/docker.md) | **English** | 🏠 [Index](../../index.md)

---

# Using with Docker

`build82` is a single static binary, which makes it flexible in containerized environments. If your Moodle installation runs in Docker (Nginx + PHP-FPM + MariaDB, or any similar stack), there are two main deployment scenarios.

---

## 🚀 Scenario A: build82 on the Host (Recommended)

In this setup, `build82` runs directly on your host machine while Moodle runs inside containers. This is the recommended option for most developers using Claude Code, Cursor, VS Code, or any other local AI client.

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

## 🐳 Scenario B: build82 as a Sidecar (Docker Compose)

Use this scenario when the server must be part of the infrastructure and accessible via the network — for example, for teams sharing a remote Moodle instance, using the `--http` transport.

---

### A minimal image

Since `build82` is a single static binary, a container for it just needs to fetch and run that binary — no runtime to install:

```dockerfile
FROM alpine:3.19
RUN apk add --no-cache curl ca-certificates && \
    curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_amd64 && \
    chmod +x build82_linux_amd64 && \
    mv build82_linux_amd64 /usr/local/bin/build82
ENTRYPOINT ["build82"]
```

On ARM64 hosts (e.g. Apple Silicon running Docker), fetch `build82_linux_arm64` instead. To verify the download, also fetch `checksums.txt` (see [Installation](../../getting-started/installation.md)).

### docker-compose.yml configuration

```yaml
services:
    build82:
        build: ./build82-image  # the Dockerfile above
        container_name: build82
        volumes:
            - ./www/html/moodle:/var/www/moodle
        environment:
            - BUILD82_MOODLE_PATH=/var/www/moodle
            - BUILD82_MOODLE_VERSION=4.5
        command: ["--http", "--port", "3000", "--host", "0.0.0.0", "--token", "my-secret-token"]
        ports:
            - "3000:3000"
```

> **Why not use `:ro`?**
> The server needs **write permission** to create context `.md` files under `.build82/` inside plugin directories (e.g. `PLUGIN_AI_CONTEXT.md`, `PLUGIN_DB_TABLES.md`). Mount the volume without `:ro` so context generation works correctly.

---

### Configuring the AI client for HTTP mode

After starting the container, configure your AI client to connect via URL.

The server exposes Streamable HTTP at `/mcp` and legacy SSE at `/sse`, both protected by the Bearer token; `GET /health` needs no token.

#### Claude Code

```bash
claude mcp add --transport http build82 http://localhost:3000/mcp \
  --header "Authorization: Bearer my-secret-token"
```

For other clients, use each client's own remote (HTTP) server syntax with the same URL and `Authorization: Bearer <token>` header; the field names differ per client, so check its official documentation and its [guide](../clients/claude-code.md).

> **Host header check:** the server rejects requests whose `Host` header is not `localhost`, `127.0.0.1`, `::1`, the `--host` value, or a value passed with `--allowed-host`. To reach it from another machine or container by IP or hostname, add `--allowed-host <that-name>` (repeatable) to the `command`, e.g. `"--allowed-host", "build82"` for other containers on the same compose network. The rejected request gets a 403 naming the host.
> There is no TLS: in production, use a reverse proxy (nginx, Caddy) with TLS. Instead of `--token`, you can set the `BUILD82_TOKEN` environment variable to keep the secret out of the process list.

---

## 🛠️ Resolving Permission Conflicts

When running `build82` on the host (Scenario A) while Moodle runs inside Docker containers, you may encounter write permission issues in plugin directories if containers create files (e.g. Moodle uploads) under a different UID than your host user.

### Fix

```bash
# Check plugin file ownership
ls -la ~/workspace/www/html/moodle/local/

# Adjust ownership if needed
sudo chown -R $USER:$USER ~/workspace/www/html/moodle/local/
```

In Scenario B, make sure the mounted volume allows write access for whichever user the container runs as — check the UID in your base image (`alpine` runs commands as `root` by default unless you add a `USER` directive) and `chown` the host-side directory to match if needed.

---

## 📋 Useful Commands (Scenario B)

```bash
# Check if the server is running
curl http://localhost:3000/health
```

```bash
# Follow logs in real time
docker logs -f build82
```

```bash
# Restart after updating the image/binary
docker compose restart build82
```

---

## ➡️ Next Steps

- [Claude Code](../clients/claude-code.md) — detailed configuration
- [Antigravity CLI](../clients/antigravity.md) — detailed configuration
- [OpenAI Codex](../clients/codex.md) — detailed configuration
- [OpenCode](../clients/opencode.md) — detailed configuration
- [Common Issues](../../troubleshooting/common-issues.md) — permission and PATH errors
- [Back to Index](../../index.md)
