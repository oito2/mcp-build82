🌐 [English](../../en/concepts/glossary.md) | **Português** | 🏠 [Índice](../index.md)

---

# Glossário

Termos técnicos usados na documentação do `build82`, organizados alfabeticamente.

---

## Arquivos legados (migração)

Arquivos gerados no layout plano legado, soltos diretamente na raiz do Moodle ou do plugin em vez de sob `.build82/`. O `MigrateLegacyFiles` os move para `.build82/` no início de toda passada de geração, e o `doctor` apenas os reporta, sem alterar nada.

→ Veja: [Generators](../architecture/generators.md)

---

## Backend (extractor)

Uma das duas implementações de parsing intercambiáveis por trás da maioria dos extractors: o backend **regex** (`internal/extractors`, o padrão) e o backend opcional **tree-sitter** (`internal/extractors/tsbackend`). A escolha é feita pela variável de ambiente `BUILD82_EXTRACTOR_BACKEND` (apenas o valor `treesitter` troca; qualquer outro usa regex) e é relida a cada chamada. Extractors que leem XML ou linhas simples de cabeçalho (`schema.go`, `moodledetect.go`) e as varreduras de diagnóstico usadas pelo `doctor` não têm equivalente em tree-sitter.

→ Veja: [Extractors](../architecture/extractors.md#dois-backends-um-contrato-só)

---

## `~/.build82`

Arquivo de configuração gerado automaticamente pelo servidor quando `init_moodle_context` é executado pela primeira vez. Armazena o caminho da instalação Moodle e a versão detectada, eliminando a necessidade de passar esses dados a cada sessão.

É lido pelo pacote `internal/config` na inicialização do servidor. Variáveis de ambiente (`BUILD82_MOODLE_PATH`, `BUILD82_MOODLE_VERSION`, `BUILD82_MOODLE_FULLVERSION`) têm prioridade sobre este arquivo.

---

## `.buildignore`

Arquivo opcional na raiz de um plugin que lista nomes extras de arquivos ou diretórios (um por linha, comentários com `#` permitidos) a deixar de fora do ZIP criado pelo `release_plugin`. Os padrões são somados ao conjunto fixo de exclusões (`.build82`, `.git`, `node_modules`, arquivos de assistentes de IA, ...) e comparados como basenames exatos em qualquer profundidade.

→ Veja: [Referência de Tools](../reference/tools.md)

---

## Cache mtime

Mecanismo de cache usado pelos Generators para evitar regeneração desnecessária de arquivos. Antes de reescrever um arquivo `.md` de contexto, o servidor compara a data de modificação (`mtime`) do arquivo-fonte PHP com a do arquivo `.md` correspondente. Se o PHP não mudou desde a última geração, o arquivo é pulado. Persistido em `.build82/.cache.json`, então reiniciar o servidor não força uma nova varredura completa.

Para forçar a regeneração completa, peça ao assistente: _"Regenere todos os índices ignorando o cache"_. A IA chamará `update_indexes` com `force=true`.

→ Veja: [Sistema de Cache](../architecture/cache-system.md)

---

## Component

Identificador único de um plugin Moodle no formato **frankenstyle** — `tipo_nome`. É o valor que aparece no `version.php` de cada plugin e é usado como parâmetro nas tools do servidor.

Exemplos: `local_myplugin`, `mod_quiz`, `block_html`, `auth_ldap`.

→ Veja: [Frankenstyle](#frankenstyle)

---

## Diretório `.build82/`

O único diretório onde o `build82` escreve tudo o que gera: na raiz do Moodle (arquivos globais `MOODLE_*.md`, `AI_CONTEXT.md`, `tags`, `.cache.json`) e na raiz de cada plugin (os 12 arquivos `PLUGIN_*.md` e o marcador `.indevelopment`). O nome é fixo. Ele é excluído dos ZIPs do `release_plugin`.

→ Veja: [Arquivos Gerados](../reference/generated-files.md)

---

## Extractor

Pacote Go interno do `build82` responsável por ler e fazer parse de um tipo específico de arquivo PHP do Moodle. Cada extractor produz dados estruturados que são consumidos pelos Generators, e (na maioria dos arquivos PHP) tem dois [backends](#backend-extractor) intercambiáveis: um parser regex (padrão) e um parser opcional baseado em tree-sitter (`BUILD82_EXTRACTOR_BACKEND=treesitter`).

Exemplos: `schema.go` lê `db/install.xml`; `events.go` lê `db/events.php`; `api.go` lê `lib/*.php`.

→ Veja: [Extractors](../architecture/extractors.md) · [Como o servidor funciona](./how-build82-works.md)

---

## Frankenstyle

Convenção de nomenclatura do Moodle para identificar plugins de forma única. O formato é `tipo_nome`, onde `tipo` é o tipo do plugin (`local`, `mod`, `block`, `auth`, etc.) e `nome` é o identificador do plugin em minúsculas sem hífens.

Exemplos: `local_mytools`, `mod_checklist`, `block_coursestats`.

O frankenstyle é usado como prefixo de tabelas no banco de dados, como namespace PHP e como parâmetro nas tools do servidor (parâmetro `plugin` em `get_plugin_info`, por exemplo).

---

## Generator

Pacote Go interno do `build82` responsável por transformar os dados produzidos pelos Extractors em arquivos `.md` de contexto escritos sob `.build82/`. Existem dois tipos: generators globais (escrevem em `{raiz_moodle}/.build82/`) e generators de plugin (escrevem dentro do `{raiz_plugin}/.build82/` de cada plugin).

→ Veja: [Generators](../architecture/generators.md) · [Arquivos Gerados](../reference/generated-files.md)

---

## Hook API

Sistema de extensão do Moodle introduzido na versão 4.3 que substitui progressivamente os callbacks legados do `lib.php`. Permite que plugins se registrem para receber notificações de eventos do core de forma mais estruturada e com melhor suporte a tipagem PHP.

O `build82` detecta e indexa tanto os callbacks legados quanto as definições da Hook API em `db/hooks.php` e `classes/hook/`.

---

## `.indevelopment`

Arquivo marcador em `<raiz_do_plugin>/.build82/.indevelopment` (seu conteúdo é apenas um timestamp). É escrito pelo `generate_plugin_context` e pelo `plugin_batch` no modo `dev` (ou com `mark_as_dev`), e só a sua existência importa. É ele que transforma um plugin em um [plugin dev](#plugin-dev).

→ Veja: [Desinstalação](../getting-started/uninstallation.md)

---

## MCP (Model Context Protocol)

Padrão aberto criado pela Anthropic que define como assistentes de IA se comunicam com ferramentas e fontes de dados externas. Permite que um único servidor seja usado por qualquer cliente compatível — Claude Code, Antigravity CLI, OpenAI Codex, OpenCode e outros.

O `build82` é um servidor MCP especializado em desenvolvimento de plugins Moodle, escrito em Go e distribuído como um único binário estático.

→ Veja: [O que é MCP?](./what-is-mcp.md)

---

## Plugin dev

Um plugin marcado como em desenvolvimento, ou seja, que possui um marcador `.build82/.indevelopment`. Plugins dev são os processados pelo `plugin_batch` no modo `dev`, listados pelo `list_dev_plugins`, regenerados pelo `update_indexes` com `include_plugins` e monitorados pelo `watch_plugins` (até 20 ao mesmo tempo).

→ Veja: [`.indevelopment`](#indevelopment)

---

## `PLUGIN_AI_CONTEXT.md`

Arquivo principal de contexto gerado pelo servidor dentro do `.build82/` de cada plugin. Consolida as informações mais relevantes do plugin — arquitetura, banco de dados, funções, eventos, callbacks e fluxo de execução — em um único arquivo otimizado para consumo pelo assistente de IA.

É o ponto de entrada recomendado para iniciar qualquer sessão de desenvolvimento em um plugin existente.

→ Veja: [Arquivos Gerados](../reference/generated-files.md)

---

## Prompt (MCP)

Template de prompt pré-construído exposto pelo servidor MCP que injeta automaticamente o contexto completo do Moodle e da instalação antes de executar uma tarefa. Diferente das Tools — que executam ações —, os Prompts orientam a IA sobre **como** realizar uma tarefa complexa, com exemplos e padrões específicos do Moodle.

O `build82` expõe três prompts:

| Prompt            | Para que serve                                    |
| ------------------ | ------------------------------------------------------ |
| `scaffold_plugin` | Criar a estrutura completa de um novo plugin            |
| `review_plugin`   | Revisar o código de um plugin com foco configurável     |
| `debug_plugin`    | Depurar um erro com contexto completo do plugin         |

Em clientes que expõem prompts MCP como slash commands (ex: Gemini Code Assist, modo Agent), eles estão disponíveis como `/scaffold_plugin`, `/review_plugin`, `/debug_plugin`.

→ Veja: [Referência de Prompts](../reference/prompts.md)

---

## Resource (MCP)

Dado estruturado exposto pelo servidor MCP via URI que o cliente de IA lê passivamente como contexto — sem necessidade de chamada explícita pelo usuário. Os resources são atualizados sempre que os arquivos `.md` de contexto são regenerados.

Exemplos de URIs: `moodle://api-index`, `moodle://plugin/local_myplugin`, `moodle://db-tables`.

→ Veja: [Referência de Resources](../reference/resources.md)

---

## stdio

Modo de transporte padrão do MCP em que o servidor roda como subprocesso do cliente de IA, comunicando-se via entrada e saída padrão (stdin/stdout). Não abre portas de rede. É o modo padrão do `build82` — rodá-lo sem flags já inicia em stdio.

→ Veja: [Arquitetura — Modos de transporte](./architecture.md#modos-de-transporte)

---

## Streamable HTTP

Modo de transporte alternativo do MCP (`build82 --http`) em que o servidor roda como serviço HTTP independente. Usado quando o Moodle está em uma máquina diferente da que executa o cliente de IA (ex: servidor remoto, ambiente Docker isolado). A autenticação via token Bearer (`--token` ou a variável de ambiente `BUILD82_TOKEN`) é opcional, mas fortemente recomendada.

→ Veja: [Arquitetura — Modos de transporte](./architecture.md#modos-de-transporte) · [Docker](../guides/environments/docker.md)

---

## Tool (MCP)

Função executável exposta pelo servidor MCP que o assistente de IA pode chamar explicitamente para realizar uma ação. Diferente dos Resources — que são lidos passivamente —, as Tools são invocadas sob demanda, geralmente em resposta a um pedido do usuário em linguagem natural.

O `build82` expõe 13 tools:

| Tool                      | O que faz                                                |
| -------------------------- | -------------------------------------------------------------- |
| `init_moodle_context`     | Inicializa o contexto completo da instalação                    |
| `generate_plugin_context` | Gera contexto de IA para um plugin específico                   |
| `plugin_batch`            | Gera contexto para múltiplos plugins de uma vez                 |
| `update_indexes`          | Regenera os índices globais                                     |
| `watch_plugins`           | Monitora plugins e atualiza contexto ao salvar                  |
| `search_plugins`          | Pesquisa plugins instalados                                     |
| `search_api`              | Pesquisa funções da API core do Moodle                          |
| `get_plugin_info`         | Carrega contexto completo de um plugin na sessão                |
| `list_dev_plugins`        | Lista plugins em desenvolvimento                                |
| `doctor`                  | Diagnostica o ambiente e reporta saúde do servidor               |
| `explain_plugin`          | Explica a arquitetura de um plugin por seção                    |
| `release_plugin`          | Empacota um diretório de plugin em um ZIP distribuível           |

→ Veja: [Referência de Tools](../reference/tools.md)

---

## Watcher

O componente baseado em `fsnotify` iniciado pelo `watch_plugins`. Ele observa os arquivos-fonte de cada plugin dev (`version.php`, `lib.php`, `db/*.php`, `db/install.xml`, ...), espera 500 ms após a última alteração (debounce), invalida as marcas de cache do plugin e regenera seu contexto. Vive apenas em memória e precisa ser iniciado novamente após reiniciar o servidor.

→ Veja: [Sistema de Cache](../architecture/cache-system.md)

---

[🏠 Voltar ao Índice](../index.md)
