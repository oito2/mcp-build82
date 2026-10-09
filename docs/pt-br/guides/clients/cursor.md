🌐 [English](../../../en/guides/clients/cursor.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com Cursor

O **Cursor** lê servidores MCP de um `mcp.json`, global ou por projeto, no formato `mcpServers`.

---

## 🛠️ Configuração do Servidor MCP

### Configuração automática

```bash
build82 install cursor
```

O que ele faz, exatamente:

1. Verifica se Cursor foi detectado (o diretório `~/.cursor` existe); caso contrário imprime `Skipped: Cursor not detected.` e não altera nada.
2. Pergunta a raiz do Moodle (sugere o diretório atual se parecer uma raiz do Moodle).
3. Mescla uma entrada `build82` em `~/.cursor/mcp.json` (objeto `mcpServers` no nível raiz), preservando as demais entradas, a ordem das chaves e cada valor como foi escrito (só a indentação é normalizada para 2 espaços), as permissões do arquivo e um link simbólico nesse caminho (o arquivo apontado é que é gravado). Uma entrada `build82` nova vai para o fim do objeto. Se o arquivo não existir, ele é criado.
4. Registra o caminho absoluto do binário `build82` em execução como comando (stdio) e define uma única variável de ambiente, `BUILD82_MOODLE_PATH`.

Se o arquivo tiver comentários ou vírgulas finais (JSONC), o build82 o lê, mas nunca o reescreve: se ele já tiver a mesma entrada `build82`, a ferramenta conta como instalada; caso contrário, imprime `Cursor... manual step needed: ...` com o trecho `build82` exato para colar manualmente no objeto `mcpServers`, o arquivo permanece inalterado e o `install` sai com código 1. Um BOM UTF-8 é tolerado. Um arquivo que não seja JSON válido, ou cuja chave `mcpServers` não seja um objeto, é reportado como `failed` e fica inalterado.

Entrada resultante:

```json
{
  "mcpServers": {
    "build82": {
      "type": "stdio",
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle" }
    }
  }
}
```

O instalador escreve apenas o arquivo **global**; arquivos de projeto (`.cursor/mcp.json`) ficam por sua conta.

### Configuração manual

| Escopo | Arquivo |
|--------|---------|
| Global (todos os projetos) | `~/.cursor/mcp.json` |
| Projeto | `.cursor/mcp.json` na raiz do projeto |

Ambos usam o mesmo conteúdo:

```json
{
  "mcpServers": {
    "build82": {
      "type": "stdio",
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle" }
    }
  }
}
```

O Cursor também suporta interpolação `${env:NOME}`, `${workspaceFolder}` e `${userHome}` dentro do `mcp.json`, por exemplo `"BUILD82_MOODLE_PATH": "${workspaceFolder}"` em um arquivo de projeto mantido na raiz do Moodle.

### Verifique

Abra o painel **Customize** na barra lateral do Cursor e confirme que o `build82` aparece e está habilitado (alterne se necessário). Depois peça ao agente:

```
Execute o doctor do build82.
```

### Remoção

```bash
build82 uninstall cursor
```

Isso apaga apenas a chave `build82` de `~/.cursor/mcp.json`. As outras entradas não são tocadas e um arquivo inexistente não gera erro. Imprime `Cursor... removed.`, ou `Cursor... not registered.` (código de saída 0) quando não havia nada a remover. Um arquivo com comentários ou vírgulas finais (JSONC) nunca é reescrito: o comando imprime `Cursor... manual step needed: ...` pedindo que você remova a entrada manualmente, e sai com código 1. Um arquivo que não pode ser lido ou interpretado é reportado como `Cursor... failed: ...`, nunca como não registrado. Execute `build82 uninstall` sem alvo para localizar e remover todos os registros do build82 de uma vez (pede confirmação).

Remova à mão uma entrada de `.cursor/mcp.json` no nível do projeto.

---

## ⚠️ Troubleshooting

- **Servidor não aparece:** recarregue a janela e verifique a sintaxe do JSON.
- **`build82: command not found`:** use o caminho absoluto (`which build82`).
- **Caminho do Moodle errado:** `BUILD82_MOODLE_PATH` deve ser o diretório que contém o `version.php`.

Referência oficial: [documentação de MCP do Cursor](https://cursor.com/docs/context/mcp).

---

## ➡️ Próximos Passos

- [Claude Desktop](./claude-desktop.md) — app desktop da Anthropic (macOS, Windows e Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — CLI da Anthropic, `build82 install claude`
- [OpenAI Codex](./codex.md) — CLI da OpenAI com configuração TOML
- [Antigravity (IDE and CLI)](./antigravity.md) — IDE e agente de terminal do Google
- [OpenCode](./opencode.md) — agente open source com interface TUI
- [Zed](./zed.md) — editor Zed
- [Cline](./cline.md) — extensão Cline para VS Code e Cline CLI
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
