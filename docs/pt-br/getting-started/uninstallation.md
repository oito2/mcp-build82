🌐 [English](../../en/getting-started/uninstallation.md) | **Português** | 🏠 [Índice](../index.md)

---

# Desinstalação

Instalar e usar o build82 deixa arquivos em vários lugares. Esta página lista todos, o que o `build82 uninstall` remove automaticamente e o que você precisa remover manualmente. Ordem recomendada: remover o registro nos clientes MCP, opcionalmente executar o purge e, por fim, apagar o binário.

| Item | Local | Removido por `build82 uninstall` | Removido por `--purge` |
| :--- | :--- | :---: | :---: |
| Registros em clientes MCP | Por cliente, veja [abaixo](#registros-em-clientes-mcp) | Sim | Não |
| Arquivo de configuração | `~/.build82` | Não | Sim |
| Arquivos Markdown globais gerados (13) | `<raiz_moodle>/.build82/*.md` | Não | Sim |
| Arquivos Markdown por plugin (12) e marcador `.indevelopment` | `<raiz_plugin>/.build82/` dos plugins marcados com `.indevelopment` | Não | Sim |
| `tags`, `.cache.json`, diretórios `.build82/` | `<raiz_moodle>/.build82/` | Não | Não |
| Diretórios `.build82/` de plugins sem `.indevelopment` | `<raiz_plugin>/.build82/` | Não | Não |
| Binário | Veja [Removendo o binário](#removendo-o-binário) | Não | Não |
| Backup `.bak` do `self-update` | Ao lado do binário | Não | Não |

---

## `build82 uninstall`

```bash
build82 uninstall [alvo] [--purge]
```

| Argumento | Descrição |
| :--- | :--- |
| `alvo` | Opcional. O primeiro argumento que não começa com `-`. Um entre `claude`, `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`. Omitido: todos os registros encontrados. |
| `--purge` | Opcional, pode aparecer em qualquer posição. Após remover os registros, apaga também os arquivos gerados e o arquivo de configuração, com confirmação própria e separada. |

Argumentos posicionais além do primeiro são ignorados. Um `alvo` desconhecido falha com `unknown target "<id>" — supported targets: <lista>` e código de saída 1.

### Sem alvo

1. O build82 verifica em cada cliente suportado se existe uma entrada `build82`. Não é exigido que o cliente seja detectado como instalado.
   - Claude Code e Codex: executam `<ferramenta> mcp get build82`; a ferramenta conta como registrada quando o comando tem sucesso (no Claude Code isso também casa com os escopos `local` e `project` do diretório atual). Se a ferramenta não existir ou o comando falhar, é tratada como não registrada.
   - Clientes baseados em arquivo: algum dos arquivos de configuração listados [abaixo](#o-que-é-removido-por-cliente) deve existir, ser legível como JSON (comentários e vírgulas finais são tolerados na leitura) e conter a chave `build82` sob a chave de nível superior do cliente.
2. Nenhuma entrada encontrada: imprime `No build82 registrations found.`
3. Caso contrário, lista os clientes e pergunta `Remove build82 from all N tool(s)? [y/N]`. Somente `y` (qualquer caixa) confirma; qualquer outra resposta, inclusive vazia, encerra sem alterações e **sem executar o `--purge`**.
4. Remove cada entrada, imprimindo `<Rótulo>... removed.` ou `<Rótulo>... failed: <erro>` (no Claude Code, `<Rótulo>... nothing removed.` quando só foi encontrado um registro `project`, seguido de um aviso; veja [abaixo](#escopos-do-claude-code)). Uma falha em um cliente não interrompe os demais e não altera o código de saída.

### Com alvo

- Sem prompt de confirmação.
- Claude Code e Codex: se a ferramenta não for encontrada no `PATH`, imprime `Skipped: <Rótulo> not detected.` (código de saída 0). Caso contrário executa `<ferramenta> mcp get build82`; se não houver registro imprime `<Rótulo>... not registered.` (código de saída 0), senão executa o comando de remoção (veja a tabela abaixo). Um registro `project` do Claude Code nunca é removido: imprime um aviso e sai com código 0 (veja [Escopos do Claude Code](#escopos-do-claude-code)). Em caso de falha imprime `<Rótulo>... failed: <erro>` e sai com código 1 (o `--purge` não é executado).
- Clientes baseados em arquivo: sem verificação de detecção. Remove a chave `build82` de todo arquivo listado para o alvo. Imprime `<Rótulo>... removed.` quando ao menos uma entrada foi apagada, ou `<Rótulo>... not registered.` (código de saída 0) quando nenhuma foi encontrada. JSON inválido em um dos arquivos é um erro (`<Rótulo>... failed: <erro>`, código de saída 1).
- Um arquivo com comentários ou vírgulas finais (JSONC, comum no `settings.json` do Zed e no `opencode.jsonc` do OpenCode) nunca é reescrito: se ele contiver uma entrada `build82`, o comando falha pedindo que você remova a entrada manualmente, e o arquivo permanece inalterado.

### O que é removido por cliente

Apenas a chave `build82` é apagada; o restante do arquivo é preservado (as permissões do arquivo são mantidas). Um arquivo nunca é criado se não existir.

| Alvo (rótulo) | Registro removido |
| :--- | :--- |
| `claude` (Claude Code) | `claude mcp remove --scope <escopo> build82` para cada registro `user` e `local`; um registro `project` é mantido inalterado, com um aviso |
| `claude-desktop` (Claude Desktop) | `mcpServers.build82` em `claude_desktop_config.json` (macOS: `~/Library/Application Support/Claude/`, Windows: `%APPDATA%\Claude\`, Linux: `~/.config/Claude/`) |
| `antigravity` (Antigravity (IDE / CLI)) | `mcpServers.build82` em `~/.gemini/config/mcp_config.json` e no legado `~/.gemini/antigravity/mcp_config.json` |
| `codex` (OpenAI Codex CLI) | `codex mcp remove build82` |
| `opencode` (OpenCode) | `mcp.build82` em `~/.config/opencode/opencode.json`, `~/.config/opencode/opencode.jsonc` e no legado `~/.config/opencode/config.json` |
| `cursor` (Cursor) | `mcpServers.build82` em `~/.cursor/mcp.json` |
| `zed` (Zed) | `context_servers.build82` em `~/.config/zed/settings.json` (Windows: `%APPDATA%\Zed\settings.json`) |
| `cline` (Cline (VS Code extension / CLI)) | `mcpServers.build82` em `<globalStorage>/settings/cline_mcp_settings.json` e em `~/.cline/data/settings/cline_mcp_settings.json` (`$CLINE_DATA_DIR/settings/cline_mcp_settings.json` quando `CLINE_DATA_DIR` está definida) |

`<globalStorage>` do Cline (VS Code estável):

| SO | Caminho |
| :--- | :--- |
| Linux | `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev` |
| macOS | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev` |
| Windows | `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev` (`%USERPROFILE%\AppData\Roaming` se `APPDATA` não estiver definida) |

### Escopos do Claude Code

O `claude mcp get build82` informa apenas o registro que tem precedência (`local`, depois `project`, depois `user`), em uma linha como `Scope: User config (available in all your projects)`. O uninstall lê esse escopo, remove o registro com `claude mcp remove --scope <escopo> build82` e consulta de novo até não restar nada nos escopos `user` e `local`. Se o escopo não puder ser reconhecido, nada é removido e o uninstall falha.

Os escopos `local` e `project` pertencem a um diretório; por isso, só são encontrados quando você executa o `build82 uninstall` a partir do diretório desse projeto.

Um registro `project` fica no `.mcp.json` do projeto, compartilhado com todos no projeto, por isso o build82 nunca o modifica. O uninstall ainda remove um registro `user` escondido por ele, depois imprime um aviso com o comando para você executar a partir do diretório do projeto e sai com código 0:

```bash
claude mcp remove --scope project build82
```

### `--purge`

Executa após a etapa de remoção dos registros (e somente se ela não tiver falhado com erro). Carrega a configuração ([precedência](../reference/configuration.md#precedência)) e exibe o que será apagado:

```text
--purge will delete:
  - config file: /home/you/.build82
  - 14 generated file(s) under .build82/
This cannot be undone. Proceed? [y/N]
```

A linha do arquivo de configuração aparece somente se `~/.build82` existir. Somente `y` (qualquer caixa) prossegue; caso contrário imprime `Purge cancelled.` e sai com código 0.

Apagado:

| O quê | Detalhes |
| :--- | :--- |
| `~/.build82` | O arquivo de configuração. |
| Arquivos globais | Cada um dos 13 arquivos existentes em `<raiz_moodle>/.build82/`: `AI_CONTEXT.md`, `MOODLE_API_INDEX.md`, `MOODLE_EVENTS_INDEX.md`, `MOODLE_TASKS_INDEX.md`, `MOODLE_SERVICES_INDEX.md`, `MOODLE_DB_TABLES_INDEX.md`, `MOODLE_CLASSES_INDEX.md`, `MOODLE_CAPABILITIES_INDEX.md`, `MOODLE_PLUGIN_INDEX.md`, `MOODLE_DEV_RULES.md`, `MOODLE_PLUGIN_GUIDE.md`, `MOODLE_AI_WORKSPACE.md`, `MOODLE_AI_INDEX.md`. |
| Arquivos de plugin | Para cada plugin com marcador `.build82/.indevelopment` sob a raiz do Moodle: cada um dos 12 arquivos `PLUGIN_*.md` existentes, mais o marcador `.indevelopment`. Veja [Arquivos Gerados](../reference/generated-files.md). |

Não apagado: `.build82/tags`, `.build82/.cache.json`, os próprios diretórios `.build82/`, arquivos de plugins sem o marcador `.indevelopment`, arquivos legados na raiz do Moodle ou do plugin (layout anterior ao `.build82/`), o binário, o backup `.bak` e os registros nos clientes. Se nenhuma configuração puder ser resolvida, apenas o arquivo de configuração é considerado. Um arquivo que não puder ser apagado é reportado (`failed to remove <caminho>: <erro>`) e os demais continuam; o código de saída permanece 0.

---

## Remoção manual

### Registros em clientes MCP

Use o `build82 uninstall` sempre que possível. Registros fora dos locais listados acima (por exemplo um `.mcp.json` de escopo de projeto, um `.cursor/mcp.json`, `.zed/settings.json` ou `.agents/mcp_config.json` de projeto, ou uma entrada escrita à mão em outro arquivo ou formato) não são encontrados por ele; remova-os no próprio cliente. Entradas de escopo `project` do Claude Code são informadas com o comando que as remove (veja [Escopos do Claude Code](#escopos-do-claude-code)). Cada entrada criada por `build82 install` define `BUILD82_MOODLE_PATH` em seu `env`, portanto apagar a entrada também remove essa variável. Veja os [guias de clientes](../guides/clients/claude-code.md).

### Removendo o binário

| SO | Passos |
| :--- | :--- |
| Linux, macOS | `sudo rm /usr/local/bin/build82` (ou o diretório que você escolheu, por exemplo `~/.local/bin/build82`). Localize com `command -v build82`. |
| Windows | `Remove-Item -Recurse "$env:LOCALAPPDATA\build82"` e depois remova `%LOCALAPPDATA%\build82` do `Path` do usuário (abaixo). |
| Compilado com `go install` | `rm "$(go env GOPATH)/bin/build82"` |
| Compilado com `go build` | Apague o binário que você gerou. |

Remover a entrada do `PATH` no Windows (PowerShell):

```powershell
$p = [Environment]::GetEnvironmentVariable("Path", "User") -split ';' | Where-Object { $_ -and $_ -ne "$env:LOCALAPPDATA\build82" }
[Environment]::SetEnvironmentVariable("Path", ($p -join ';'), "User")
```

### O backup `.bak`

O `build82 self-update` renomeia o binário anterior para `<caminho do binário>.bak` (por exemplo `/usr/local/bin/build82.bak`). Apague-o junto com o binário. Depois disso, `build82 self-update --rollback` deixa de ser possível.

### O arquivo de configuração

```bash
rm ~/.build82
```

Windows: `Remove-Item "$env:USERPROFILE\.build82"`. Escrito pela ferramenta `init_moodle_context`; veja a [Referência de Configuração](../reference/configuration.md).

### Diretórios `.build82/`

Ficam na raiz do Moodle e na raiz de cada plugin. O conteúdo gerado pode ser regenerado, então apagar é seguro. Nada mais nesses diretórios pertence ao seu código.

```bash
# Raiz do Moodle: arquivos globais, tags, .cache.json
rm -rf /caminho/do/moodle/.build82

# Raiz de todos os plugins
find /caminho/do/moodle -type d -name .build82 -prune -exec rm -rf {} +
```

Os marcadores `.indevelopment` ficam dentro desses diretórios (`<raiz_plugin>/.build82/.indevelopment`), então remover os diretórios também desmarca os plugins. Plugins de um layout anterior ainda podem ter arquivos `PLUGIN_*.md` ou `.indevelopment` direto na raiz do plugin, e arquivos globais ou `tags` direto na raiz do Moodle; apague-os manualmente.

Arquivos que o build82 não cria por você e não apaga: um `.buildignore` que você escreveu na raiz de um plugin, ZIPs gerados pelo `release_plugin` (`<componente>_<versão>.zip`, em `output_dir` ou no diretório de trabalho) e plugins criados pela ferramenta de scaffold.

### Configuração de `tags` no editor

Se você adicionou `set tags=./.build82/tags;` à configuração do editor (veja o README), remova.

---

## Relacionado

- [Instalação](./installation.md)
- [Referência de Configuração](../reference/configuration.md)
- [Referência da CLI](../reference/cli.md)
