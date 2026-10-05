🌐 [English](../../../en/guides/clients/cline.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com Cline

O **Cline** é uma extensão do VS Code (`saoudrizwan.claude-dev`). Seus servidores MCP ficam em um único arquivo `cline_mcp_settings.json`, no formato `mcpServers`.

---

## 🛠️ Configuração do Servidor MCP

### Configuração automática

```bash
build82 install cline
```

O que ele faz, exatamente:

1. Verifica se Cline (VS Code extension / CLI) foi detectado (o diretório globalStorage do Cline no VS Code estável existe, ou o diretório da CLI do Cline existe: `~/.cline`, ou `$CLINE_DATA_DIR` quando essa variável está definida); caso contrário imprime `Skipped: Cline (VS Code extension / CLI) not detected.` e não altera nada.
2. Pergunta a raiz do Moodle (sugere o diretório atual se parecer uma raiz do Moodle).
3. Mescla uma entrada `build82` em cada arquivo de configurações do Cline cujo local pai existe (veja a tabela abaixo), no objeto `mcpServers` do nível raiz, preservando as demais entradas e as permissões do arquivo. Se o arquivo não existir, ele é criado.
4. Registra o caminho absoluto do binário `build82` em execução como comando (stdio) e define uma única variável de ambiente, `BUILD82_MOODLE_PATH`.

Se o arquivo tiver comentários ou vírgulas finais (JSONC), o build82 o lê, mas nunca o reescreve: o comando falha com `Cline (VS Code extension / CLI)... failed: ...`, o arquivo permanece inalterado e a mensagem traz o trecho `build82` exato para colar manualmente no objeto `mcpServers`. Um arquivo que não seja JSON válido também aborta sem escrever.

Entrada resultante:

```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle" }
    }
  }
}
```

O instalador escreve em cada local que existir, então uma máquina com a extensão do VS Code e o Cline CLI recebe os dois arquivos:

| Local | Caminho |
|-------|---------|
| Extensão do VS Code, Linux | `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| Extensão do VS Code, macOS | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| Extensão do VS Code, Windows | `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json` |
| Cline CLI (todos os sistemas) | `~/.cline/data/settings/cline_mcp_settings.json` |
| Cline CLI com `CLINE_DATA_DIR` definida | `$CLINE_DATA_DIR/settings/cline_mcp_settings.json` |

Apenas o VS Code estável é detectado (não Insiders, VSCodium nem instalações portáteis).

`CLINE_DATA_DIR` é a variável da própria CLI do Cline para um diretório de dados personalizado, e substitui `~/.cline/data` ([configuração da Cline CLI](https://docs.cline.bot/cline-cli/configuration)). Quando definida com um valor não vazio, o build82 a usa no lugar de `~/.cline/data` para detecção, instalação e desinstalação; por isso, execute o `build82` com o mesmo valor usado pela CLI do Cline. O instalador imprime `Cline (VS Code extension / CLI)... configured.`, ou `... updated.` quando já existia uma entrada `build82` em um dos arquivos.

### Configuração manual

O Cline documenta apenas um arquivo global (sem escopo de projeto). Para o Cline CLI use `~/.cline/data/settings/cline_mcp_settings.json` (ou `$CLINE_DATA_DIR/settings/cline_mcp_settings.json`) com o mesmo conteúdo. No VS Code clique no ícone **MCP Servers** na barra superior do Cline → aba **Configure** → **Configure MCP Servers**; isso abre o arquivo. Adicione:

```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": {
        "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle"
      }
    }
  }
}
```

### Verifique

Abra o painel **MCP Servers** do Cline e confirme que o `build82` aparece conectado (indicador verde) e lista suas tools. Depois peça:

```
Execute o doctor do build82.
```

### Remoção

```bash
build82 uninstall cline
```

Isso apaga apenas a chave `build82` de `cline_mcp_settings.json` nos dois locais (globalStorage do VS Code e `~/.cline/data/settings/`, ou `$CLINE_DATA_DIR/settings/` quando definida) (as outras entradas não são tocadas; arquivo inexistente não gera erro) e imprime `Cline (VS Code extension / CLI)... removed.`, ou `Cline (VS Code extension / CLI)... not registered.` (código de saída 0) quando não havia nada a remover. Um arquivo com comentários ou vírgulas finais (JSONC) nunca é reescrito: o comando falha com `Cline (VS Code extension / CLI)... failed: ...` pedindo que você remova a entrada manualmente. Execute `build82 uninstall` sem alvo para localizar e remover todos os registros do build82 de uma vez (pede confirmação).

---

## ⚠️ Troubleshooting

- **Servidor não aparece:** salve o arquivo e reabra o painel MCP Servers; verifique a sintaxe do JSON.
- **`build82: command not found`:** use o caminho absoluto (`which build82`).
- **Caminho do Moodle errado:** `BUILD82_MOODLE_PATH` deve ser o diretório que contém o `version.php`.

Referência oficial: [Configuring MCP servers (Cline)](https://docs.cline.bot/mcp/configuring-mcp-servers).

---

## ➡️ Próximos Passos

- [Claude Desktop](./claude-desktop.md) — app desktop da Anthropic (macOS, Windows e Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — CLI da Anthropic, `build82 install claude`
- [OpenAI Codex](./codex.md) — CLI da OpenAI com configuração TOML
- [Antigravity (IDE and CLI)](./antigravity.md) — IDE e agente de terminal do Google
- [OpenCode](./opencode.md) — agente open source com interface TUI
- [Cursor](./cursor.md) — editor Cursor
- [Zed](./zed.md) — editor Zed
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
