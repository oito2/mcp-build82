🌐 [English](../../en/reference/tools.md) | **Português** | 🏠 [Índice](../index.md)

---

# Referência de Tools

As 15 MCP tools expostas pelo build82, lidas dos registros e das structs de entrada em `internal/tools/`.

## Convenções

| Tópico | Comportamento |
|-------|----------|
| Parâmetro `format` | Opcional: `"text"` (padrão) ou `"json"`; uma string vazia significa o padrão. O schema de entrada declara os valores como um `enum`, então qualquer outro valor é rejeitado antes de a tool rodar. Aceito por 11 tools: `init_moodle_context`, `generate_plugin_context`, `plugin_batch`, `update_indexes`, `search_plugins`, `search_api`, `get_plugin_info`, `list_dev_plugins`, `doctor`, `bump_plugin_version`, `check_coding_style`. Escolhe só o bloco de texto: Markdown para `text`, a saída estruturada como um bloco JSON indentado para `json`. |
| Saída estruturada | Essas 11 tools declaram um `outputSchema`, e todo resultado de sucesso traz o mesmo documento como `structuredContent`, **qualquer que seja o `format`** (o "JSON" de cada tool abaixo o descreve). Clientes que leem o `structuredContent` — o Claude Code o mostra ao agente no lugar do bloco de texto — recebem, portanto, JSON nos dois formatos. |
| Sem parâmetro `format` | `watch_plugins`, `explain_plugin`, `release_plugin`, `create_plugin_skeleton` — a saída é sempre texto/Markdown, sem `outputSchema` e sem `structuredContent`. |
| Erros | Uma chamada que falha (argumento inválido, configuração ausente, plugin desconhecido, índice ausente) é um resultado de erro (`isError: true`) cujo único conteúdo é o texto da mensagem, nos dois formatos, e não traz `structuredContent`. A única exceção é o `doctor`, cujo diagnóstico com falha é um resultado de erro que ainda traz o relatório estruturado completo. |
| `doctor` | O relatório estruturado está descrito em [`doctor`](#doctor). |
| Guarda de inicialização | Toda tool, exceto `init_moodle_context`, `doctor` e `watch_plugins` (`stop`/`status`), precisa de uma configuração resolvível (`BUILD82_MOODLE_PATH` ou `~/.build82`). Caso contrário, retorna um resultado de erro: ``❌ build82 is not initialized. Run `init_moodle_context` first.`` |
| Falha de configuração | Se o local da configuração não puder ser resolvido (ex.: sem diretório home): resultado de erro `❌ Failed to resolve build82 configuration: <causa>`. |
| Panics | Um panic dentro de qualquer tool é recuperado e retornado como resultado de erro: `❌ Internal error while handling this request: <causa>`. |
| Arquivos de plugin | Arquivos lidos dos diretórios de plugin (e os arquivos gerados sob `.build82/`) só são abertos quando são arquivos regulares: um link simbólico no arquivo não é seguido e um FIFO ou dispositivo nunca é aguardado, para que um arquivo especial plantado não trave o servidor. Os arquivos gerados são escritos num arquivo temporário renomeado para o lugar, então um link simbólico plantado numa saída é substituído, não seguido, e leitores nunca veem um arquivo parcial; um diretório `.build82/` que seja link simbólico é recusado (as saídas falham com `.build82 is a symbolic link: symbolic links are not followed for writes`). |
| Identificadores de plugin | As tools que recebem um plugin aceitam formas diferentes; veja cada tool. `get_plugin_info`, `plugin_batch mode="list"`, `generate_plugin_context`, `explain_plugin`, `release_plugin`, `bump_plugin_version` e `check_coding_style` aceitam component (`local_myplugin`), caminho relativo à raiz do Moodle (`local/myplugin`) ou caminho absoluto. |
| Contenção de caminho | Todo caminho de plugin deve resolver dentro da raiz do Moodle configurada, com links simbólicos resolvidos — também para um caminho que ainda não existe, pelo diretório existente mais profundo. A própria raiz do Moodle nunca é um plugin, e um identificador de component deve ser `tipo_nome` com letras minúsculas, dígitos e sublinhados (então `mod_..` não é um caminho). Um `$plugin->component` no `version.php` que não seja um nome assim é ignorado e o plugin recebe o nome `tipo_nome` a partir do diretório. |
| Caminhos reportados | Os relatórios de geração e de plugin não incluem caminhos absolutos (o `doctor` e o `init_moodle_context` reportam o caminho do Moodle configurado, o arquivo de config e o arquivo de cache). As listas de arquivos gerados são relativas (à raiz do Moodle para arquivos globais, ao plugin para arquivos de plugin); as localizações de plugin (`Path`, `Source`, o local do esqueleto) são relativas à raiz do Moodle. |


### Anotações

Toda tool declara explicitamente as quatro dicas do MCP, mais um título, para que nenhum cliente caia nos padrões do protocolo (que supõem uma tool destrutiva e de mundo aberto). Nenhuma tool sai da instalação do Moodle (`openWorldHint: false` em todas).

| Tool | Título | `readOnlyHint` | `destructiveHint` | `idempotentHint` |
|------|-------|:---:|:---:|:---:|
| `init_moodle_context` | Initialize Moodle Context | false | false | true |
| `generate_plugin_context` | Generate Plugin Context | false | false | true |
| `plugin_batch` | Generate Context for Many Plugins | false | false | true |
| `update_indexes` | Update Indexes | false | false | true |
| `watch_plugins` | Watch Dev Plugins | false | false | true |
| `search_plugins` | Search Plugins | true | false | true |
| `search_api` | Search Moodle API | true | false | true |
| `get_plugin_info` | Get Plugin Info | true | false | true |
| `list_dev_plugins` | List Dev Plugins | true | false | true |
| `doctor` | Doctor | true | false | true |
| `explain_plugin` | Explain Plugin | true | false | true |
| `release_plugin` | Package Plugin Release | false | true (substitui um ZIP de mesmo nome) | true |
| `create_plugin_skeleton` | Create Plugin Skeleton | false | false | false (uma segunda chamada falha: o plugin já existe) |
| `bump_plugin_version` | Bump Plugin Version | false | true (edita `version.php` e `db/upgrade.php` existentes) | false (cada chamada aumenta a versão de novo) |
| `check_coding_style` | Check Coding Style | true | false | true |

Os schemas exatos de entrada e de saída de cada tool ficam como arquivos golden em `internal/server/testdata/tools/`.

---

## `init_moodle_context`

Inicializa o contexto de uma instalação Moodle: valida o caminho, detecta a versão, salva a configuração e gera todos os arquivos de índice global.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `moodle_path` | string | ✅ | — | Caminho para a raiz do Moodle. Um caminho relativo é resolvido a partir do diretório de trabalho do servidor e armazenado como caminho absoluto |
| `force` | boolean | ❌ | `false` | Reinicializa mesmo que já exista uma configuração resolvível |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Validação** (todas devem valer, senão erro `❌ Invalid Moodle path: <motivo>`): o diretório existe; `version.php` existe; `lib/` existe; `config.php` ou `config-dist.php` existe.

**Efeitos colaterais:** grava `~/.build82` (veja [Configuração](./configuration.md)); executa os 13 generators globais mais a etapa do ctags, escrevendo em `{raiz_moodle}/.build82/` e atualizando `{raiz_moodle}/.build82/.cache.json`; migra arquivos legados soltos para `.build82/`. Veja [Arquivos Gerados](./generated-files.md).

**Retorno:** relatório em texto com caminho do Moodle, versão, versão completa, caminho do arquivo de config e listas de arquivos Generated/Cached/Failed. JSON: `success`, `already_initialized`, `moodle_path`, `moodle_version`, `moodle_full_version`, `config_path`, `generated[]`, `skipped[]`, `failed[]` (`{file, error}`).

**Sobrescrita por ambiente:** quando `force` é `true` e `BUILD82_MOODLE_PATH` está definida no ambiente do servidor, o arquivo ainda é gravado, mas o relatório avisa que a variável continua tendo precedência sobre ele.

**Já inicializado:** se já existe uma configuração resolvível (variável de ambiente ou arquivo) e `force` não é `true`, a tool retorna uma mensagem sem erro com o caminho e a versão armazenados e não faz mais nada.

> Quando `BUILD82_MOODLE_PATH` está definido, a configuração sempre resolve pelo ambiente; assim, esta tool informa "already initialized" sem gerar nada, a menos que `force: true`. Nesse cenário, execute `update_indexes` para gerar os arquivos globais. Com `force: true` a tool também grava `~/.build82`, mas a variável de ambiente mantém a precedência quando a configuração é carregada.

---

## `generate_plugin_context`

Gera os 12 arquivos `PLUGIN_*.md` de um plugin e o marca como em desenvolvimento.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `plugin_path` | string | ✅ | — | Component (`local_myplugin`), caminho relativo à raiz do Moodle (`local/myplugin`) ou caminho absoluto |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Validação:** o caminho está dentro da raiz do Moodle, existe e contém `version.php`. Erros: `❌ Invalid plugin path: must be within the Moodle installation.`, `❌ Plugin directory not found: <rel>`, `❌ <rel> does not appear to be a Moodle plugin.`, `❌ Failed to detect plugin: <causa>`.

**Efeitos colaterais:** escreve os 12 arquivos em `{plugin}/.build82/` (sujeito ao cache mtime), sempre escreve `{plugin}/.build82/.indevelopment`, atualiza `{raiz_moodle}/.build82/MOODLE_AI_INDEX.md` e `.cache.json`, migra arquivos legados soltos.

**Retorno:** relatório em texto (component, tipo, versão, caminho relativo à raiz do Moodle, listas Generated/Cached/Failed, ponteiro para `.build82/PLUGIN_AI_CONTEXT.md`). JSON: `component`, `type`, `version`, `path` (relativo à raiz do Moodle), `generated[]`, `skipped[]`, `failed[]`.

> Esta tool não tem parâmetro `force` — sempre respeita o cache mtime. Para forçar a regeneração, use `plugin_batch` com `mode="list"` e `force: true`.

---

## `plugin_batch`

Gera ou atualiza o contexto de vários plugins.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `mode` | `dev`/`all`/`list` | ❌ | `dev` | `dev`: todo plugin `.indevelopment`. `all`: todo plugin encontrado nos diretórios de tipo conhecidos. `list`: os plugins em `plugins` |
| `plugins` | string[] | ❌ | — | Obrigatório e não vazio quando `mode=list`; máx. 500 itens. Cada item: component, caminho relativo ou absoluto; itens que apontam para o mesmo plugin são processados uma vez. Ignorado nos outros modos |
| `force` | boolean | ❌ | `false` | Regenera os 12 arquivos de cada plugin do lote independentemente dos mtimes dos arquivos: cada saída é marcada como desatualizada no cache antes de gerar |
| `mark_as_dev` | boolean | ❌ | `false` | Para `mode=all`/`list`, também escreve `.build82/.indevelopment`. `mode=dev` sempre marca |
| `parallel` | number | ❌ | `0` | Tamanho do pool de workers. `0` (ou negativo) = sequencial. Limitado a 16 e ao número de plugins |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Erros:** `❌ No plugins found in this Moodle installation.` (`all`); `❌ mode='list' requires a non-empty 'plugins' array.`; `❌ too many plugins in 'plugins' (max 500).`; `❌ Could not resolve the following plugin identifiers: <lista>` (não resolvível ou fora da raiz do Moodle; nada é gerado). `mode=dev` sem plugins marcados retorna uma mensagem informativa sem erro. A falha em um plugin não aborta o lote.

**Efeitos colaterais:** as mesmas escritas por plugin de `generate_plugin_context`; atualiza `MOODLE_AI_INDEX.md` e `.cache.json`.

**Retorno:** relatório em texto agrupado em Regenerated / Cached / Failed, os contadores de hit/miss/skip do cache desta chamada (`Cache (this call): …`) e uma dica ou contagem de marcação. JSON: `{mode, plugins[]}`, cada plugin `{component, path, generated, skipped, failed, error?}` (`path` relativo à raiz do Moodle; as contagens são de arquivos). No modo `dev` sem plugin marcado, a chamada tem sucesso com a lista `plugins` vazia e uma dica em texto.

---

## `update_indexes`

Regenera os 13 índices globais (e o `tags`), redetectando a versão do Moodle.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `force` | boolean | ❌ | `false` | Regenera todas as saídas globais (os 13 arquivos Markdown e o `tags`) independentemente dos mtimes dos arquivos; com `include_plugins`, também os 12 arquivos de cada plugin dev |
| `include_plugins` | boolean | ❌ | `false` | Também regenera todo plugin `.indevelopment` (e regrava seu marcador) |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Efeitos colaterais:** reescreve `~/.build82` quando a versão detectada difere da armazenada e a configuração foi carregada desse arquivo (com `BUILD82_MOODLE_PATH` definida, o arquivo não é alterado); escreve os arquivos globais e `.cache.json`; com `include_plugins`, os arquivos por plugin.

**Retorno:** relatório em texto com a versão do Moodle, contagens de arquivos globais Regenerated/Cached/Failed, linhas por plugin (quando pedido) e os contadores de cache desta chamada (`Cache (this call): …`). JSON: `moodle_version`, `regenerated[]`, `skipped[]`, `failed[]` (`{file, error}`), `plugins[]` (uma linha de resumo por plugin dev).

---

## `watch_plugins`

Inicia, para ou reporta o status do watcher de arquivos que regenera o contexto de plugins dev quando mudam. Sem parâmetro `format`.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `action` | `start`/`stop`/`status` | ❌ | `start` | Ação a executar |

**Comportamento de `start`:**

| Aspecto | Detalhe |
|--------|--------|
| Escopo | Plugins marcados `.indevelopment`, ordenados por caminho; só os 20 primeiros são monitorados (os excedentes são registrados no stderr) |
| Arquivos monitorados | Qualquer um entre `version.php`, `lib.php`, `locallib.php`, `settings.php`, `db/install.xml`, `db/access.php`, `db/events.php`, `db/tasks.php`, `db/services.php`, `db/upgrade.php`, `db/hooks.php`, `db/subplugins.json`, `db/subplugins.php` que exista quando o watcher inicia. Salvar, criar, renomear ou remover um deles dispara uma regeneração |
| Debounce | 500 ms por plugin |
| Ao mudar | Marca as 12 saídas do plugin como desatualizadas no cache (forçando a regeneração, independentemente dos mtimes), regenera seus 12 arquivos (marcando-o novamente), atualiza `MOODLE_AI_INDEX.md` e envia `notifications/resources/updated` para os resources do plugin, `moodle://index` e `moodle://plugins/with-context` às sessões que os assinaram (veja [Assinaturas](./resources.md#-assinaturas)). Também ainda envia uma notificação de log MCP (nível `info`, logger `build82/watcher`) a toda sessão conectada; o logging do MCP está obsoleto, então essa mensagem de log é transitória e pode ser removida numa release futura |
| Sem arquivos monitoráveis | Se nenhum plugin `.indevelopment` tiver arquivos monitoráveis, o watcher não é iniciado: a chamada retorna, sem erro, `ℹ️ Watcher not started — no watchable files found.` com uma dica para marcar um plugin antes, e nenhum watcher fica ativo (`status` informa que não está em execução; `stop` informa que não há nada a parar) |
| Persistência | Apenas em memória; não sobrevive a reinicializações do servidor |

**Retorno (texto):** `start` — `✅ Watcher started — monitoring <n> files across dev plugins.`, ou `ℹ️ Watcher not started — no watchable files found.` (sem erro) quando não há o que monitorar; já em execução — `⚠ Watcher is already running. Use action: 'stop' first.`; `stop` — `✔ Watcher stopped.` ou `No active watcher to stop.`; `status` — `✔ Watcher is active.` ou `Watcher is not running.`. `start` exige inicialização (resultado de erro caso contrário).

---

## `search_plugins`

Pesquisa o índice de plugins (`MOODLE_PLUGIN_INDEX.md`) por um termo.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `query` | string | ✅ | — | Termo de busca; máx. 200 caracteres |
| `limit` | number | ❌ | `20` | Máximo de resultados. Valores `<= 0` usam o padrão; valores acima de `100` são limitados a `100` |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Correspondência:** substring sem diferenciar maiúsculas/minúsculas nas linhas da tabela do índice. Se nada corresponder, um fallback fuzzy compara cada palavra de cada linha com o termo por distância de edição (distância permitida: 0 para termos de até 3 caracteres, 1 até 6, 2 acima).

**Erros:** `❌ query too long (max 200 characters).`; ``❌ Plugin index not found. Run `init_moodle_context` or `update_indexes` first.``

**Retorno:** tabela em texto `Component | Type | Name | Version | Path` (com uma nota quando fuzzy), ou `No plugins matched "<query>".` (não é erro). JSON: `{query, fuzzy, matches[]}`, onde `matches` são linhas brutas da tabela Markdown.

---

## `search_api`

Pesquisa o índice da API do Moodle (`MOODLE_API_INDEX.md`, linhas de função) por um termo.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `query` | string | ✅ | — | Nome da função ou palavra-chave no resumo; máx. 200 caracteres |
| `visibility` | `public`/`deprecated`/`all` | ❌ | `public` | `public` exclui linhas marcadas `@deprecated`; `deprecated` mantém só essas; `all` mantém ambas. Qualquer outro valor é rejeitado pelo schema de entrada |
| `limit` | number | ❌ | `30` | Máximo de resultados. Valores `<= 0` usam o padrão; valores acima de `100` são limitados a `100` |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Correspondência:** mesma estratégia exata-depois-fuzzy de `search_plugins`, sobre linhas que começam com `` - ` ``. Visibilidade e `limit` são aplicados após a correspondência.

**Erros:** `❌ query too long (max 200 characters).`; ``❌ API index not found. Run `init_moodle_context` or `update_indexes` first.``

**Retorno:** lista em texto das linhas de índice correspondentes, ou `No functions matched "<query>".` (não é erro). JSON: `{query, visibility, fuzzy, matches[]}`.

---

## `get_plugin_info`

Retorna o contexto gerado de um plugin — ou metadados detectados ao vivo, se ainda não gerado.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), caminho relativo ou caminho absoluto |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Comportamento, em ordem:**

1. Se o plugin não for encontrado dentro da raiz do Moodle (incluindo a própria raiz e diretórios sem `version.php`), o índice de plugins é pesquisado pelo valor; as linhas correspondentes são retornadas como "possible matches" num resultado de erro. Sem correspondência: erro `❌ Plugin not found: <plugin>`.
2. Se `{plugin}/.build82/PLUGIN_AI_CONTEXT.md` existir, seu conteúdo completo é o bloco de texto.
3. Caso contrário, o plugin é detectado ao vivo e retorna-se uma tabela Field/Value (Type, Version, Requires, Display name, Path) com uma nota de que `generate_plugin_context` ainda não foi executado. Erro `❌ Failed to detect plugin: <causa>` se a detecção falhar.

JSON (nos dois casos): `{path, component, type, name, version, requires, display_name, maturity, has_ai_context, ai_context?}` — os metadados detectados e, quando existe, o `PLUGIN_AI_CONTEXT.md` completo em `ai_context`.

Todos os caminhos na resposta são relativos à raiz do Moodle. Somente leitura.

---

## `list_dev_plugins`

Lista todo plugin marcado `.indevelopment`.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Retorno:** tabela `Component | Path | Has AI Context` (`✔` quando `.build82/PLUGIN_AI_CONTEXT.md` existe), ordenada por caminho. JSON: `{plugins[]}`, cada um `{component, path, has_ai_context}`. Sem plugin marcado, a chamada tem sucesso com a lista `plugins` vazia e o texto `No .indevelopment plugins found.`. Somente leitura.

**Exemplo:**
```
Quais plugins estão marcados como em desenvolvimento?
```

---

## 🏷️ Marcando Plugins em Desenvolvimento

Um plugin está "em desenvolvimento" quando tem um arquivo `.indevelopment` **dentro do seu diretório `.build82/`** (`{plugin}/.build82/.indevelopment`). O marcador é usado por `list_dev_plugins`, `watch_plugins`, `plugin_batch mode="dev"`, `update_indexes include_plugins` e `doctor`.

### Como marcar um plugin

**Opção 1 — Via assistente (ao gerar o contexto):**

`generate_plugin_context` sempre marca o plugin como efeito colateral — basta pedir:

```
Gere o contexto para o plugin local_myplugin.
```

**Opção 2 — Múltiplos plugins de uma vez:**

```
Gere o contexto para os plugins local_relatorios e local_auditoria
e marque ambos como em desenvolvimento.
```

O assistente chamará `plugin_batch` com `mode="list"` e `mark_as_dev=true`.

**Opção 3 — Manual:**

```bash
mkdir -p /seu/moodle/local/myplugin/.build82
touch /seu/moodle/local/myplugin/.build82/.indevelopment
```

> Um arquivo `.indevelopment` colocado na raiz do plugin (o layout anterior ao `.build82/`) não é reconhecido até a próxima geração naquele plugin movê-lo para `.build82/`.

### Como desmarcar um plugin

```bash
rm /seu/moodle/local/myplugin/.build82/.indevelopment
```

### Verificar quais plugins estão marcados

```
Quais plugins estão marcados como em desenvolvimento?
```

O assistente chamará `list_dev_plugins`.

---

## `doctor`

Diagnóstico do ambiente, somente leitura. Retorna um relatório em Markdown por padrão, ou o relatório estruturado como texto JSON com `format: "json"`; o relatório estruturado é o `structuredContent` nos dois formatos, também quando o veredito é `fail` (um resultado de erro).

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Seções do relatório, em ordem:**

| Seção | Verificações |
|---------|--------|
| System Dependencies | `php`, `ctags`, `git`, `phpcs` no `PATH` (ausente = aviso, "optional"; um `ctags` que não é o Universal Ctags = aviso, o arquivo `tags` é ignorado; um `phpcs` cujo `phpcs -i` falha ou não lista o padrão `moodle` = aviso, o `check_coding_style` fica indisponível) |
| Configuration | Caminho do arquivo de config, caminho e versão do Moodle. Se não inicializado: `Config — not initialized` e o relatório para (resultado sem erro). Se o local da config não puder ser resolvido: resultado de erro |
| Moodle Installation | O diretório existe e parece uma raiz de Moodle |
| Global Index Files | Cada um dos 13 arquivos globais: ausente = falha; mais antigo que 7 dias = aviso (`stale`); caso contrário, idade em dias |
| Development Plugins | Para cada plugin `.indevelopment`, a mesma verificação de atualidade nos seus 12 arquivos |
| Legacy Files Pending Migration | Contagem de arquivos legados soltos aguardando mover para `.build82/`: os globais (movidos pelo próximo `init_moodle_context` ou `update_indexes`) e os de plugins dev (movidos pela próxima execução de `generate_plugin_context` ou `plugin_batch` do plugin, ou por uma regeneração do watcher) |
| Cross-Plugin Consistency | Nomes de capability declarados por mais de um plugin dev (exige ao menos 2 plugins dev) |
| Deprecated Core API Usage | Chamadas diretas, no PHP dos plugins dev, a funções marcadas `@deprecated` no índice global da API; uma chamada global totalmente qualificada (`\nome(`) também conta. Não são sinalizadas: chamadas de método e estáticas, declarações de função (`function nome(`), instanciações de classe (`new nome(`), chamadas por variável (`$nome(`) e nomes com namespace (`ns\nome(`). Pulado sem plugins dev |
| Capability Usage | Chamadas `has_capability()`/`require_capability()` citando uma das capabilities do próprio plugin (`tipo/nome:`, a forma que o Moodle exige para todo tipo de plugin, ex.: `local/myplugin:view`) não declarada em `db/access.php` |
| Lang String Usage | Chamadas `get_string()` citando o component do próprio plugin (num módulo de atividade, `mod_forum` ou `forum`) com uma string não declarada no arquivo de idioma inglês (`lang/en/{component}.php`; `lang/en/{nome}.php` num módulo de atividade) |
| Upgrade Consistency | O `db/upgrade.php` de cada plugin dev comparado com o `db/install.xml` e o `version.php`. **fail:** um passo sem savepoint, ou cujo último savepoint é de outra versão; função ou argumentos do savepoint errados para o tipo do plugin (`upgrade_mod_savepoint(true, V, nome)` para `mod`, `upgrade_block_savepoint(true, V, nome)` para `block`, `upgrade_plugin_savepoint(true, V, tipo, nome)` nos demais); versões de passos que não crescem estritamente; um passo mais novo que `$plugin->version`; uma tabela ou campo que os passos deixam existindo e que o `db/install.xml` não define (uma instalação nova ficaria sem ele). **warn:** uma tabela ou campo que os passos removem e que o `db/install.xml` ainda define. As operações de schema são reproduzidas em ordem e só conta o estado final de cada nome; nomes que um passo posterior trata só por uma função auxiliar, ou campos nomeados por uma variável de `foreach`, são ignorados. Uma tabela adicionada ao `db/install.xml` sem passo de upgrade não é detectada (isso exige o histórico do arquivo) |
| Privacy API | O `classes/privacy/provider.php` de cada plugin dev (comentários ignorados), todos **warn**: o arquivo não existe; ele não implementa `\core_privacy\local\metadata\null_provider` nem `\core_privacy\local\metadata\provider`; ele é um `null_provider` enquanto uma tabela do `db/install.xml` tem campo `userid`, `usermodified` ou `relateduserid` ou chave estrangeira para `user` |
| Cache | Contadores de hits/misses/skips e tamanho do `.cache.json` |

**Linha de veredito:** `❌ Issues found` (qualquer falha), `⚠️ Warnings found` (qualquer aviso), senão `✅ All checks passed.`

**Saída estruturada (JSON):** um objeto com um array por seção do relatório, mais o veredito geral. Cada verificação é `{label, status, detail}`, em que `status` é `ok`, `warn` ou `fail` (`detail` é omitido quando vazio).

| Chave | Conteúdo |
|-------|---------|
| `system_dependencies` | Verificações de `php`, `ctags`, `git` |
| `configuration` | Verificações do arquivo de config, do caminho e da versão do Moodle |
| `moodle_installation` | Verificação do diretório de instalação |
| `global_index_files` | Uma verificação por arquivo global |
| `development_plugins` | Array de `{component, checks[]}`, um por plugin dev |
| `legacy_files` | A verificação de arquivos legados |
| `cross_plugin_consistency` | Verificações de capabilities duplicadas |
| `deprecated_api_usage` | Verificações de API core depreciada |
| `capability_usage` | Verificações de uso de capabilities |
| `lang_string_usage` | Verificações de uso de lang strings |
| `upgrade_consistency` | Verificações de consistência de upgrade |
| `privacy_api` | Verificações de Privacy API |
| `cache` | `{hits, misses, skips, file?, file_bytes?}`; `file`/`file_bytes` somente quando o `.cache.json` existe. Omitido nas saídas antecipadas |
| `verdict` | `ok`, `warn` ou `fail` (mesma regra da linha de veredito em texto, calculada a partir das seções de verificação; veja as saídas antecipadas abaixo) |
| `hint` | Somente quando não inicializado: ``Run `init_moodle_context` to initialize.`` |

Toda chave de seção está sempre presente como array (vazio quando a seção não rodou ou não tem nada a reportar), nunca `null`. As saídas antecipadas mantêm o mesmo formato: se o local da configuração não puder ser resolvido (resultado de erro) ou o caminho do arquivo de config não puder ser resolvido (resultado de erro), `configuration` contém a única verificação `fail` e `verdict` é `fail`; se o build82 não estiver inicializado (resultado sem erro), `configuration` contém `Config` = `fail` ("not initialized"), `verdict` é `fail` e `hint` é preenchido. Todas as outras seções ficam vazias nesses casos.

---

## `explain_plugin`

Explicação compacta e por seção de um plugin. Sem parâmetro `format` (somente Markdown).

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), caminho relativo à raiz do Moodle ou caminho absoluto |
| `section` | `all`/`overview`/`database`/`events`/`classes`/`services`/`flow` | ❌ | `all` | Seção a retornar |

**Comportamento:** com `section=all` e um `.build82/PLUGIN_AI_CONTEXT.md` existente, retorna seus primeiros 1 MiB. Caso contrário, extrai ao vivo dos fontes PHP/XML: `overview` (tabela de metadados com o caminho do plugin relativo à raiz do Moodle + checklist de arquivos-chave), `database` (tabelas com contagem de campos/chaves), `classes` (FQN, tipo, extends), `events` (observer → callback), `services` (funções de web service e capabilities), `flow` (checklist de entry points e tasks agendadas); `all` retorna todas as seções. Toda seção ao vivo, exceto `overview`, é precedida pelo overview.

**Erros:** `❌ Unknown section "<x>". Valid values: all, overview, database, events, classes, services, flow.`; os erros de caminho listados em `generate_plugin_context`. Somente leitura.

---

## `release_plugin`

Empacota um diretório de plugin em um ZIP distribuível, excluindo os arquivos gerados pelo próprio build82 e outros artefatos não distribuíveis. Sem parâmetro `format` (texto simples).

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `component` | string | ✅ | — | Identificador do plugin: component (`local_myplugin`), caminho relativo à raiz do Moodle (`local/myplugin`) ou caminho absoluto |
| `output_dir` | string | ❌ | diretório de trabalho atual | Diretório onde o ZIP será salvo. Deve já existir |
| `strict` | boolean | ❌ | `false` | Valida os requisitos de submissão do diretório de plugins do moodle.org antes de empacotar |

**Erros:** `❌ component is required: ...` (valor vazio ou em branco); `❌ Output directory does not exist: <dir>`; erros de caminho do plugin (como em `generate_plugin_context`); `❌ Could not read version from <plugin>/version.php`; `❌ Failed to create ZIP: <causa>`; no modo strict, a lista de verificações que falharam.

**Verificações de `strict:true`** (se qualquer uma falhar, o ZIP **não** é criado e todas as falhas são listadas):

| Verificação | Requisito |
|-------|-------------|
| Component | O valor solicitado é igual a `$plugin->component` no `version.php`. Para um identificador de caminho (relativo ou absoluto), o valor esperado é `{type}_{dir}` (tipo do plugin mais o nome do seu diretório) |
| `$plugin->requires` | Presente |
| `$plugin->maturity` | Presente e um de `MATURITY_ALPHA`, `MATURITY_BETA`, `MATURITY_RC`, `MATURITY_STABLE` |
| Arquivo de idioma | `lang/en/{component}.php` existe (`lang/en/{nome}.php` num módulo de atividade) |
| Privacy API | `classes/privacy/provider.php` existe |
| Bibliotecas de terceiros | `thirdpartylibs.xml` existe quando há um diretório `thirdparty/` |
| Git | Nenhum diretório `.git` no plugin |

**Saída:** ZIP `{output_dir}/{component}_{version}.zip`, usando o component e o `$plugin->version` lidos do `version.php`. Um arquivo existente com esse nome é substituído. O ZIP é gravado de forma atômica (em um arquivo temporário, depois renomeado), de modo que uma falha nunca deixa um ZIP parcial e mantém intacto um ZIP anterior. O arquivo tem uma única pasta raiz com o nome do diretório do plugin (ex.: `caedauth/`); diretórios vazios não são incluídos. Cada entrada é comprimida (Deflate) e mantém a data de modificação e os bits de permissão do arquivo (no Windows, só o bit de somente leitura).

**Excluídos do ZIP** (comparados pelo nome base em qualquer profundidade; os arquivos permanecem no projeto):

| Nome | Motivo |
|------|--------|
| `.build82/` (e os 12 nomes legados `PLUGIN_*.md` soltos) | Gerados pelo build82 |
| `.indevelopment` | Marcador de desenvolvimento |
| `.git` | Histórico do controle de versão (excluído em todos os modos) |
| `CLAUDE.md`, `GEMINI.md`, `AGENTS.md` | Arquivos de contexto de assistentes de IA |
| `.claudeignore`, `.geminiignore`, `.aiexclude` | Configurações de ferramentas de IA |
| `.buildignore` | Arquivo de configuração do próprio build82 |
| `node_modules` | Dependências |
| Nomes listados em `.buildignore` | Arquivo opcional na raiz do plugin: um nome base por linha, linhas em branco e comentários `#` ignorados |

**Retorno (texto):**
```
✅ Released local_caedauth (version 2026041000).

ZIP folder: caedauth/
Output: local_caedauth_2026041000.zip
Source: local/caedauth

Excluded from the archive:
- .indevelopment
- .build82
- CLAUDE.md
```
A primeira linha traz o component lido do `version.php`. `Source` é relativo à raiz do Moodle. `Output` é relativo à raiz do Moodle quando o ZIP é gravado dentro dela; caso contrário, é apenas o nome do arquivo (o ZIP fica em `output_dir` ou no diretório de trabalho). A lista "Excluded" contém apenas os nomes realmente encontrados no plugin.

**Links simbólicos:** links simbólicos (para arquivos ou diretórios) nunca são adicionados ao ZIP, de modo que um link apontando para fora do plugin não leva seu alvo para o arquivo. Cada link ignorado é listado após a lista "Excluded" sob `⚠️ Symbolic links skipped (never added to the archive):`, como caminho relativo ao plugin (nunca absoluto).

**Exemplo:**
```
Empacote o plugin local_caedauth para distribuição.
```

---

## `create_plugin_skeleton`

Materializa em disco a estrutura de diretórios de um novo plugin Moodle — scaffolding puramente determinístico, sem geração de lógica de negócio. Sem parâmetro `format` (texto simples).

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `type` | string | ✅ | — | Tipo do plugin; um dos valores da tabela abaixo |
| `name` | string | ✅ | — | Um nome que o Moodle aceita: para `mod`, `^[a-z][a-z0-9]*$` (sem underscores); para os demais tipos, letras minúsculas, dígitos e underscores simples, começando com letra, terminando com letra ou dígito, com pelo menos 2 caracteres |
| `display_name` | string | ❌ | `name` com `_` trocado por espaços | Valor de `$string['pluginname']` no arquivo de idioma inglês (escapado para PHP) |
| `features` | string | ❌ | — | Lista de stubs a adicionar, separados por vírgula (veja abaixo) |
| `requires` | string | ❌ | número de build do Moodle configurado | `$plugin->requires`; deve corresponder a `^\d+(\.\d+)?$`. Se nenhum número de build for conhecido, grava `0` com um comentário `TODO` |
| `maturity` | string | ❌ | `MATURITY_ALPHA` | Um de `MATURITY_ALPHA`, `MATURITY_BETA`, `MATURITY_RC`, `MATURITY_STABLE` |

**Diretório de destino:** `{raiz_moodle}/{diretório do tipo}/{name}`. Valores de `type` aceitos e seus diretórios:

| Tipo | Diretório | Tipo | Diretório |
|------|-----------|------|-----------|
| `mod` | `mod` | `block` | `blocks` |
| `local` | `local` | `tool` | `admin/tool` |
| `auth` | `auth` | `enrol` | `enrol` |
| `theme` | `theme` | `report` | `report` |
| `format` | `course/format` | `filter` | `filter` |
| `qtype` | `question/type` | `availability` | `availability/condition` |
| `assignsubmission` | `mod/assign/submission` | `assignfeedback` | `mod/assign/feedback` |
| `gradereport` | `grade/report` | `gradeimport` | `grade/import` |
| `gradeexport` | `grade/export` | `plagiarism` | `plagiarism` |
| `portfolio` | `portfolio` | `repository` | `repository` |
| `profilefield` | `user/profile/field` | `workshopform` | `mod/workshop/form` |
| `workshopallocation` | `mod/workshop/allocation` | `workshopeval` | `mod/workshop/evaluation` |
| `datafield` | `mod/data/field` | `datapreset` | `mod/data/preset` |
| `ltisource` | `mod/lti/source` | `ltiservice` | `mod/lti/service` |
| `quizaccess` | `mod/quiz/accessrule` | `scormreport` | `mod/scorm/report` |
| `tinymce` | `lib/editor/tinymce/plugins` | `atto` | `lib/editor/atto/plugins` |
| `editor` | `lib/editor` | `adminpresets` | `admin/presets` |
| `antivirus` | `lib/antivirus` | `calendartype` | `calendar/type` |
| `logstore` | `admin/tool/log/store` | `paygw` | `payment/gateway` |
| `mlbackend` | `lib/mlbackend` | `search` | `search/engine` |
| `aiplacement` | `ai/placement` | `aiprovider` | `ai/provider` |
| `cachelock` | `cache/locks` | `cachestore` | `cache/stores` |
| `communication` | `communication/provider` | `contenttype` | `contentbank/contenttype` |
| `coursereport` | `course/report` | `customfield` | `customfield/field` |
| `dataformat` | `dataformat` | `fileconverter` | `files/converter` |
| `gradepenalty` | `grade/penalty` | `gradingform` | `grade/grading/form` |
| `h5plib` | `h5p/h5plib` | `media` | `media/player` |
| `message` | `message/output` | `mnetservice` | `mnet/service` |
| `qbank` | `question/bank` | `qbehaviour` | `question/behaviour` |
| `qformat` | `question/format` | `smsgateway` | `sms/gateway` |
| `tiny` | `lib/editor/tiny/plugins` | `webservice` | `webservice` |

**Arquivos escritos:**

| Arquivo | Condição |
|--------|-----------|
| `version.php` (component, versão `YYYYMMDD00` de hoje, requires, maturity, release `1.0.0`) | Sempre |
| `lang/en/{component}.php` (`lang/en/{nome}.php` para `mod`) | Sempre |
| Arquivo(s) de entrada por tipo: `mod` → `lib.php` (`{nome}_add_instance`, `{nome}_update_instance`, `{nome}_delete_instance`, como o Moodle os chama), `index.php`, `view.php`, `mod_form.php`; `block` → `block_{name}.php`; `auth` → `auth.php`; `tool` → `index.php`; qualquer outro tipo → `lib.php` | Sempre |
| `db/install.xml` | `features` corresponde a database/table; sempre para `mod` (com a tabela de instâncias `{nome}`: `id`, `course`, `name`, `intro`, `introformat`, `timecreated`, `timemodified`) |
| `db/tasks.php` | corresponde a task |
| `db/services.php` | corresponde a service/api |
| `db/events.php` + `classes/observer.php` | corresponde a event |
| `db/access.php` | corresponde a capability/permission |
| `settings.php` | corresponde a setting |

`features` é dividido por vírgulas; cada item é comparado sem diferenciar maiúsculas/minúsculas, por substring, com `database`/`table`, `task`, `service`/`api`, `event`, `capabilit`/`permission`, `setting` (a primeira correspondência vence por item). Itens não reconhecidos são ignorados.

**Erros:** `❌ Unknown plugin type "<type>".`; `❌ name must start with a lowercase letter ...`; `❌ Resolved plugin path escapes the Moodle root.`; `❌ <caminho> already exists — create_plugin_skeleton never overwrites an existing plugin.`; `❌ Unrecognized maturity "<x>" ...`; `❌ requires must be a plain Moodle build number ...`; `❌ Failed to write <arquivo>: <causa>` (os diretórios criados por esta chamada são removidos, então nenhum esqueleto parcial é deixado para trás; se essa limpeza também falhar, a mensagem informa).

**Retorno (texto):** `✅ Created skeleton for <component> at <caminho>` (caminho relativo à raiz do Moodle; o erro "already exists" também identifica o plugin pelo caminho relativo ao Moodle) seguido da lista ordenada de arquivos escritos.

Combine com o [prompt `scaffold_plugin`](../prompts.md#criar-um-plugin-completo-com-rascunho-da-ia) para a IA preencher a implementação sobre o esqueleto.

**Exemplo:**
```
Crie o esqueleto de um novo plugin local chamado "attendance_export" com tabelas de banco de dados e uma tarefa agendada.
```

---

## `bump_plugin_version`

Aumenta o `$plugin->version` de um plugin no `version.php` e, opcionalmente, adiciona o passo de upgrade correspondente ao `db/upgrade.php`. Edição determinística: só as atribuições alteradas são reescritas, todos os outros bytes são mantidos. Todas as mudanças são calculadas e validadas antes de qualquer escrita.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), caminho relativo ou caminho absoluto |
| `version` | string | ❌ | hoje como `YYYYMMDD00`; a parte inteira da versão atual + 1 quando a de hoje não for maior | Novo `$plugin->version`; exatamente dez dígitos e maior que a versão atual |
| `release` | string | ❌ | inalterado | Novo `$plugin->release`, escrito como string PHP entre aspas simples (escapada); uma única linha sem caracteres de controle |
| `maturity` | string | ❌ | inalterado | Um de `MATURITY_ALPHA`, `MATURITY_BETA`, `MATURITY_RC`, `MATURITY_STABLE` |
| `upgrade_step` | boolean | ❌ | `false` | Também adiciona um passo de upgrade para a nova versão ao `db/upgrade.php` |
| `upgrade_description` | string | ❌ | — | Uma linha escrita como comentário no passo (sem caracteres de controle, sem `?>`) |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**`version.php`:** cada atribuição que a chamada altera (`version` sempre; `release`/`maturity` quando informados) deve aparecer exatamente uma vez fora de comentários e conter um literal simples — um número para `version`; uma string entre aspas (sem interpolação com `$`) ou um número para `release`; uma constante `MATURITY_*` para `maturity`. Caso contrário, a chamada é recusada e a atribuição deve ser editada à mão.

**Passo de upgrade** (`upgrade_step: true`): inserido antes do `return true;` final da função de upgrade do plugin — `xmldb_{name}_upgrade` para `mod`, `xmldb_{component}_upgrade` para os demais tipos — com a indentação dessa linha e as quebras de linha do arquivo (LF ou CRLF):

```php
    if ($oldversion < 2026100900) {
        // <upgrade_description, quando informado>
        // TODO: implement this upgrade step.

        // local_myplugin savepoint reached.
        upgrade_plugin_savepoint(true, 2026100900, 'local', 'myplugin');
    }
```

O savepoint corresponde ao tipo do plugin: `upgrade_mod_savepoint(true, V, nome)` para `mod`, `upgrade_block_savepoint(true, V, nome)` para `block`, `upgrade_plugin_savepoint(true, V, tipo, nome)` nos demais — a forma que a seção Upgrade Consistency do [`doctor`](#doctor) verifica. Um `db/upgrade.php` ausente é criado com uma função de upgrade que só retorna `true`, e o passo vai para ela.

**Erros** (nada é escrito): `❌ Unrecognized maturity "<x>" ...`; `❌ release must be a single line ...`; `❌ upgrade_description must be a single line ...`; `❌ version.php has no $plugin-><x> assignment; add it first`; `❌ version.php assigns $plugin-><x> <n> times ...`; `❌ version.php assigns $plugin-><x> a value that is not a plain literal ...`; `❌ version must have exactly ten digits (YYYYMMDDXX) ...`; `❌ version <v> is not greater than the current version <c>`; `❌ db/upgrade.php does not define function <f>() ...`; ``❌ <f>() in db/upgrade.php does not end with a `return true;` on its own line ...``; `❌ db/upgrade.php already has a step for version <v> ...` (um passo para a nova versão ou uma posterior); os erros de caminho do plugin listados em `generate_plugin_context`. Se a escrita do `version.php` falhar depois que o `db/upgrade.php` foi escrito, o `db/upgrade.php` é restaurado (ou removido, junto com o diretório `db/` criado pela chamada) e a mensagem informa.

**Retorna:** `✅ Bumped <component> from <antiga> to <nova>.`, o caminho do plugin relativo à raiz do Moodle, o release e a maturity escritos, os arquivos alterados (`db/upgrade.php (created)` quando novo) e o passo gerado. JSON: `{component, path, old_version, new_version, release?, maturity?, files_changed[], upgrade_file_created?, upgrade_step?}`.

Ela não regenera os arquivos de contexto do plugin; execute `generate_plugin_context` depois (ou deixe o `watch_plugins` fazer isso).

**Exemplo:**
```
Suba a versão do meu plugin local_attendance, defina o release como 1.2.0 e adicione um passo de upgrade para a nova tabela de relatórios.
```

---

## `check_coding_style`

Verifica os arquivos PHP de um plugin contra o **padrão de código do Moodle** com o [`phpcs`](https://github.com/PHPCSStandards/PHP_CodeSniffer) e o padrão [`moodlehq/moodle-cs`](https://github.com/moodlehq/moodle-cs) — ambos dependências opcionais, veja [Instalação](../getting-started/installation.md#-pré-requisitos). Somente leitura: nunca corrige arquivos.

| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), caminho relativo ou caminho absoluto |
| `severity` | string | ❌ | `error` | `error` reporta só erros; `warning` reporta também avisos |
| `files` | string[] | ❌ | o plugin inteiro | Arquivos ou diretórios a verificar, relativos ao diretório do plugin; cada um deve existir dentro dele (links simbólicos são resolvidos) |
| `limit` | integer | ❌ | `200` | Número máximo de mensagens retornadas |
| `format` | `text`/`json` | ❌ | `text` | Formato de saída |

**Comando executado** (no diretório do plugin, limitado a 120 s; sem shell):

```bash
phpcs --standard=moodle --report=json --extensions=php -q \
  --ignore=*/.build82/*,*/vendor/*,*/node_modules/* [--warning-severity=0] -- <caminhos>
```

`--warning-severity=0` é adicionado para `severity: "error"`. O phpcs sai com código diferente de zero quando encontra violações; o resultado vem do relatório JSON, qualquer que seja o código de saída. Antes de executar, a tool confere com `phpcs -i` (limitado a 30 s) que o padrão `moodle` está instalado.

**Caminhos reportados** são relativos ao diretório do plugin. Quando o `phpcs` é um wrapper que o executa em outro lugar (por exemplo, dentro de um container) e reporta outros caminhos absolutos, usa-se a parte depois do caminho do plugin relativo ao Moodle (`blocks/html/…`); por isso o container deve montar a árvore do Moodle com o mesmo layout.

**Retorna (texto):** os totais (`N error(s), N warning(s) in N file(s) checked; N fixable with phpcbf.`) e uma tabela por arquivo com violações — `Line | Col | Severity | Message | Sniff` — ordenadas por caminho, depois por linha e coluna; `✅ No violations found.` quando não há nada; um aviso `⚠ Showing N of M message(s)` quando o `limit` cortou a lista. JSON: `{component, path, standard, severity, errors, warnings, fixable, files_checked, files[], returned, truncated?}`, cada arquivo `{file, errors, warnings, messages[]}`, cada mensagem `{line, column, severity, source, message, fixable?}`. `errors`/`warnings`/`fixable` são os totais do phpcs, sem o corte do `limit`.

**Erros:** ``❌ phpcs was not found on PATH. To check the Moodle coding style, install it with `composer global require moodlehq/moodle-cs` ...``; `❌ phpcs was found, but the moodle coding standard is not installed. ...`; `❌ phpcs was found but does not work: <motivo>` (`phpcs -i` falhou ou estourou o tempo); `❌ phpcs did not finish in time (limit 2m0s)`; `❌ phpcs produced no report: <mensagem do phpcs>`; `❌ severity must be "error" or "warning", got "<x>".`; `❌ files must be paths relative to the plugin directory, got "<x>"`; `❌ "<x>" is outside the plugin directory`; `❌ "<x>" does not exist in the plugin`; os erros de caminho do plugin listados em `generate_plugin_context`. Veja [Troubleshooting](../troubleshooting/common-issues.md#check_coding_style-diz-que-falta-o-phpcs-ou-o-padrão-moodle).

Quando o phpcs está disponível, o [prompt `review_plugin`](../prompts.md#revisar-um-plugin) com foco `all` ou `standards` pede ao agente que chame esta tool primeiro.

**Exemplo:**
```
Verifique o local_attendance contra o padrão de código do Moodle, incluindo avisos.
```

---

[← Voltar ao Índice](../index.md)
