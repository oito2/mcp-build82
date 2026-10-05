🌐 [English](../../en/reference/generated-files.md) | **Português** | 🏠 [Índice](../index.md)

---

# Arquivos de Contexto Gerados

O `build82` gera arquivos Markdown estruturados diretamente na sua instalação Moodle. Eles permitem que o assistente de IA entenda a arquitetura do seu código sem ler milhares de linhas de PHP bruto.

Todo arquivo gerado vive sob um diretório `.build82/` — na raiz do Moodle para os arquivos globais, e na raiz de cada plugin para os arquivos por plugin. Nada é escrito diretamente na raiz do Moodle ou de um plugin. Um arquivo só é reescrito quando não existe ou quando algum fonte é mais novo que ele (veja [Regras de regeneração](#regras-de-regeneração)).

---

## 📂 Arquivos Globais (`{raiz_moodle}/.build82/`)

Gerados por `init_moodle_context` e `update_indexes` (13 arquivos Markdown mais `tags`).

| Arquivo | Descrição | Regenerado quando mais novo que a saída |
|---|---|---|
| `AI_CONTEXT.md` | Resumo da instalação: versão, finalidade dos diretórios, APIs principais de `lib/`, diretrizes de codificação, links para os demais índices | `version.php` de cada plugin |
| `MOODLE_AI_INDEX.md` | Índice mestre de todos os arquivos globais gerados e dos plugins com contexto de IA (também atualizado por `generate_plugin_context`, `plugin_batch` e pelo watcher) | `version.php` de cada plugin |
| `MOODLE_AI_WORKSPACE.md` | Status do workspace: versão, contagem de plugins, plugins em desenvolvimento, plugins com contexto de IA | `version.php` de cada plugin |
| `MOODLE_API_INDEX.md` | Funções da `lib/` do core com resumo, tipo de retorno, `@since` e marcas `@deprecated` | arquivos da raiz do Moodle (veja abaixo) |
| `MOODLE_CLASSES_INDEX.md` | Classes, interfaces, traits e enums PHP em diretórios `classes/` | arquivos da raiz do Moodle (veja abaixo) |
| `MOODLE_CAPABILITIES_INDEX.md` | Capabilities declaradas em todo `db/access.php` | `db/access.php` |
| `MOODLE_DB_TABLES_INDEX.md` | Tabelas declaradas em todo `db/install.xml` | `db/install.xml` |
| `MOODLE_EVENTS_INDEX.md` | Observers de eventos declarados em todo `db/events.php` | `db/events.php` |
| `MOODLE_PLUGIN_INDEX.md` | Plugins instalados com component, tipo, nome, versão e caminho | `version.php` de cada plugin |
| `MOODLE_SERVICES_INDEX.md` | Funções de web service declaradas em todo `db/services.php` | `db/services.php` |
| `MOODLE_TASKS_INDEX.md` | Tasks agendadas declaradas em todo `db/tasks.php` | `db/tasks.php` |
| `MOODLE_DEV_RULES.md` | Trechos de padrões de código: segurança, API de banco, estrutura de arquivos de plugin, eventos, tasks | arquivos da raiz do Moodle (veja abaixo) |
| `MOODLE_PLUGIN_GUIDE.md` | Nomenclatura de components, arquivos obrigatórios, template de `version.php`, caminhos de classes com autoload | arquivos da raiz do Moodle (veja abaixo) |
| `tags` | Índice de símbolos ctags do código PHP (`ctags -R --languages=PHP`, excluindo `vendor` e `node_modules`). Ignorado silenciosamente quando não há executável `ctags` no `PATH` | arquivos da raiz do Moodle (veja abaixo) |

**Arquivos da raiz do Moodle:** `version.php`, `lib/moodlelib.php` e `lib/accesslib.php` na raiz do Moodle.

Os fontes `db/*` são coletados varrendo toda a árvore do Moodle (ignorando `vendor/`, `node_modules/`, `.git/` e arquivos que são symlinks). Os `version.php` por plugin são os dos plugins encontrados nos diretórios de tipo conhecidos.

---

## 🧩 Arquivos por Plugin (`{raiz_plugin}/.build82/`)

Gerados por `generate_plugin_context`, `plugin_batch`, `update_indexes` com `include_plugins` e pelo watcher de `watch_plugins`. Criados dentro do diretório de cada plugin — por exemplo `local/myplugin/.build82/`. Os 12 arquivos compartilham os mesmos fontes (veja [Regras de regeneração](#regras-de-regeneração)).

| Arquivo | Descrição |
|---|---|
| `PLUGIN_AI_CONTEXT.md` | **Arquivo principal.** Referência rápida que combina metadados e contagens por funcionalidade (tabelas, eventos, tasks, serviços, capabilities, Hook API, classes), com links para os demais arquivos do plugin |
| `PLUGIN_CONTEXT.md` | Metadados (component, tipo, versão, requires, nome de exibição, maturity, caminho) e contagem de funcionalidades |
| `PLUGIN_STRUCTURE.md` | Árvore de diretórios (2 níveis; entradas iniciadas por ponto, `node_modules` e `vendor` omitidos) e checklist de arquivos-chave |
| `PLUGIN_ARCHITECTURE.md` | Contagem de classes e classes agrupadas por diretório |
| `PLUGIN_SETTINGS.md` | Configurações de admin declaradas em `settings.php` (entradas `admin_setting_*`) |
| `PLUGIN_RUNTIME_FLOW.md` | Entry points, arquivos de lógica principal e um resumo de classes/eventos/tasks/serviços |
| `PLUGIN_DB_TABLES.md` | Schema extraído de `db/install.xml`: tabelas, campos, tipos, chaves e índices |
| `PLUGIN_FUNCTION_INDEX.md` | Funções PHP de nível superior, agrupadas por arquivo |
| `PLUGIN_CALLBACK_INDEX.md` | Callbacks legados de `lib.php` e registros/definições da Hook API (Moodle 4.3+), com avisos de migração de legado para hook |
| `PLUGIN_EVENTS.md` | Observers de eventos registrados em `db/events.php` |
| `PLUGIN_ENDPOINT_INDEX.md` | Web services, endpoints AJAX e módulos AMD definidos pelo plugin |
| `PLUGIN_DEPENDENCIES.md` | Tasks agendadas, web services, capabilities, uso da Hook API, histórico de upgrades e tipos de subplugin hospedados |

---

## Outros Arquivos em `.build82/`

| Arquivo | Local | Descrição |
|---|---|---|
| `.indevelopment` | `{raiz_plugin}/.build82/` | Marcador que coloca o plugin em desenvolvimento. Contém um timestamp. Escrito por `generate_plugin_context`, por `plugin_batch` no modo `dev` (ou com `mark_as_dev`), por `update_indexes` com `include_plugins` e pelo watcher |
| `.cache.json` | `{raiz_moodle}/.build82/` | Marcas de atualidade persistidas do cache mtime: `{"version": 1, "entries": {"<caminho absoluto da saída>": "<timestamp>"}}`. Um arquivo ausente, corrompido ou de versão diferente é tratado como cache vazio. Gravado de forma atômica; sem lock entre processos |

---

## Regras de regeneração

Antes de cada generator executar, o cache decide se a saída está desatualizada:

0. A saída foi invalidada explicitamente (por `force: true` ou por um evento do watcher) e não foi regenerada desde então: regenerar, independentemente dos mtimes dos arquivos.
1. O arquivo de saída não existe: regenerar.
2. A saída tem uma marca de atualidade (definida quando o build82 a escreveu): regenerar apenas se algum fonte for mais novo que a marca; senão, manter.
3. Sem marca (ou a marca foi descartada no passo 2): regenerar se algum fonte for mais novo que o mtime do arquivo de saída; senão, manter.

Fontes que não existem são ignorados. `force: true` em `plugin_batch`/`update_indexes` (passo 0) regenera as saídas afetadas mesmo quando todo fonte é mais antigo que a saída (veja o [Sistema de Cache](../architecture/cache-system.md)).

**Fontes por plugin** (caminhos relativos à raiz do plugin; a mesma lista é monitorada por `watch_plugins`):

`version.php`, `lib.php`, `locallib.php`, `settings.php`, `db/install.xml`, `db/access.php`, `db/events.php`, `db/tasks.php`, `db/services.php`, `db/upgrade.php`, `db/hooks.php`, `db/subplugins.json`, `db/subplugins.php`

Mudanças em outros pontos do plugin (por exemplo arquivos em `classes/`, `lang/` ou `amd/`) não tornam, por si só, os arquivos por plugin desatualizados.

---

## Arquivo `tags` do editor

O build82 grava a saída do ctags em `{raiz_moodle}/.build82/tags`, e somente quando há um executável `ctags` (universal-ctags) no `PATH`. Usuários de Vim podem apontar o editor para ele adicionando isto ao `~/.vimrc`:

```vim
set tags=./.build82/tags;
```

O `;` final faz o Vim procurar para cima a partir do diretório do arquivo atual até encontrar um `.build82/tags`, funcionando em qualquer subdiretório da árvore do Moodle.

---

## ❓ Perguntas Frequentes

### Devo versionar esses arquivos no Git?

**Não recomendado.** Esses arquivos são gerados automaticamente e mudam sempre que o código-fonte é atualizado. Como tudo vive sob um único diretório `.build82/`, uma linha basta:

```gitignore
# arquivos de contexto do build82
.build82/
```

### Esses arquivos pesam no servidor?

Não. São arquivos de texto Markdown simples, geralmente com poucos KB cada. Apenas arquivos cujos fontes mudaram são reescritos, então execuções subsequentes são rápidas.

### Posso editar esses arquivos manualmente?

Não é recomendado. Qualquer edição manual é sobrescrita na próxima vez que o servidor regenerar o contexto. Para adicionar notas persistentes para a IA sobre um projeto ou plugin, use `CLAUDE.md` (Claude Code) ou `GEMINI.md`/`AGENTS.md` (outros clientes) na raiz da instalação. O servidor nunca toca nesses arquivos.

### Por que um arquivo não foi atualizado após eu mudar o código?

Apenas os fontes listados em [Regras de regeneração](#regras-de-regeneração) são comparados. Uma mudança fora dessa lista (por exemplo dentro de `classes/`) não dispara a regeneração. Para forçar uma atualização, o mais simples é `force: true` no `plugin_batch` (arquivos de plugin) ou no `update_indexes` (arquivos globais). Alternativas:

- Faça `touch` em um dos fontes listados (por exemplo `touch local/myplugin/version.php`) e execute `generate_plugin_context`, ou `plugin_batch` em `mode="list"`.
- Ou apague o arquivo gerado (ou todo o diretório `.build82/` do plugin) e execute a tool novamente; uma saída ausente é sempre regenerada.
- Para arquivos globais, apague o arquivo em `{raiz_moodle}/.build82/` e execute `update_indexes`.

### O que aconteceu com o layout antigo de arquivos soltos?

Arquivos no layout plano legado ficam diretamente na raiz do Moodle/plugin, em vez de sob `.build82/`. Toda execução de generator primeiro move qualquer arquivo legado encontrado para o novo local — nenhuma limpeza manual necessária. O `doctor` informa quantos arquivos legados estão pendentes.

---

## Veja também

- [Como o servidor funciona](../concepts/how-build82-works.md) — pipeline de Extractors e Generators
- [Sistema de Cache](../architecture/cache-system.md) — estratégia de cache e invalidação
- [Referência de Resources](./resources.md) — como esses arquivos são expostos via URIs MCP
- [Referência de Tools](./tools.md) — tools que geram e atualizam estes arquivos
- [Desinstalação](../getting-started/uninstallation.md) — remoção dos arquivos gerados

---

[🏠 Voltar ao Índice](../index.md)
