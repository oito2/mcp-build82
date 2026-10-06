🌐 [English](../../en/reference/cli.md) | **Português** | 🏠 [Índice](../index.md)

---

# Referência da CLI

Binário: `build82`. Fonte da verdade: `cmd/build82/main.go` e `internal/transport/http.go`.

## Comandos

O subcomando é lido apenas a partir do primeiro argumento.

| Invocação | Descrição |
| :--- | :--- |
| `build82` | Executa o servidor MCP via stdio. |
| `build82 --http [flags]` | Executa o servidor MCP via Streamable HTTP e SSE. |
| `build82 install [alvo]` | Registra o build82 em uma ferramenta de IA. |
| `build82 uninstall [alvo] [--purge]` | Remove o registro. Veja [Desinstalação](../getting-started/uninstallation.md). |
| `build82 self-update [flags]` | Atualiza o binário para a última release. |
| `build82 -h`, `build82 --help` | Imprime o texto de ajuda no stdout e sai com código 0. |
| `build82 --version` | Imprime a versão (`dev` em um build sem ldflags de release) e sai com código 0. |

Resolução do primeiro argumento:

| Primeiro argumento | Comportamento |
| :--- | :--- |
| Nenhum | Servidor stdio. |
| `--http`, `install`, `uninstall`, `self-update`, `--help`, `-h`, `--version` | Conforme listado acima. |
| Qualquer outro valor iniciado por `-` | Tratado como flags de servidor (veja abaixo). Uma flag desconhecida, ou uma flag exclusiva do servidor sem `--http` entre os argumentos, é um erro de uso (código de saída 2). |
| Qualquer outro valor | `Error: unknown command "<valor>"`, seguido do texto de ajuda, no stderr. Código de saída 1. |

## Flags do servidor

Interpretadas por uma varredura manual dos argumentos, em qualquer ordem (`build82 --port 8080 --http` funciona). Um argumento não reconhecido, uma flag sem o seu valor ou uma flag exclusiva do servidor (`--port`, `--host`, `--token`, `--allowed-host`) sem `--http` é um erro de uso: `Error: <mensagem>` e `Run 'build82 --help' for usage.` no stderr, código de saída 2. `--help`, `-h` e `--version` também são tratados nessa varredura (imprimem e saem com 0).

| Flag | Tipo | Padrão | Aplica-se a | Descrição |
| :--- | :--- | :--- | :--- | :--- |
| `--http` | bool | desligado | — | Seleciona o transporte HTTP. Exigida por todas as demais flags de servidor. |
| `--port <n>` | inteiro 1–65535 | `3000` | `--http` | Porta TCP. Valor inválido imprime `warning: invalid --port value "<v>", keeping <n>` no stderr e mantém o valor anterior. Valor ausente é um erro de uso. O argumento seguinte é sempre consumido como valor. |
| `--host <host>` | string | `127.0.0.1` | `--http` | Endereço de bind. Também é adicionado à lista de Hosts permitidos. Valor ausente: erro de uso. |
| `--token <token>` | string | nenhum (autenticação desativada) | `--http` | Token Bearer exigido em `/mcp` e `/sse`. A última ocorrência prevalece. Valor ausente: erro de uso. |
| `--allowed-host <host>` | string, repetível | nenhum | `--http` | Valor extra aceito no cabeçalho `Host`. Valor ausente: erro de uso. |

### Resolução do token

Vale com `--http`, avaliada nesta ordem:

| # | Condição | Resultado |
| :--- | :--- | :--- |
| 1 | `--token <valor>` informado, valor não vazio | Usa o valor. `BUILD82_TOKEN` é ignorada. |
| 2 | `--token ""` informado | Erro: `--token was passed with an empty value; omit --token entirely to run --http without authentication`. Código de saída 1. |
| 3 | `--token` sem argumento seguinte | Aviso; tratada como não informada, segue para o 4. |
| 4 | `BUILD82_TOKEN` definida e não vazia | Usa o valor. |
| 5 | `BUILD82_TOKEN` definida porém vazia | Erro: `BUILD82_TOKEN is set but empty; unset it entirely to run --http without authentication`. Código de saída 1. |
| 6 | Nenhuma presente (variável não definida) | Autenticação desativada. |

Detalhes: [Referência de Configuração](./configuration.md#variáveis-de-ambiente).

## `install`

```bash
build82 install [alvo]
```

| Item | Valor |
| :--- | :--- |
| Flags | Nenhuma. No máximo um alvo posicional; qualquer flag ou segundo argumento é um erro de uso (saída 2). |
| `alvo` | `claude` (Claude Code), `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`. Omitido: todas as ferramentas detectadas. |
| Prompts | Caminho da raiz do Moodle (oferece o diretório atual quando ele é uma raiz do Moodle; validado como raiz do Moodle) e `Install build82 into all N detected tool(s)? [y/N]` quando nenhum alvo é informado. |
| Grava | A entrada `build82` na configuração de cada ferramenta, com `BUILD82_MOODLE_PATH` no ambiente e o caminho absoluto resolvido do binário em execução como comando. `claude` executa `claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=<caminho> -- <binário>`; `codex` executa `codex mcp add build82 --env BUILD82_MOODLE_PATH=<caminho> -- <binário>`. Nos dois casos, um registro existente informado por `<ferramenta> mcp get build82` é removido antes (no `claude`: todo registro `user` e `local`; um registro `project` no `.mcp.json` é mantido inalterado e informado com um aviso, veja [Claude Code](../guides/clients/claude-code.md)). Os demais alvos mesclam a entrada no seu arquivo JSON (veja os [guias de cliente](../guides/clients/claude-code.md)); `cline` escreve em todo arquivo de configurações do Cline que existir (o da CLI sob `CLINE_DATA_DIR`, quando definida), e um arquivo com comentários ou vírgulas finais nunca é reescrito (o comando imprime o trecho para colar manualmente e falha). |
| Saída | `<ferramenta>... configured.` para um registro novo, `<ferramenta>... updated.` quando um existente foi substituído, `<ferramenta>... failed: <erro>` em caso de erro; cada aviso vem em sua própria linha como `  Warning: <texto>`. |
| Código de saída | 0 ao terminar, inclusive em "No supported AI tools detected", `Skipped: <ferramenta> not detected.` e `Skipped: <ferramenta> (<motivo>).` para uma ferramenta indisponível no sistema operacional atual; 1 em alvo desconhecido, erro ao instalar em um alvo explícito, ou quando o stdin termina antes de uma raiz do Moodle válida ser informada (`no Moodle path provided (stdin closed)`). Sem alvo, uma falha por ferramenta é impressa (`<ferramenta>... failed: <erro>`) e o código de saída permanece 0. |

## `uninstall`

```bash
build82 uninstall [alvo] [--purge]
```

| Item | Valor |
| :--- | :--- |
| `alvo` | Mesmos valores de `install`. Primeiro argumento que não começa com `-`. |
| `--purge` | bool, desligado por padrão. Apaga os arquivos gerados e `~/.build82` após confirmação separada. |
| Saída | `<ferramenta>... removed.`, `<ferramenta>... not registered.`, `<ferramenta>... nothing removed.` (só foi encontrado um registro `project` do Claude Code) ou `<ferramenta>... failed: <erro>`; cada aviso vem em seguida como `  Warning: <texto>`. |
| Código de saída | 0 em sucesso, cancelamento ou nada encontrado (inclusive um registro `project` do Claude Code mantido no lugar); 1 em alvo desconhecido ou falha com alvo explícito. |

Comportamento completo: [Desinstalação](../getting-started/uninstallation.md).

## `self-update`

```bash
build82 self-update [--check] [--channel <nome>] [--yes | -y] [--rollback]
```

| Flag | Tipo | Padrão | Descrição |
| :--- | :--- | :--- | :--- |
| `--check` | bool | desligado | Informa se existe release mais nova; não instala nada. Código de saída 0 nos dois casos. |
| `--channel <nome>` | string | `stable` | Somente `stable` é aceito; qualquer outro valor falha com `unsupported channel "<nome>": only "stable" is currently supported` (saída 1). Valor ausente é um erro de uso (saída 2). |
| `--yes`, `-y` | bool | desligado | Pula o prompt `Replace the running binary with <tag>? [y/N]`. Somente `y` (qualquer caixa) confirma. |
| `--rollback` | bool | desligado | Renomeia `<binário>.bak` sobre o binário e executa `--version` nele como diagnóstico. Se aparecer em qualquer posição, todas as outras flags do self-update são ignoradas. Falha com `no backup found at <caminho> — nothing to roll back` se o backup não existir. Em caso de sucesso imprime `Rolled back to previous version at <caminho>.` |

Sequência da atualização:

| Etapa | Detalhe |
| :--- | :--- |
| 1 | Consulta `https://api.github.com/repos/oito2/mcp-build82/releases/latest` (timeout de 30 s). Um 404 imprime `No releases found.` e sai com 0. |
| 2 | Compara a tag com a versão atual como `X.Y.Z` (precedência semver: uma versão final supera a mesma versão com sufixo de pré-lançamento, como `-rc1`; metadados de build após `+` são ignorados). Um build `dev` é sempre considerado desatualizado; uma tag não interpretável nunca dispara atualização. Se não for mais nova: imprime `Already on the latest version (<tag>).`, saída 0. |
| 3 | Confirmação (exceto com `--yes`), ou parada após o relatório com `--check`. |
| 4 | Baixa `checksums.txt` e o asset `build82_<GOOS>_<GOARCH>` (`.exe` no Windows) para o diretório do binário. As URLs devem ser `https` em `github.com` ou `*.githubusercontent.com`; redirecionamentos para não-https são recusados; cada download é limitado a 200 MiB e 5 minutos. |
| 5 | Verifica o SHA-256 contra `checksums.txt`; divergência aborta antes de qualquer substituição. |
| 6 | Aplica modo `0755`, executa `<novo binário> --version` (timeout de 10 s; a saída deve ser igual à tag da release, caso contrário nada é substituído), renomeia o binário atual para `<binário>.bak` e move o novo para o lugar. |
| Código de saída | 0 em sucesso, cancelamento, já na última versão; 1 em qualquer erro. |

O caminho do binário é resolvido a partir do executável em execução, seguindo links simbólicos.

## Códigos de saída

| Código | Situação |
| :--- | :--- |
| 0 | Conclusão normal, `--help`, `--version`, prompts cancelados, encerramento gracioso do HTTP (inclusive quando o timeout de 10 s de encerramento é excedido). |
| 2 | Erro de uso: flag ou argumento desconhecido, flag sem o seu valor, flag exclusiva do servidor sem `--http`, ou argumento extra para `install`, `uninstall` ou `self-update`. Reportado como `Error: <mensagem>` seguido de `Run 'build82 --help' for usage.` no stderr. |
| 1 | Comando desconhecido; qualquer erro impresso como `Error: <mensagem>` por `install`, `uninstall`, `self-update`; erros de configuração do token; `Fatal error: <mensagem>` do servidor stdio, ou do servidor HTTP ao falhar no bind (`listen on <host>:<porta>: <causa>`). |

## Transportes

| Transporte | Selecionado por | Endpoints | Autenticação | Observações |
| :--- | :--- | :--- | :--- | :--- |
| stdio | sem `--http` | — | nenhuma | Registra `build82 server running on stdio` no stderr. Executa até o cliente desconectar. O stdout carrega o protocolo. |
| Streamable HTTP | `--http` | `/mcp` (tratado pelo handler Streamable HTTP do SDK Go) | Bearer, se houver token | Uma instância do servidor MCP por sessão. |
| SSE | `--http` | `/sse` (endpoint de mensagens tratado pelo mesmo handler) | Bearer, se houver token | Transporte legado HTTP+SSE. |
| Health | `--http` | `GET /health` | nenhuma, isento da checagem de Host | Retorna `200 {"status":"ok"}`. |

Outros caminhos retornam `404` com `{"error":"Not Found","endpoints":["/mcp","/sse","/health"]}`.

### Comportamento do servidor HTTP

| Aspecto | Valor |
| :--- | :--- |
| Endereço de escuta | `<host>:<porta>`; padrão `127.0.0.1:3000`. O bind ocorre antes das mensagens de inicialização; falha sai com 1. |
| Mensagens de inicialização (stderr) | `[build82] HTTP server listening on http://<host>:<porta>`, `MCP endpoint: http://<host>:<porta>/mcp`, `SSE endpoint: http://<host>:<porta>/sse`. |
| TLS | Nenhum. Somente HTTP simples. Use um proxy reverso para HTTPS. |
| Autenticação Bearer | Cabeçalho `Authorization: Bearer <token>` (o prefixo diferencia maiúsculas de minúsculas); comparação em tempo constante. Vale somente para `/mcp` e `/sse`. Falha: `401` `{"error":"Unauthorized","message":"Valid Bearer token required. Set Authorization: Bearer <token> header."}`. |
| Lista de Hosts permitidos | Cabeçalho `Host`, sem a porta, comparado sem diferenciar caixa com: `localhost`, `127.0.0.1`, `::1`, o valor de `--host` e cada `--allowed-host`. Não listado: `403` `{"error":"Forbidden","message":"Host \"<host>\" is not allowed. Pass --allowed-host to permit it."}`. `/health` é isento. Avaliada antes da autenticação. |
| Aviso sem token | `[build82] WARNING: no token set — /mcp and /sse are UNAUTHENTICATED` em toda inicialização sem token. |
| Aviso de host não loopback | Adicionalmente, sem token e com `--host` diferente de `127.0.0.1`, `localhost` ou `::1` (sem diferenciar caixa): `[build82] WARNING: host "<host>" is not loopback — the MCP server is exposed to the network with no authentication`. |
| `ReadHeaderTimeout` | 10 s |
| `IdleTimeout` | 120 s |
| `WriteTimeout` | Não definido (streams de longa duração). |
| Encerramento | Em `SIGINT` ou `SIGTERM`: imprime `[build82] shutting down...`, aguarda até 10 s o fim das conexões e depois as fecha. |

Exemplo:

```bash
export BUILD82_TOKEN="$(openssl rand -hex 32)"
build82 --http --port 3000
curl http://127.0.0.1:3000/health
curl -H "Authorization: Bearer $BUILD82_TOKEN" http://127.0.0.1:3000/mcp
```

## Variáveis de ambiente lidas

| Variável | Lida por | Referência |
| :--- | :--- | :--- |
| `BUILD82_TOKEN` | `--http` | [Referência de Configuração](./configuration.md#variáveis-de-ambiente) |
| `BUILD82_MOODLE_PATH`, `BUILD82_MOODLE_VERSION`, `BUILD82_MOODLE_FULLVERSION` | Servidor (todos os transportes) | [Referência de Configuração](./configuration.md#variáveis-de-ambiente) |
| `BUILD82_EXTRACTOR_BACKEND` | Servidor (todos os transportes) | [Referência de Configuração](./configuration.md#variáveis-de-ambiente) |
| `HOME` / `USERPROFILE` | Localizar `~/.build82` e os arquivos de configuração dos clientes | [Referência de Configuração](./configuration.md#o-arquivo-de-configuração) |
| `APPDATA` | Somente Windows, caminhos de configuração do Claude Desktop, Zed e da extensão Cline do VS Code | [Desinstalação](../getting-started/uninstallation.md#build82-uninstall) |
| `XDG_CONFIG_HOME` | Somente Linux, base do diretório de configuração do Claude Desktop (padrão `~/.config`) | [Desinstalação](../getting-started/uninstallation.md#build82-uninstall) |
