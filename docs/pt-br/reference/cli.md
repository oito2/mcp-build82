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
| `build82 <subcomando> -h`, `build82 <subcomando> --help` | Para `install`, `uninstall` e `self-update`: imprime o uso e as opções desse subcomando no stdout e sai com código 0, sem executá-lo. |
| `build82 --version` | Imprime a versão (`dev` em um build sem ldflags de release) e sai com código 0. |

Resolução do primeiro argumento:

| Primeiro argumento | Comportamento |
| :--- | :--- |
| Nenhum | Servidor stdio. |
| `--http`, `install`, `uninstall`, `self-update`, `--help`, `-h`, `--version` | Conforme listado acima. |
| Qualquer outro valor iniciado por `-` | Tratado como flags de servidor (veja abaixo). Uma flag desconhecida, ou uma flag exclusiva do servidor sem `--http` entre os argumentos, é um erro de uso (código de saída 2). |
| Qualquer outro valor | `Error: unknown command "<valor>"`, seguido do texto de ajuda, no stderr. Código de saída 1. |

### Ajuda dos subcomandos, `--` e interrupções

| Regra | Comportamento |
| :--- | :--- |
| Ajuda do subcomando | `-h` ou `--help` em qualquer posição entre os argumentos de `install`, `uninstall` ou `self-update` — antes de um `--` — imprime a ajuda do subcomando e sai com 0; nada mais é interpretado ou executado (`build82 self-update --check --help` só imprime a ajuda). |
| `--` | Encerra as opções. Em `install` e `uninstall`, todo argumento depois dele é o alvo, mesmo começando com `-` (`build82 uninstall -- --help` procura um alvo chamado `--help`). `self-update` e o servidor não aceitam argumento posicional, então qualquer coisa depois do `--` é erro de uso; um `--` sozinho no fim é aceito. |
| Indicação no erro de uso | Um erro de uso de `install`, `uninstall` ou `self-update` termina com `Run 'build82 <subcomando> --help' for usage.`; um erro de flag do servidor, com `Run 'build82 --help' for usage.` |
| Ctrl-C num prompt | O primeiro SIGINT ou SIGTERM enquanto `install`, `uninstall` (inclusive a confirmação do `--purge`) ou `self-update` espera uma resposta imprime `Interrupted; nothing was changed.` no stderr e sai com código 1. No `self-update`, também aborta um download em andamento (antes de o binário ser substituído). Entre dois alvos de `install`/`uninstall` sem alvo, os alvos restantes são pulados e o comando sai com código 1. Um segundo Ctrl-C encerra o processo na hora. |

## Flags do servidor

Interpretadas por uma varredura manual dos argumentos, em qualquer ordem (`build82 --port 8080 --http` funciona). Um `--` encerra as flags (nada pode vir depois dele). Um argumento não reconhecido, uma flag sem o seu valor ou uma flag exclusiva do servidor (`--port`, `--host`, `--token`, `--allowed-host`) sem `--http` é um erro de uso: `Error: <mensagem>` e `Run 'build82 --help' for usage.` no stderr, código de saída 2. `--help`, `-h` e `--version` também são tratados nessa varredura (imprimem e saem com 0).

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
| 3 | `--token` sem argumento seguinte | Erro de uso: `--token requires a value`. Código de saída 2. |
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
| Grava | A entrada `build82` na configuração de cada ferramenta, com `BUILD82_MOODLE_PATH` no ambiente e o caminho absoluto resolvido do binário em execução como comando. `claude` executa `claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=<caminho> -- <binário>`; `codex` executa `codex mcp add build82 --env BUILD82_MOODLE_PATH=<caminho> -- <binário>`. Nos dois casos, um registro existente informado por `<ferramenta> mcp get build82` é removido antes (no `claude`: todo registro `user` e `local`; um registro `project` no `.mcp.json` é mantido inalterado e informado com um aviso, veja [Claude Code](../guides/clients/claude-code.md)). Os demais alvos mesclam a entrada no seu arquivo JSON (veja os [guias de cliente](../guides/clients/claude-code.md)); `claude-desktop` escreve em todo diretório de configuração do Claude Desktop que existir (no Windows, também nos do pacote MSIX); `cline` escreve em todo arquivo de configurações do Cline que existir (o da CLI sob `CLINE_DATA_DIR` ou `CLINE_DIR`, quando definidas). As demais chaves, a ordem delas e cada valor são mantidos como foram escritos, assim como o modo do arquivo e um link simbólico no caminho. Um arquivo com comentários ou vírgulas finais nunca é reescrito: quando já tem a mesma entrada, a ferramenta conta como instalada; caso contrário, o comando imprime o trecho para colar manualmente. Feche o cliente enquanto o `install` roda, pois ele edita um arquivo que o cliente também grava. |
| Saída | `<ferramenta>... configured.` para um registro novo, `<ferramenta>... updated.` quando um existente foi substituído (ou já estava correto), `<ferramenta>... manual step needed: <instruções>` para um arquivo com comentários (JSONC), `<ferramenta>... failed: <erro>` em qualquer outro erro (inclusive um arquivo de configuração que não pode ser lido ou interpretado); cada aviso vem em sua própria linha como `  Warning: <texto>`. |
| Código de saída | 0 ao terminar, inclusive em "No supported AI tools detected", `Skipped: <ferramenta> not detected.` e `Skipped: <ferramenta> (<motivo>).` para uma ferramenta indisponível no sistema operacional atual; 1 em alvo desconhecido, quando alguma ferramenta falhou ou precisa de um passo manual (com ou sem alvo explícito; sem alvo, as demais ferramentas detectadas continuam sendo configuradas), ou quando o stdin termina antes de uma raiz do Moodle válida ser informada (`no Moodle path provided (stdin closed)`). |

## `uninstall`

```bash
build82 uninstall [alvo] [--purge]
```

| Item | Valor |
| :--- | :--- |
| `alvo` | Mesmos valores de `install`. Primeiro argumento que não começa com `-`. |
| `--purge` | bool, desligado por padrão. Apaga os arquivos gerados e `~/.build82` após confirmação separada. |
| Candidatos | Sem alvo: toda ferramenta com registro, inclusive ferramentas baseadas em arquivo que não são mais detectadas mas cujo arquivo de configuração ainda tem a entrada, e arquivos de configuração que não podem ser lidos (reportados como falha). Recusar a confirmação imprime `Nothing removed.` (com `--purge`: `Nothing removed; --purge was skipped too.`). |
| Saída | `<ferramenta>... removed.`, `<ferramenta>... not registered.`, `<ferramenta>... nothing removed.` (só foi encontrado um registro `project` do Claude Code), `<ferramenta>... manual step needed: <instruções>` (arquivo JSONC com comentários) ou `<ferramenta>... failed: <erro>`; cada aviso vem em seguida como `  Warning: <texto>`. |
| Código de saída | 0 em sucesso, cancelamento ou nada encontrado (inclusive um registro `project` do Claude Code mantido no lugar); 1 em alvo desconhecido ou quando alguma ferramenta falhou ou precisa de um passo manual, com ou sem alvo explícito. |

Comportamento completo: [Desinstalação](../getting-started/uninstallation.md).

## `self-update`

```bash
build82 self-update [--check] [--channel <nome>] [--yes | -y] [--require-signature]
build82 self-update --rollback
```

| Flag | Tipo | Padrão | Descrição |
| :--- | :--- | :--- | :--- |
| `--check` | bool | desligado | Informa se existe release mais nova; não instala nada. Sai com **10** quando há atualização e com 0 quando já está na última versão. |
| `--channel <nome>` | string | `stable` | Somente `stable` é aceito; qualquer outro valor falha com `unsupported channel "<nome>": only "stable" is currently supported` (saída 1). Valor ausente é um erro de uso (saída 2). |
| `--yes`, `-y` | bool | desligado | Pula o prompt `Replace the running binary with <tag>? [y/N]`. Somente `y` (qualquer caixa) confirma. |
| `--require-signature` | bool | desligado | Recusa a atualização, antes de baixar qualquer coisa, quando não há um cosign utilizável (v3 ou mais novo no `PATH`) para verificar a assinatura da release. Não pode ser combinada com `--check` (saída 2). |
| `--rollback` | bool | desligado | Troca o binário com `<binário>.bak`: o backup precisa passar antes em `<backup> --version` (senão nada muda); depois o binário atual é renomeado para `<binário>.bak` e o backup assume o lugar dele, então rodar `--rollback` de novo desfaz a troca. Renomear em vez de sobrescrever funciona no Windows, onde um binário em execução não pode ser substituído. Não pode ser combinada com nenhuma outra flag do self-update (saída 2). Falha com `no backup found at <caminho> — nothing to roll back` se o backup não existir. Em caso de sucesso imprime `Rolled back to <versão> at <caminho> (the replaced version is kept at <caminho>.bak; run --rollback again to undo).` |

Sequência da atualização:

| Etapa | Detalhe |
| :--- | :--- |
| 1 | Consulta `https://api.github.com/repos/oito2/mcp-build82/releases/latest` (timeout de 30 s). Um 404 imprime `No releases found.` e sai com 0. Uma tag que não seja `vX.Y.Z` (opcionalmente com sufixo `-pré-lançamento`) é recusada (saída 1). |
| 2 | Compara a tag com a versão atual como `X.Y.Z` (precedência semver: uma versão final supera a mesma versão com sufixo de pré-lançamento, como `-rc1`). Um build `dev` é sempre considerado desatualizado. Se não for mais nova: imprime `Already on the latest version (<tag>).`, saída 0. |
| 3 | Com `--check`, para após o relatório (saída 10 quando há versão mais nova). |
| 4 | Procura o `cosign` no `PATH` e lê `cosign version --json`; só a v3 ou mais nova é usada. Sem um cosign utilizável, imprime um aviso no stderr e segue apenas com o checksum — ou, com `--require-signature`, falha aqui. |
| 5 | Confirmação (exceto com `--yes`). |
| 6 | Toda URL de asset precisa ser exatamente `https://github.com/oito2/mcp-build82/releases/download/<tag>/<nome>`. Os downloads vão para o diretório do binário; cada redirecionamento (no máximo 10) precisa ser `https` em `github.com`, `objects.githubusercontent.com` ou `release-assets.githubusercontent.com`, sem porta nem credenciais; cada download é limitado a 200 MiB e 5 minutos e gravado em disco (flush). |
| 7 | Com cosign: baixa `checksums.txt.sigstore.json` e executa `cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json --certificate-identity https://github.com/oito2/mcp-build82/.github/workflows/release.yml@refs/tags/<tag> --certificate-oidc-issuer https://token.actions.githubusercontent.com`. Uma falha aborta com a saída do cosign, antes de o binário ser baixado. |
| 8 | Baixa `build82_<GOOS>_<GOARCH>` (`.exe` no Windows) e verifica o SHA-256 contra `checksums.txt`; divergência aborta antes de qualquer substituição. |
| 9 | Dá ao novo binário as permissões do binário atual (execução pelo dono sempre ligada), executa `<novo binário> --version` (timeout de 10 s; a saída deve ser igual à tag da release, caso contrário nada é substituído), renomeia o binário atual para `<binário>.bak`, move o novo para o lugar e sincroniza o diretório. Se a movimentação falhar, o backup é renomeado de volta. |
| Código de saída | 0 em sucesso, cancelamento, já na última versão; 10 com `--check` quando há atualização; 1 em qualquer erro; 2 em erro de uso. |

O caminho do binário é resolvido a partir do executável em execução, seguindo links simbólicos; assim, quando o `build82` é acessado por um symlink, o arquivo apontado é substituído e o link é mantido.

No Windows, um `.bak` anterior ainda em execução (aberto por um cliente MCP) não pode ser apagado; ele é movido para `<binário>.bak.old-<n>` e removido numa atualização futura. Um rename bloqueado por outro processo (por exemplo, a varredura de um antivírus) é repetido por até 1 segundo; se o backup não puder ser removido nem movido, o erro pede para reiniciar os clientes MCP e tentar de novo.

As releases publicadas antes da v1.1.0 não têm `checksums.txt.sigstore.json`. Atualizar para uma delas com o cosign instalado falha com `release <tag> has no asset named checksums.txt.sigstore.json`; tire o cosign do `PATH` para essa atualização.

## Códigos de saída

| Código | Situação |
| :--- | :--- |
| 0 | Conclusão normal, `--help`, `--version`, prompts cancelados, encerramento gracioso do HTTP (inclusive quando o timeout de 10 s de encerramento é excedido). |
| 10 | `self-update --check` encontrou uma release mais nova. |
| 2 | Erro de uso: flag ou argumento desconhecido, flag sem o seu valor, flag exclusiva do servidor sem `--http`, argumento extra para `install`, `uninstall` ou `self-update`, ou combinação inválida de flags do self-update (`--rollback` com outra flag, `--require-signature` com `--check`). Reportado como `Error: <mensagem>` seguido de `Run 'build82 <subcomando> --help' for usage.` (ou `Run 'build82 --help' for usage.` para flags do servidor) no stderr. |
| 1 | Comando desconhecido; um Ctrl-C num prompt de subcomando (`Interrupted; nothing was changed.`); qualquer erro impresso como `Error: <mensagem>` por `install`, `uninstall`, `self-update`; erros de configuração do token; `Fatal error: <mensagem>` do servidor stdio, ou do servidor HTTP ao falhar no bind (`listen on <host>:<porta>: <causa>`). |

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
| `LOCALAPPDATA` | Somente Windows, diretórios do pacote MSIX do Claude Desktop | [Referência de Configuração](./configuration.md#variáveis-de-ambiente) |
| `CLINE_DIR`, `CLINE_DATA_DIR` | `install`/`uninstall`, diretórios da CLI do Cline | [Referência de Configuração](./configuration.md#variáveis-de-ambiente) |
| `XDG_CONFIG_HOME` | Somente Linux, base do diretório de configuração do Claude Desktop (padrão `~/.config`) | [Desinstalação](../getting-started/uninstallation.md#build82-uninstall) |
