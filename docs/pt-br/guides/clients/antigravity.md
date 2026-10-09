🌐 [English](../../../en/guides/clients/antigravity.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com Antigravity (IDE e CLI)

O **Google Antigravity** é distribuído como IDE e como agente de terminal, o **Antigravity CLI** (`agy`, sucessor do Gemini CLI). Ambos leem servidores MCP do mesmo formato de `mcp_config.json`, com um arquivo global e um de workspace.

---

## 🛠️ Configuração do Servidor MCP

### Configuração automática

```bash
build82 install antigravity
```

O que ele faz, exatamente:

1. Verifica se Antigravity (IDE / CLI) foi detectado (o comando `agy` está no `PATH`, ou o diretório `~/.gemini/config` ou `~/.gemini/antigravity` existe); caso contrário imprime `Skipped: Antigravity (IDE / CLI) not detected.` e não altera nada.
2. Pergunta a raiz do Moodle (sugere o diretório atual se parecer uma raiz do Moodle).
3. Mescla uma entrada `build82` em `~/.gemini/config/mcp_config.json` (objeto `mcpServers` no nível raiz), preservando as demais entradas, a ordem das chaves e cada valor como foi escrito (só a indentação é normalizada para 2 espaços), as permissões do arquivo e um link simbólico nesse caminho (o arquivo apontado é que é gravado). Uma entrada `build82` nova vai para o fim do objeto. Se o arquivo não existir, ele é criado.
4. Registra o caminho absoluto do binário `build82` em execução como comando (stdio) e define uma única variável de ambiente, `BUILD82_MOODLE_PATH`.

Se o arquivo tiver comentários ou vírgulas finais (JSONC), o build82 o lê, mas nunca o reescreve: se ele já tiver a mesma entrada `build82`, a ferramenta conta como instalada; caso contrário, imprime `Antigravity (IDE / CLI)... manual step needed: ...` com o trecho `build82` exato para colar manualmente no objeto `mcpServers`, o arquivo permanece inalterado e o `install` sai com código 1. Um BOM UTF-8 é tolerado. Um arquivo que não seja JSON válido, ou cuja chave `mcpServers` não seja um objeto, é reportado como `failed` e fica inalterado.

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

Esse é o arquivo global que a IDE e o CLI leem (veja a configuração manual abaixo), então quem usa só a IDE também é detectado. O arquivo de workspace `.agents/mcp_config.json` nunca é escrito pelo instalador.

### Configuração manual

A IDE e o CLI usam o mesmo formato de arquivo e os mesmos locais:

| Escopo | Arquivo |
|--------|---------|
| Global (todos os workspaces) | `~/.gemini/config/mcp_config.json` (Windows: `%USERPROFILE%\.gemini\config\mcp_config.json`) |
| Workspace | `.agents/mcp_config.json` na raiz do workspace |

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

Use caminho **absoluto** em `command`: caminhos relativos não são resolvidos e o cliente pode não herdar o `PATH` do seu shell (`which build82`).

Também é possível adicionar servidores sem editar arquivos: no **CLI** digite `/mcp` para abrir o gerenciador interativo de MCP; na **IDE** abra o menu do painel lateral do agente → **MCP Servers** → **Manage MCP Servers** (arquivo de configuração bruto).

> **Problema conhecido (CLI, escopo de workspace):** a issue [google-antigravity/antigravity-cli#60](https://github.com/google-antigravity/antigravity-cli/issues/60) relata que um arquivo de configuração MCP local do projeto é descoberto na inicialização, mas seus `mcpServers` não são iniciados; só a configuração no diretório home realmente sobe os servidores. No momento da escrita (verificado em 2026-09-29) a issue está **aberta**, atribuída e sem versão de correção. Até ser corrigida, prefira o arquivo **global** no CLI. A IDE não consta como afetada.

### Verifique

- **CLI:** reinicie a sessão (`Ctrl+C`, depois `agy`) e digite `/mcp`; o `build82` deve estar listado.
- **IDE:** reabra o painel MCP Servers; o `build82` deve estar listado com suas tools.

Depois peça ao agente para executar a tool `doctor` do build82.

### Remoção

```bash
build82 uninstall antigravity
```

Isso apaga apenas a chave `build82` de `~/.gemini/config/mcp_config.json` e do `~/.gemini/antigravity/mcp_config.json` legado. As outras entradas não são tocadas e um arquivo inexistente não gera erro. Imprime `Antigravity (IDE / CLI)... removed.`, ou `Antigravity (IDE / CLI)... not registered.` (código de saída 0) quando não havia nada a remover. Um arquivo com comentários ou vírgulas finais (JSONC) nunca é reescrito: o comando imprime `Antigravity (IDE / CLI)... manual step needed: ...` pedindo que você remova a entrada manualmente, e sai com código 1. Um arquivo que não pode ser lido ou interpretado é reportado como `Antigravity (IDE / CLI)... failed: ...`, nunca como não registrado. Execute `build82 uninstall` sem alvo para localizar e remover todos os registros do build82 de uma vez (pede confirmação).

Remover a entrada do arquivo pode não bastar: a IDE e o CLI mantêm cópias em cache em `~/.gemini/antigravity-ide/mcp/` e `~/.gemini/antigravity-cli/mcp/` e podem continuar exibindo um servidor removido. Se o build82 ainda aparecer, apague a pasta de cache correspondente (se houver) e reinicie. Uma entrada adicionada à mão no arquivo de workspace `.agents/mcp_config.json` não é tocada pelo uninstall; remova-a você mesmo.

Referências oficiais: [documentação de MCP do Antigravity](https://antigravity.google/docs/mcp/), [Where does Antigravity look for MCP servers?](https://atamel.dev/posts/2026/07-10_where_agy_mcp_servers/).

---

## 💡 Fluxos de Trabalho Recomendados

### Iniciando uma sessão de desenvolvimento

No início de cada sessão, carregue o contexto do plugin:

```
Estou trabalhando no plugin local_myplugin. Carregue o contexto completo.
```

O Antigravity CLI chamará `get_plugin_info` e passará a conhecer a arquitetura, banco de dados, funções e padrões do plugin.

### Consultando a API do core

```
Quais funções da API do core devo usar para verificar se um usuário
está matriculado em um curso? Prefira funções públicas e não depreciadas.
```

O Antigravity CLI usará `search_api` e retornará as funções com assinaturas e arquivos fonte.

### Criação de novos plugins com slash command

Use o slash command diretamente:

```
/scaffold_plugin type="local" name="web_service_test" description="Plugin de teste de web services" features="web services, capabilities"
```

Após criar os arquivos, gere o contexto:

```
Gere o contexto de IA para o plugin local_web_service_test.
```

### Revisão antes de um commit

```
/review_plugin plugin="local/myplugin" focus="security"
```

---

## ⚠️ Solução de Problemas

### Primeiro passo: verifique a conexão

Digite `/mcp` no CLI (ou abra o painel MCP Servers na IDE). Se o `build82` não aparecer, o problema é a configuração, não o seu prompt.

### O servidor não aparece após configurar

- Confirme que você editou o arquivo que o cliente realmente lê: `~/.gemini/config/mcp_config.json` (global) ou `.agents/mcp_config.json` (workspace). O `build82 install` escreve o global.
- Valide o JSON e garanta que `command` seja um caminho absoluto.
- Arquivo de workspace ignorado no CLI? Veja o problema conhecido acima e use o arquivo global.
- Reinicie a sessão do `agy` ou a IDE após corrigir.

### Caminhos relativos não funcionam

O `mcp_config.json` exige **caminhos absolutos**.

### BUILD82_MOODLE_PATH incorreto

`BUILD82_MOODLE_PATH` deve apontar para o diretório que contém o `version.php`. A tool `doctor` reporta erro se a instalação não puder ser validada.

### Contexto desatualizado após mudanças

- **Novo plugin instalado:** _"Regenere todos os índices globais do Moodle."_ → `update_indexes`
- **Mudanças em um plugin:** _"Regenere o contexto do local_myplugin."_ → `generate_plugin_context`

---

## ➡️ Próximos Passos

- [Claude Desktop](./claude-desktop.md) — app desktop da Anthropic (macOS, Windows e Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — CLI da Anthropic, `build82 install claude`
- [OpenAI Codex](./codex.md) — CLI da OpenAI com configuração TOML
- [OpenCode](./opencode.md) — agente open source com interface TUI
- [Cursor](./cursor.md) — editor Cursor
- [Zed](./zed.md) — editor Zed
- [Cline](./cline.md) — extensão Cline para VS Code e Cline CLI
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
