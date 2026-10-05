🌐 [English](../../../en/guides/clients/zed.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com Zed

O **Zed** registra servidores MCP como **context servers** no `settings.json`, na chave `context_servers` (e não `mcpServers`).

---

## 🛠️ Configuração do Servidor MCP

### Configuração automática

```bash
build82 install zed
```

O que ele faz, exatamente:

1. Verifica se Zed foi detectado (o diretório `~/.config/zed` existe; no Windows, `%APPDATA%\Zed`); caso contrário imprime `Skipped: Zed not detected.` e não altera nada.
2. Pergunta a raiz do Moodle (sugere o diretório atual se parecer uma raiz do Moodle).
3. Mescla uma entrada `build82` em `~/.config/zed/settings.json` (Windows: `%APPDATA%\Zed\settings.json`) (objeto `context_servers` no nível raiz), preservando as demais entradas e as permissões do arquivo. Se o arquivo não existir, ele é criado.
4. Registra o caminho absoluto do binário `build82` em execução como comando (stdio) e define uma única variável de ambiente, `BUILD82_MOODLE_PATH`.

Se o arquivo tiver comentários ou vírgulas finais (JSONC), o build82 o lê, mas nunca o reescreve: o comando falha com `Zed... failed: ...`, o arquivo permanece inalterado e a mensagem traz o trecho `build82` exato para colar manualmente no objeto `context_servers`. Um arquivo que não seja JSON válido também aborta sem escrever.

Entrada resultante:

```json
{
  "context_servers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle" }
    }
  }
}
```

O `settings.json` do Zed costuma conter comentários e vírgulas finais. Nesse caso o instalador o deixa intacto e imprime o trecho para você colar no objeto `context_servers`. O arquivo de projeto `.zed/settings.json` nunca é escrito.

### Configuração manual

Abra o arquivo com o comando `zed: open settings file` (global) ou use `.zed/settings.json` na raiz do projeto (escopo de projeto):

```json
{
  "context_servers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle" }
    }
  }
}
```

Como alternativa, use **Settings → AI → MCP Servers → Add Server**.

### Verifique

Em **Settings → AI → MCP Servers** (ou nas configurações do Agent Panel), a entrada `build82` deve mostrar um **ponto verde** ("Server is active"). Passe o mouse sobre outros estados para ver o erro.

### Remoção

```bash
build82 uninstall zed
```

Isso apaga apenas a chave `build82` de `~/.config/zed/settings.json` (Windows: `%APPDATA%\Zed\settings.json`). As outras entradas não são tocadas e um arquivo inexistente não gera erro. Imprime `Zed... removed.`, ou `Zed... not registered.` (código de saída 0) quando não havia nada a remover. Um arquivo com comentários ou vírgulas finais (JSONC) nunca é reescrito: o comando falha com `Zed... failed: ...` pedindo que você remova a entrada manualmente. Execute `build82 uninstall` sem alvo para localizar e remover todos os registros do build82 de uma vez (pede confirmação).

Remova à mão entradas de um `.zed/settings.json` de projeto.

---

## ⚠️ Troubleshooting

- **Indicador não está verde:** passe o mouse para ver o motivo; confira se `command` é um caminho absoluto.
- **Caminho do Moodle errado:** `BUILD82_MOODLE_PATH` deve ser o diretório que contém o `version.php`.

Referência oficial: [Model Context Protocol in Zed](https://zed.dev/docs/ai/mcp).

---

## ➡️ Próximos Passos

- [Claude Desktop](./claude-desktop.md) — app desktop da Anthropic (macOS, Windows e Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — CLI da Anthropic, `build82 install claude`
- [OpenAI Codex](./codex.md) — CLI da OpenAI com configuração TOML
- [Antigravity (IDE and CLI)](./antigravity.md) — IDE e agente de terminal do Google
- [OpenCode](./opencode.md) — agente open source com interface TUI
- [Cursor](./cursor.md) — editor Cursor
- [Cline](./cline.md) — extensão Cline para VS Code e Cline CLI
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
