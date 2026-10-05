🌐 [English](../../../en/guides/clients/claude-desktop.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com Claude Desktop

O **Claude Desktop** é o aplicativo desktop da Anthropic para macOS, Windows e Linux. A versão para Linux está em beta e suporta Ubuntu 22.04+ e Debian 12+ (x86-64 e arm64); veja [Claude Desktop on Linux](https://code.claude.com/docs/en/desktop-linux). Ele inicia servidores MCP locais via **stdio** a partir de um arquivo JSON, o `claude_desktop_config.json`.

---

## 🛠️ Configuração do Servidor MCP

### Extensão desktop (`.mcpb`)

Todo release traz o `build82.mcpb`, um bundle de extensão desktop com os binários do `build82` para macOS (universal: Intel e Apple Silicon), Windows (x86-64) e Linux (x86-64 e arm64, escolhido na inicialização por um pequeno script). Ele não precisa de um binário `build82` instalado à parte.

1. Baixe o `build82.mcpb` da [release mais recente](https://github.com/oito2/mcp-build82/releases/latest) e verifique-o contra o `checksums.txt` (veja [Instalação](../../getting-started/installation.md)).
2. No Claude Desktop, abra **Settings > Extensions**, clique em **Advanced settings** e, na seção **Extension Developer**, clique em **Install Extension…**. Selecione o `build82.mcpb`.
3. Quando solicitado, escolha o **diretório raiz do Moodle** (obrigatório). Ele é repassado ao servidor como `BUILD82_MOODLE_PATH`.

Use a extensão ou a configuração abaixo, não as duas: cada uma registra o seu próprio servidor `build82`. Para atualizar a extensão, instale o `build82.mcpb` da release mais nova. O bundle não é assinado.

### Configuração automática

```bash
build82 install claude-desktop
```

O que ele faz, exatamente:

1. Verifica se o Claude Desktop foi detectado (o diretório de configuração dele existe: `~/Library/Application Support/Claude` no macOS, `%APPDATA%\Claude` no Windows, `$XDG_CONFIG_HOME/Claude` no Linux, com padrão `~/.config/Claude`); caso contrário imprime `Skipped: Claude Desktop not detected.` e não altera nada. O Claude Desktop cria esse diretório na primeira vez que é aberto, então abra-o uma vez antes de rodar o comando.
2. Pergunta a raiz do Moodle (sugere o diretório atual se parecer uma raiz do Moodle).
3. Mescla uma entrada `build82` no `claude_desktop_config.json` desse diretório (objeto `mcpServers` no nível raiz), preservando as demais entradas e as permissões do arquivo. Se o arquivo não existir, ele é criado.
4. Registra o caminho absoluto do binário `build82` em execução como comando (stdio) e define uma única variável de ambiente, `BUILD82_MOODLE_PATH`.

Se o arquivo tiver comentários ou vírgulas finais (JSONC), o build82 o lê, mas nunca o reescreve: o comando falha com `Claude Desktop... failed: ...`, o arquivo permanece inalterado e a mensagem traz o trecho `build82` exato para colar manualmente no objeto `mcpServers`. Um arquivo que não seja JSON válido também aborta sem escrever.

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

Reinicie o Claude Desktop depois (veja [Reinicie](#reinicie)). O alvo `claude` configura apenas o **Claude Code** (veja [Claude Code](./claude-code.md)).

### Configuração manual (global)

O Claude Desktop tem um único arquivo de configuração por usuário (não existe escopo de projeto). Abra-o em **menu Claude → Settings… → Developer → Edit Config**, ou edite diretamente:

| Sistema operacional | Caminho |
|---------------------|---------|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |
| Linux | `~/.config/Claude/claude_desktop_config.json` (`$XDG_CONFIG_HOME/Claude/` quando `XDG_CONFIG_HOME` está definida) |

Adicione o `build82` em `mcpServers` (mantenha os servidores já existentes):

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

No Windows, escape as barras invertidas nos caminhos (`"C:\\Tools\\build82.exe"`). Use sempre **caminho absoluto** em `command` — o Claude Desktop não herda o `PATH` do seu shell. Execute `which build82` (macOS e Linux) ou `where build82` (Windows) para descobri-lo.

### Reinicie

Encerre o Claude Desktop por completo e abra-o novamente. A configuração só é lida na inicialização.

### Verifique

Após reiniciar, clique no botão **+ / Add files, connectors, and more** no campo de conversa, passe o mouse sobre **Connectors** e confirme que o `build82` aparece com suas tools. Depois peça:

```
Execute o doctor do build82.
```

### Remoção

```bash
build82 uninstall claude-desktop
```

Isso apaga apenas a chave `build82` de `claude_desktop_config.json` (as outras entradas não são tocadas; arquivo inexistente não gera erro) e imprime `Claude Desktop... removed.`, ou `Claude Desktop... not registered.` (código de saída 0) quando não havia nada a remover. Um arquivo com comentários ou vírgulas finais (JSONC) nunca é reescrito: o comando falha com `Claude Desktop... failed: ...` pedindo que você remova a entrada manualmente. Execute `build82 uninstall` sem alvo para localizar e remover todos os registros do build82 de uma vez (pede confirmação).

Reinicie o Claude Desktop depois.

---

## ⚠️ Troubleshooting

- **Servidor não aparece:** valide a sintaxe do JSON e confirme que `command` é um caminho absoluto para um arquivo executável.
- **Logs:** macOS `~/Library/Logs/Claude`, Windows `%APPDATA%\Claude\logs`, Linux `~/.config/Claude/logs`. O `mcp.log` cobre problemas de conexão; o `mcp-server-build82.log` guarda o stderr do build82.
- **Caminho do Moodle errado:** `BUILD82_MOODLE_PATH` deve apontar para o diretório que contém o `version.php`; a tool `doctor` reporta erros de validação.
- **Transporte:** o Claude Desktop inicia o build82 via stdio (padrão). O modo `--http` não é usado aqui.

Referência oficial: [Connect to local MCP servers](https://modelcontextprotocol.io/docs/develop/connect-local-servers).

---

## ➡️ Próximos Passos

- [Claude Code](./claude-code.md) — CLI da Anthropic, `build82 install claude`
- [OpenAI Codex](./codex.md) — CLI da OpenAI com configuração TOML
- [Antigravity (IDE and CLI)](./antigravity.md) — IDE e agente de terminal do Google
- [OpenCode](./opencode.md) — agente open source com interface TUI
- [Cursor](./cursor.md) — editor Cursor
- [Zed](./zed.md) — editor Zed
- [Cline](./cline.md) — extensão Cline para VS Code e Cline CLI
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
