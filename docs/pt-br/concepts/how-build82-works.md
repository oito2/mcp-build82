🌐 [English](../../en/concepts/how-build82-works.md) | **Português** | 🏠 [Índice](../index.md)

---

# Como o build82 funciona

O `build82` lê sua instalação Moodle do disco e dá ao seu assistente de IA conhecimento profundo e preciso da sua base de código — sem copiar e colar nada manualmente.

---

## O pipeline

O fluxo tem três etapas: os **Extractors** leem os arquivos PHP, os **Generators** transformam esse conteúdo em arquivos `.md` de contexto, e o **Servidor MCP** serve esse contexto ao cliente de IA via Tools, Resources e Prompts.

```
Arquivos PHP do Moodle (em disco)
         │
         ▼  Extractors
    Fazem parse de db/install.xml, db/events.php, db/hooks.php,
    lib/*.php, classes/, db/tasks.php, db/services.php,
    db/access.php, db/upgrade.php
         │
         ▼  Generators + cache mtime
    Escrevem arquivos .md de contexto sob .build82/:
    MOODLE_API_INDEX.md, PLUGIN_AI_CONTEXT.md, etc.
    (arquivos inalterados são ignorados via cache mtime)
         │
         ▼  Servidor MCP
    Serve contexto via Tools, Resources e Prompts
    para o cliente de IA via stdio ou HTTP
```

---

## Extractors

Cada extractor faz parse de um tipo de arquivo PHP ou XML do Moodle e alimenta os generators com dados estruturados. A maioria dos extractors que fazem parse de PHP tem dois backends intercambiáveis: um parser baseado em regex (padrão, rápido) e um parser opcional baseado em tree-sitter (`BUILD82_EXTRACTOR_BACKEND=treesitter`), que troca um pouco de velocidade por alguns casos extremos que o backend regex não consegue expressar — veja [Extractors](../architecture/extractors.md#dois-backends-um-contrato-só) para saber quais extractors têm os dois e quando isso realmente importa.

| Extractor           | Faz parse de                      | Produz                                                            |
| -------------------- | ------------------------------------ | ---------------------------------------------------------------------- |
| `api.go`             | `lib/*.php`                          | Funções com visibilidade PHPDoc → `MOODLE_API_INDEX.md`                |
| `schema.go`          | `db/install.xml`                     | Schema do banco (tabelas, campos, chaves) → `PLUGIN_DB_TABLES.md`      |
| `events.go`          | `db/events.php`                      | Registros de observers de eventos → `PLUGIN_EVENTS.md`                 |
| `hooks.go`           | `db/hooks.php` + `classes/hook/`     | Callbacks da Hook API (4.3+) e avisos de callback legado → `PLUGIN_CALLBACK_INDEX.md` |
| `tasks.go`           | `db/tasks.php`                       | Definições de tasks agendadas → `PLUGIN_DEPENDENCIES.md`               |
| `services.go`        | `db/services.php`                    | Registros de web services → `PLUGIN_ENDPOINT_INDEX.md`                |
| `capabilities.go`    | `db/access.php`                      | Definições de capabilities → `PLUGIN_DEPENDENCIES.md`                  |
| `upgrade.go`         | `db/upgrade.php`                     | Histórico de steps de upgrade → `PLUGIN_DEPENDENCIES.md`               |
| `classes.go`         | `classes/**/*.php`                   | Classes, interfaces, traits e enums PHP → `MOODLE_CLASSES_INDEX.md`, `PLUGIN_ARCHITECTURE.md` |
| `plugin.go`          | `version.php` + arquivos de lang     | Metadados do plugin → `PLUGIN_CONTEXT.md`                              |
| `settings.go`        | `settings.php`                       | Configurações de admin (`admin_setting_*`) → `PLUGIN_SETTINGS.md`      |
| `subplugins.go`      | `db/subplugins.json` (ou `db/subplugins.php` legado) | Tipos de subplugin → `PLUGIN_DEPENDENCIES.md`, `PLUGIN_AI_CONTEXT.md` |

Para a lista completa dos arquivos gerados, veja a [Referência de Arquivos Gerados](../reference/generated-files.md).

---

## Generators

Os generators recebem a saída dos extractors e escrevem arquivos Markdown estruturados sob `.build82/` — nunca diretamente na raiz do Moodle ou de um plugin:

- **Generators globais** — escrevem 13 arquivos em `{raiz_moodle}/.build82/` (`MOODLE_API_INDEX.md`, `MOODLE_PLUGIN_INDEX.md`, `MOODLE_DB_TABLES_INDEX.md`, etc.)
- **Generators de plugin** — escrevem 12 arquivos em `{raiz_plugin}/.build82/` (`PLUGIN_AI_CONTEXT.md`, `PLUGIN_DB_TABLES.md`, `PLUGIN_FUNCTION_INDEX.md`, etc.)

O **cache mtime** compara a data de modificação de cada arquivo-fonte com a do arquivo `.md` correspondente e pula a regeneração quando nada mudou — tornando execuções subsequentes muito mais rápidas. Ele é persistido em `.build82/.cache.json`, então reiniciar o servidor não força uma nova varredura completa.

> Para forçar a regeneração completa ignorando o cache, peça ao assistente: _"Regenere todos os índices do Moodle ignorando o cache"_. A IA chamará `update_indexes` com `force=true`.

---

## Tools, Resources e Prompts

Com os arquivos de contexto criados, o servidor MCP os expõe ao cliente de IA de três formas:

- **Tools** — a IA as chama explicitamente para disparar ações (`init_moodle_context`, `search_api`, `get_plugin_info`, `watch_plugins`, etc.)
- **Resources** — a IA os lê passivamente como contexto, sem ação explícita do usuário (`moodle://api-index`, `moodle://plugin/{component}`, etc.)
- **Prompts** — templates pré-construídos que injetam contexto automaticamente e guiam a IA em tarefas complexas (`scaffold_plugin`, `review_plugin`, `debug_plugin`)

---

## Veja também

- [Por que build82?](./why-build82.md)
- [Arquitetura](./architecture.md)
- [Referência de Tools](../reference/tools.md)
- [Referência de Arquivos Gerados](../reference/generated-files.md)

---

[🏠 Voltar ao Índice](../index.md)
