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

1. Verifica se Cline (VS Code extension / CLI) foi detectado (o diretório globalStorage do Cline no VS Code estável existe, ou o diretório da CLI do Cline existe: `~/.cline`, `$CLINE_DIR` quando essa variável tem um caminho absoluto, ou `$CLINE_DATA_DIR` quando essa variável está definida); caso contrário imprime `Skipped: Cline (VS Code extension / CLI) not detected.` e não altera nada.
2. Pergunta a raiz do Moodle (sugere o diretório atual se parecer uma raiz do Moodle).
3. Mescla uma entrada `build82` em cada arquivo de configurações do Cline cujo local pai existe (veja a tabela abaixo), no objeto `mcpServers` do nível raiz, preservando as demais entradas, a ordem das chaves e cada valor como foi escrito (só a indentação é normalizada para 2 espaços), as permissões do arquivo e um link simbólico nesse caminho (o arquivo apontado é que é gravado). Uma entrada `build82` nova vai para o fim do objeto. Se o arquivo não existir, ele é criado.
4. Registra o caminho absoluto do binário `build82` em execução como comando (stdio) e define uma única variável de ambiente, `BUILD82_MOODLE_PATH`.

Se o arquivo tiver comentários ou vírgulas finais (JSONC), o build82 o lê, mas nunca o reescreve: se ele já tiver a mesma entrada `build82`, a ferramenta conta como instalada; caso contrário, imprime `Cline (VS Code extension / CLI)... manual step needed: ...` com o trecho `build82` exato para colar manualmente no objeto `mcpServers`, o arquivo permanece inalterado e o `install` sai com código 1. Um BOM UTF-8 é tolerado. Um arquivo que não seja JSON válido, ou cuja chave `mcpServers` não seja um objeto, é reportado como `failed` e fica inalterado.

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
| Cline CLI com `CLINE_DIR` definida (caminho absoluto) | `$CLINE_DIR/data/settings/cline_mcp_settings.json` |
| Cline CLI com `CLINE_DATA_DIR` definida | `$CLINE_DATA_DIR/settings/cline_mcp_settings.json` (tem precedência sobre `CLINE_DIR`) |

Apenas o VS Code estável é detectado (não Insiders, VSCodium nem instalações portáteis).

`CLINE_DIR` e `CLINE_DATA_DIR` são variáveis da própria CLI do Cline: `CLINE_DIR` substitui o diretório de configuração `~/.cline`, e `CLINE_DATA_DIR` substitui o diretório de dados `~/.cline/data` ([configuração da Cline CLI](https://docs.cline.bot/cline-cli/configuration)). O build82 usa um `CLINE_DIR` absoluto no lugar de `~/.cline` (um valor relativo é ignorado) e um `CLINE_DATA_DIR` não vazio no lugar do diretório de dados, para detecção, instalação e desinstalação; por isso, execute o `build82` com os mesmos valores usados pela CLI do Cline. O instalador imprime `Cline (VS Code extension / CLI)... configured.`, ou `... updated.` quando já existia uma entrada `build82` em um dos arquivos.

### Configuração manual

O Cline documenta apenas um arquivo global (sem escopo de projeto). Para o Cline CLI use `~/.cline/data/settings/cline_mcp_settings.json` (ou `$CLINE_DIR/data/settings/cline_mcp_settings.json`, ou `$CLINE_DATA_DIR/settings/cline_mcp_settings.json`) com o mesmo conteúdo. No VS Code clique no ícone **MCP Servers** na barra superior do Cline → aba **Configure** → **Configure MCP Servers**; isso abre o arquivo. Adicione:

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

Isso apaga apenas a chave `build82` de `cline_mcp_settings.json` nos dois locais (globalStorage do VS Code e `~/.cline/data/settings/`, `$CLINE_DIR/data/settings/` ou `$CLINE_DATA_DIR/settings/` quando definidas) (as outras entradas não são tocadas; arquivo inexistente não gera erro) e imprime `Cline (VS Code extension / CLI)... removed.`, ou `Cline (VS Code extension / CLI)... not registered.` (código de saída 0) quando não havia nada a remover. Um arquivo com comentários ou vírgulas finais (JSONC) nunca é reescrito: o comando imprime `Cline (VS Code extension / CLI)... manual step needed: ...` pedindo que você remova a entrada manualmente, e sai com código 1. Um arquivo que não pode ser lido ou interpretado é reportado como `Cline (VS Code extension / CLI)... failed: ...`, nunca como não registrado. Execute `build82 uninstall` sem alvo para localizar e remover todos os registros do build82 de uma vez (pede confirmação).

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
