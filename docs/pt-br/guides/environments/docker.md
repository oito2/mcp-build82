🌐 [English](../../../en/guides/environments/docker.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Uso com Docker

O `build82` é um único binário estático, o que o torna flexível em ambientes containerizados. Se sua instalação Moodle roda em Docker (Nginx + PHP-FPM + MariaDB, ou qualquer stack semelhante), existem dois cenários principais de operação.

---

## 🚀 Cenário A: build82 no Host (Recomendado)

Neste cenário, o `build82` roda diretamente na sua máquina física, enquanto o Moodle roda nos containers. Esta é a opção recomendada para a maioria dos desenvolvedores usando Claude Code, Cursor, VS Code ou qualquer outro cliente de IA local.

### Por que usar assim?

- **Performance:** Leitura de arquivos sem o overhead do sistema de arquivos do Docker (especialmente no macOS e Windows).
- **Simplicidade:** O assistente de IA no host enxerga o binário `build82` diretamente — sem configuração de rede, sem problemas de PATH dentro de um container.
- **Persistência:** Os arquivos `.md` de contexto são escritos no volume montado do host (sob `.build82/`) e ficam visíveis imediatamente dentro do container Moodle.

### Como funciona

Se o seu diretório Moodle fica em, digamos:

```
~/workspace/www/html/<projeto>/
```

e é montado (bind mount) nos seus containers PHP, o `build82` rodando no host lê esses arquivos diretamente, como se fossem um diretório local qualquer — o Docker não precisa entrar na jogada.

#### Configuração do cliente de IA (Claude Code)

```bash
build82 install claude
```

ou manualmente:

```bash
claude mcp add build82 \
  -e BUILD82_MOODLE_PATH=/home/usuario/workspace/www/html/moodle \
  -- build82
```

#### Configuração do cliente de IA (Antigravity CLI — `~/.gemini/config/mcp_config.json`)

> O `build82 install antigravity` escreve um arquivo diferente do mostrado aqui; veja o [guia do Antigravity](../clients/antigravity.md) antes de confiar nele. Os outros clientes (Codex, Cursor, Zed, Cline) estão nos seus próprios [guias](../clients/codex.md).

```json
{
    "mcpServers": {
        "build82": {
            "command": "build82",
            "args": [],
            "env": {
                "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle"
            }
        }
    }
}
```

#### Configuração do cliente de IA (OpenCode — `opencode.json` na raiz do Moodle)

```json
{
    "$schema": "https://opencode.ai/config.json",
    "mcp": {
        "build82": {
            "type": "local",
            "command": ["build82"],
            "environment": {
                "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle"
            }
        }
    }
}
```

#### Inicializando o contexto

Com o cliente configurado, peça ao assistente de IA:

```
Inicialize o contexto do build82 para minha instalação Moodle.
```

O assistente chamará `init_moodle_context`, detectará a versão do Moodle e gerará todos os índices globais — sem qualquer comando de terminal adicional.

---

## 🐳 Cenário B: build82 como Sidecar (Docker Compose)

Use este cenário quando precisar que o servidor faça parte da infraestrutura e seja acessível via rede — por exemplo, para equipes que compartilham uma instância Moodle remota, usando o transporte `--http`.

### Uma imagem mínima

Como o `build82` é um único binário estático, um container para ele só precisa buscar e rodar esse binário — nenhum runtime para instalar:

```dockerfile
FROM alpine:3.19
RUN apk add --no-cache curl ca-certificates && \
    curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_amd64 && \
    chmod +x build82_linux_amd64 && \
    mv build82_linux_amd64 /usr/local/bin/build82
ENTRYPOINT ["build82"]
```

Em hosts ARM64 (ex.: Apple Silicon rodando Docker), baixe `build82_linux_arm64`. Para verificar o download, baixe também o `checksums.txt` (veja [Instalação](../../getting-started/installation.md)).

### Configuração do docker-compose.yml

```yaml
services:
    build82:
        build: ./build82-image  # o Dockerfile acima
        container_name: build82
        volumes:
            - ./www/html/moodle:/var/www/moodle
        environment:
            - BUILD82_MOODLE_PATH=/var/www/moodle
            - BUILD82_MOODLE_VERSION=4.5
        command: ["--http", "--port", "3000", "--host", "0.0.0.0", "--token", "meu-segredo-aqui"]
        ports:
            - "3000:3000"
```

> **Por que não usar `:ro`?** O servidor precisa de permissão de **escrita** para criar os arquivos `.md` de contexto sob `.build82/` dentro dos diretórios de plugin (ex: `PLUGIN_AI_CONTEXT.md`, `PLUGIN_DB_TABLES.md`). Monte o volume sem `:ro` para que a geração de contexto funcione corretamente.

### Configurando o cliente de IA para o modo HTTP

Após subir o container, configure seu cliente de IA para se conectar via URL:

O servidor expõe Streamable HTTP em `/mcp` e SSE legado em `/sse`, ambos protegidos pelo token Bearer; `GET /health` não exige token.

#### Claude Code

```bash
claude mcp add --transport http build82 http://localhost:3000/mcp \
  --header "Authorization: Bearer meu-segredo-aqui"
```

Para os outros clientes, use a sintaxe própria de cada um para servidores remotos (HTTP), com a mesma URL e o cabeçalho `Authorization: Bearer <token>`; os nomes dos campos variam por cliente, então consulte a documentação oficial dele e o seu [guia](../clients/claude-code.md).

> **Verificação do cabeçalho Host:** o servidor rejeita requisições cujo cabeçalho `Host` não seja `localhost`, `127.0.0.1`, `::1`, o valor de `--host` ou um valor passado com `--allowed-host`. Para acessá-lo de outra máquina ou container por IP ou hostname, adicione `--allowed-host <esse-nome>` (repetível) ao `command`, ex.: `"--allowed-host", "build82"` para outros containers na mesma rede do compose. A requisição rejeitada recebe um 403 citando o host.
> Não há TLS: em produção, use um reverse proxy (nginx, Caddy) com TLS. Em vez de `--token`, você pode definir a variável de ambiente `BUILD82_TOKEN` para manter o segredo fora da lista de processos.

---

## 🛠️ Resolvendo Conflitos de Permissão

Ao rodar o `build82` no host (Cenário A) com o Moodle em containers Docker, podem ocorrer problemas de permissão de escrita nos diretórios de plugin se os containers criarem arquivos (ex: uploads do Moodle) com um UID diferente do seu usuário do host.

### Solução

```bash
# Verificar o proprietário dos arquivos de plugin
ls -la ~/workspace/www/html/moodle/local/

# Se necessário, ajustar a propriedade
sudo chown -R $USER:$USER ~/workspace/www/html/moodle/local/
```

No Cenário B, certifique-se de que o volume montado tem permissão de escrita para o usuário com que o container roda — verifique o UID na sua imagem base (a `alpine` roda comandos como `root` por padrão, a menos que você adicione uma diretiva `USER`) e ajuste a propriedade do diretório do lado do host se necessário.

---

## 📋 Comandos Úteis (Cenário B)

```bash
# Verificar se o servidor está ativo
curl http://localhost:3000/health

# Acompanhar logs em tempo real
docker logs -f build82

# Reiniciar após atualizar a imagem/binário
docker compose restart build82
```

---

## ➡️ Próximos Passos

- [Claude Code](../clients/claude-code.md) — configuração detalhada
- [Antigravity CLI](../clients/antigravity.md) — configuração detalhada
- [OpenAI Codex](../clients/codex.md) — configuração detalhada
- [OpenCode](../clients/opencode.md) — configuração detalhada
- [Problemas Comuns](../../troubleshooting/common-issues.md) — erros de permissão e PATH
- [Voltar ao Índice](../../index.md)
