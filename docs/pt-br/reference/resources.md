🌐 [English](../../en/reference/resources.md) | **Português** | 🏠 [Índice](../index.md)

---

# Referência de Resources (MCP)

Os **Resources** expõem os arquivos de contexto gerados aos clientes MCP como documentos somente leitura, sem chamada de tool. Todo resource tem MIME type `text/markdown` e retorna o conteúdo de um arquivo gerado, lido do disco a cada requisição.

## Como a IA usa os Resources

A IA decide quando ler um resource com base na sua pergunta. Você pode ser explícito para guiar a consulta:

- _"Dê uma olhada no resource `moodle://api-index` e veja se existe uma função para deletar usuários em massa."_
- _"Analise o esquema em `moodle://plugin/local_myplugin/database` e sugira índices faltando."_
- _"Antes de sugerir correções, verifique os padrões em `moodle://dev-rules`."_

Use `init_moodle_context` ou `generate_plugin_context` para gerar ou atualizar o conteúdo.

---

## Comportamento

| Aspecto | Detalhe |
|---|---|
| MIME type | `text/markdown` para os 14 resources e os 12 templates |
| Origem do conteúdo | O arquivo é lido do disco a cada requisição; o servidor não mantém cópia (exceto o cache de 5 segundos da listagem de `moodle://plugins/with-context`) |
| Não inicializado | Retorna o placeholder `# Resource not available` (`build82 has not been initialized. Run the init_moodle_context tool ...`), não um erro |
| Arquivo ainda não gerado | Retorna `# <arquivo> — Not found` com o caminho relativo esperado e a tool a executar (`init_moodle_context`/`update_indexes` para arquivos globais, `generate_plugin_context` para arquivos de plugin), não um erro |
| Erros de protocolo | Apenas quando o local da configuração não pode ser resolvido, ou em um panic interno (`internal error while reading this resource: <causa>`) |
| Quando o conteúdo muda | Quando o arquivo de origem é regenerado (veja [Arquivos Gerados](./generated-files.md) para os gatilhos): por `init_moodle_context`, `update_indexes`, `generate_plugin_context`, `plugin_batch` ou pelo watcher de `watch_plugins` |

---

## 🌍 Resources Globais

URIs fixas apoiadas por arquivos em `{raiz_moodle}/.build82/`.

| URI | Arquivo de origem | Conteúdo |
|---|---|---|
| `moodle://context` | `AI_CONTEXT.md` | Visão geral da instalação: versão, finalidade dos diretórios, APIs principais de `lib/`, diretrizes de codificação |
| `moodle://index` | `MOODLE_AI_INDEX.md` | Índice mestre com links para todos os arquivos globais gerados e todos os contextos de IA de plugins |
| `moodle://workspace` | `MOODLE_AI_WORKSPACE.md` | Guia do workspace: versão, todos os plugins, plugins em desenvolvimento, plugins com contexto de IA |
| `moodle://api-index` | `MOODLE_API_INDEX.md` | Funções da `lib/` do core (públicas e `@deprecated`), agrupadas por arquivo de origem |
| `moodle://events-index` | `MOODLE_EVENTS_INDEX.md` | Observers de eventos de todos os plugins |
| `moodle://tasks-index` | `MOODLE_TASKS_INDEX.md` | Tasks agendadas de todos os plugins |
| `moodle://services-index` | `MOODLE_SERVICES_INDEX.md` | Funções de web service de todos os plugins |
| `moodle://db-tables` | `MOODLE_DB_TABLES_INDEX.md` | Tabelas de banco de dados declaradas em todos os plugins |
| `moodle://classes-index` | `MOODLE_CLASSES_INDEX.md` | Classes, interfaces, traits e enums encontrados em diretórios `classes/` (FQNs) |
| `moodle://capabilities-index` | `MOODLE_CAPABILITIES_INDEX.md` | Capabilities declaradas em todos os plugins |
| `moodle://plugin-index` | `MOODLE_PLUGIN_INDEX.md` | Mapa component / tipo / nome / versão / caminho dos plugins instalados |
| `moodle://dev-rules` | `MOODLE_DEV_RULES.md` | Padrões de código, regras de segurança, convenções de banco de dados |
| `moodle://plugin-guide` | `MOODLE_PLUGIN_GUIDE.md` | Nomenclatura de components, arquivos obrigatórios, template de `version.php`, caminhos de classes com autoload |
| `moodle://plugins/with-context` | — | Tabela com colunas Component, Type, Path dos plugins que têm `.build82/PLUGIN_AI_CONTEXT.md`, ordenada por component, mais um trecho de uso. Calculada varrendo a árvore do Moodle (ignorando `vendor/` e `node_modules/`); o resultado fica em cache por 5 segundos |

O arquivo `tags` não é exposto como resource.

---

## 🧩 Resources de Plugin

Doze templates de URI, um por arquivo de plugin gerado. `{component}` é um component de plugin como `local_myplugin`, `mod_assign` ou `block_html`. Ele é resolvido assim: o prefixo de tipo mapeado para seu diretório (um prefixo desconhecido é usado como nome de diretório) mais o nome após o primeiro underscore; o resultado deve existir e estar dentro da raiz do Moodle, senão a resposta é `# Plugin not found` com o texto `Could not resolve "<component>" to a plugin directory under the configured Moodle root.` (nenhum caminho do sistema de arquivos é incluído). Os templates não podem ser enumerados pelo cliente; use `moodle://plugins/with-context` para listar os plugins.

| Template de URI | Arquivo de origem (`{plugin}/.build82/`) | Conteúdo |
|---|---|---|
| `moodle://plugin/{component}` | `PLUGIN_AI_CONTEXT.md` | Contexto de IA consolidado — ponto de entrada recomendado |
| `moodle://plugin/{component}/context` | `PLUGIN_CONTEXT.md` | Metadados (component, tipo, versão, requires, nome de exibição, maturity, caminho) e contagem de funcionalidades |
| `moodle://plugin/{component}/structure` | `PLUGIN_STRUCTURE.md` | Árvore de diretórios (2 níveis) e checklist de arquivos-chave |
| `moodle://plugin/{component}/architecture` | `PLUGIN_ARCHITECTURE.md` | Contagem de classes e classes agrupadas por diretório |
| `moodle://plugin/{component}/settings` | `PLUGIN_SETTINGS.md` | Configurações de admin declaradas em `settings.php` |
| `moodle://plugin/{component}/functions` | `PLUGIN_FUNCTION_INDEX.md` | Funções PHP de nível superior, agrupadas por arquivo |
| `moodle://plugin/{component}/database` | `PLUGIN_DB_TABLES.md` | Schema de `db/install.xml` |
| `moodle://plugin/{component}/events` | `PLUGIN_EVENTS.md` | Observers de eventos de `db/events.php` |
| `moodle://plugin/{component}/callbacks` | `PLUGIN_CALLBACK_INDEX.md` | Callbacks legados de `lib.php` e registros da Hook API |
| `moodle://plugin/{component}/endpoints` | `PLUGIN_ENDPOINT_INDEX.md` | Web services, endpoints AJAX e módulos AMD |
| `moodle://plugin/{component}/dependencies` | `PLUGIN_DEPENDENCIES.md` | Tasks, serviços, capabilities, Hook API, histórico de upgrades, subplugins |
| `moodle://plugin/{component}/flow` | `PLUGIN_RUNTIME_FLOW.md` | Entry points, arquivos de lógica principal e resumo de classes/eventos/tasks/serviços |

---

## Veja também

- [Referência de Tools](./tools.md) — ações que a IA pode executar ativamente
- [Referência de Prompts](./prompts.md) — templates para scaffold, revisão e debugging
- [Arquivos Gerados](./generated-files.md) — os arquivos `.md` que alimentam estes resources

---

[🏠 Voltar ao Índice](../index.md)
