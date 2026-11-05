🌐 [English](../../../en/guides/environments/docker.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Uso com Docker

O `build82` é um único binário estático, o que o torna flexível em ambientes containerizados. Se sua instalação Moodle roda em Docker (Nginx + PHP-FPM + MariaDB, ou qualquer stack semelhante), rode o `build82` no host, ao lado do seu cliente de IA, e deixe-o ler os arquivos do Moodle pelo volume montado. O build82 conversa com o cliente via stdio, então roda onde o cliente roda; não há modo de rede para colocá-lo num container próprio (o antigo modo `--http` foi removido na v1.2.0).

---

## 🚀 build82 no Host

Neste cenário, o `build82` roda diretamente na sua máquina física, enquanto o Moodle roda nos containers, com o Claude Code, Cursor, VS Code ou qualquer outro cliente de IA local.

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

## 🛠️ Resolvendo Conflitos de Permissão

Ao rodar o `build82` no host com o Moodle em containers Docker, podem ocorrer problemas de permissão de escrita nos diretórios de plugin se os containers criarem arquivos (ex: uploads do Moodle) com um UID diferente do seu usuário do host.

### Solução

```bash
# Verificar o proprietário dos arquivos de plugin
ls -la ~/workspace/www/html/moodle/local/

# Se necessário, ajustar a propriedade
sudo chown -R $USER:$USER ~/workspace/www/html/moodle/local/
```

---

## ➡️ Próximos Passos

- [Claude Code](../clients/claude-code.md) — configuração detalhada
- [Antigravity CLI](../clients/antigravity.md) — configuração detalhada
- [OpenAI Codex](../clients/codex.md) — configuração detalhada
- [OpenCode](../clients/opencode.md) — configuração detalhada
- [Problemas Comuns](../../troubleshooting/common-issues.md) — erros de permissão e PATH
- [Voltar ao Índice](../../index.md)
