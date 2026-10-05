🌐 [English](../../en/reference/prompts.md) | **Português** | 🏠 [Índice](../index.md)

---

# Referência de Prompts (MCP)

Os **Prompts MCP** são templates especializados que combinam instruções precisas com o contexto em tempo real da sua instalação Moodle. Diferente de uma tool — que executa uma ação —, um prompt orienta a IA sobre **como** realizar uma tarefa complexa, injetando automaticamente padrões de código, exemplos few-shot e os índices relevantes da instalação.

---

## Como os Prompts funcionam

Cada prompt retorna uma sequência fixa de três mensagens:

| # | Role | Conteúdo |
|---|------|---------|
| 1 | `user` | Uma requisição curta de exemplo (few-shot) |
| 2 | `assistant` | A resposta do exemplo few-shot, mostrando o formato de saída esperado |
| 3 | `user` | A requisição montada: uma tabela de campos, contexto injetado (trechos truncados de arquivos), critérios/dicas da tarefa e uma instrução de formato de saída |

Não há mensagem de sistema. O contexto é lido dos arquivos gerados no momento da requisição; cada trecho é cortado no limite de bytes indicado por prompt.

**Comportamento comum:**

| Aspecto | Detalhe |
|--------|--------|
| Tipos dos argumentos | Todos os argumentos são strings |
| Argumentos obrigatórios | Validados pelo servidor. Um argumento obrigatório ausente ou em branco retorna um erro JSON-RPC `invalid params`: `missing required argument(s): <nomes>` |
| Limite de texto livre | `description`, `features`, `files`, `error` e `context` são truncados em 20.000 bytes, com `...(truncated)` ao final |
| Não inicializado | Os prompts nunca exigem inicialização. Sem configuração, a versão do Moodle e o contexto da raiz do Moodle (ex.: `MOODLE_DEV_RULES.md`) são omitidos. Para `review_plugin` e `debug_plugin`, o argumento `plugin` passa a ser usado como caminho no sistema de arquivos, tal como informado (absoluto, ou relativo ao diretório de trabalho do servidor): se esse diretório tiver arquivos `.build82/PLUGIN_*.md`, eles são lidos e injetados, e os metadados do plugin são detectados, sem verificação de raiz do Moodle |
| Resolução do plugin | `review_plugin` e `debug_plugin` resolvem `plugin` como component (`local_mytools`), caminho relativo ou caminho absoluto; se o resultado estiver fora da raiz do Moodle, ele não é lido |
| Efeitos colaterais | Nenhum (somente leitura) |
| Panics | Retornados como erro: `internal error while rendering this prompt: <causa>` |

---

## Prompts disponíveis

### `scaffold_plugin`

Monta uma requisição para projetar um novo plugin Moodle completo.

**Argumentos:**

| Argumento | Tipo | Obrigatório | Descrição |
|-----------|------|:-----------:|-----------|
| `type` | string | ✅ | Tipo do plugin (`local`, `mod`, `block`, `auth`, `tool`, `enrol`, `theme`, `report`, `format`, `filter`, `qtype`, ...). Não é validado contra uma lista; um tipo desconhecido usa o próprio nome como diretório |
| `name` | string | ✅ | Nome do plugin (minúsculas, letras e underscores). Não é validado pelo prompt |
| `description` | string | ✅ | O que o plugin faz |
| `features` | string | ❌ | Separadas por vírgula. Cada item é comparado sem diferenciar maiúsculas/minúsculas, por substring: `database`/`table`, `task`, `service`/`api`, `event`, `capabilit`/`permission`, `setting`. Outros itens (incluindo `hooks`) são ignorados |

**Como usar:**

```
scaffold_plugin
  type="local"
  name="audit_log"
  description="Registra histórico de auditoria das ações dos usuários"
  features="database tables, scheduled tasks, capabilities, event observers"
```

Em clientes que expõem prompts MCP como slash commands (ex: Gemini Code Assist, modo Agent):
```
/scaffold_plugin type="local" name="audit_log" description="Histórico de auditoria" features="database tables, capabilities"
```

**A mensagem 3 contém:** uma tabela (component, tipo, diretório de destino, versão do Moodle, descrição); uma lista "Features to Implement" (apenas quando `features` não está vazio); notas específicas do tipo (`mod`, `local`, `block`, `auth`, `tool`; uma frase genérica nos demais); os primeiros 2.000 bytes de `MOODLE_DEV_RULES.md` (ou uma lista embutida de seis padrões de código quando o arquivo não está disponível); a lista "Files to Generate" (`version.php`, `lang/en/{component}.php`, os arquivos de entrada do tipo, mais um grupo por feature reconhecida); e a instrução de formato de saída.

→ Exemplos práticos: [Scaffold de Plugins](../prompts.md#criar-um-plugin-completo-com-rascunho-da-ia)

---

### `review_plugin`

Monta uma requisição de revisão de código para um plugin existente.

**Argumentos:**

| Argumento | Tipo | Obrigatório | Descrição |
|-----------|------|:-----------:|-----------|
| `plugin` | string | ✅ | Component (`local_mytools`), caminho relativo ou caminho absoluto |
| `focus` | string | ❌ | Foco da revisão; padrão `all` (também usado quando vazio ou em branco). O valor é aparado (trim) e deve ser um dos valores abaixo, exatamente como escrito (diferencia maiúsculas de minúsculas); qualquer outro é rejeitado com um erro JSON-RPC de parâmetros inválidos (`-32602`), `invalid focus "<valor>": must be one of all, security, performance, standards, database, apis` |
| `files` | string | ❌ | Arquivos a revisar, separados por vírgula; quando definido, as instruções pedem a revisão desses arquivos em vez do plugin inteiro |

**Valores de `focus`:**

| Valor | Critérios incluídos |
|-------|---------------------|
| `all` | As cinco seções abaixo, nesta ordem |
| `security` | `require_login()`/`require_capability()` antes de ações protegidas; `required_param()`/`optional_param()` com tipos `PARAM_*` em vez de superglobais; `sesskey()` em mudanças de estado; escaping de saída (`s()`, `format_string()`, `format_text()`); SQL parametrizado |
| `performance` | Sem queries em loops (N+1); selecionar só as colunas necessárias; paginação; MUC para consultas repetidas; streaming de arquivos grandes; tratamento de timeout/retry em tasks |
| `standards` | Funções `snake_case` com prefixo do component, classes `PascalCase` compatíveis com o namespace, constantes `UPPER_CASE`; indentação de 4 espaços; PHPDoc; guarda `MOODLE_INTERNAL` |
| `database` | Tipos de coluna XMLDB, índices e chaves estrangeiras; queries parametrizadas e `MUST_EXIST`/`IGNORE_MISSING`; transações para escritas multi-etapa; `db/upgrade.php` versionado e coerente com o `version.php` |
| `apis` | Sistema de eventos em vez de handlers legados; tasks estendendo `\core\task\scheduled_task`; `external_function_parameters`/`external_value`; callbacks legados e migração para Hook API; renderers/Mustache e `moodleform` |

**Como usar:**

```
/review_plugin plugin="local_mytools" focus="security"
```

```
Execute o review_plugin no plugin local_mytools com foco em segurança
e padrões de código Moodle.
```

**A mensagem 3 contém:** uma tabela (component, versão do Moodle, tipo, versão, foco; valores vazios omitidos); os primeiros 3.000 bytes do `PLUGIN_AI_CONTEXT.md` do plugin (se gerado); os primeiros 1.500 bytes de `MOODLE_DEV_RULES.md` (se disponível); os critérios de revisão; instruções; e o formato de saída (`## Issue N — {Severity}` com Critical/High/Medium/Low, depois um `## Summary`).

→ Exemplos práticos: [Prompts de Revisão](../prompts.md#revisar-um-plugin)

---

### `debug_plugin`

Monta uma requisição de diagnóstico para um erro do Moodle em um plugin.

**Argumentos:**

| Argumento | Tipo | Obrigatório | Descrição |
|-----------|------|:-----------:|-----------|
| `plugin` | string | ✅ | Component (`local_mytools`), caminho relativo ou caminho absoluto |
| `error` | string | ✅ | Mensagem de erro completa ou stack trace |
| `context` | string | ❌ | Quando ou como o erro ocorre |

**Como usar:**

```
/debug_plugin
  plugin="local_mytools"
  error="Table 'moodle.mdl_local_mytools_sessions' doesn't exist"
  context="Ocorre ao abrir a página principal para usuários não-administradores"
```

```
Use o debug_plugin para analisar este erro no plugin local_mytools:
"Cannot find class 'local_mytools\output\renderer'"
O erro aparece somente na página de relatórios.
```

**Categorias de dica** (correspondência por substring, sem diferenciar maiúsculas/minúsculas, no texto do erro; várias categorias podem corresponder e suas dicas são concatenadas):

| Categoria | Palavras-chave |
|----------|----------|
| Capabilities | `capability`, `access denied` |
| Banco de dados | `table`, `column`, `sql` |
| Autoload | `class not found`, `autoload`, `namespace` |
| Eventos | `event`, `observer` |
| Tasks | `task`, `cron` |
| Web services / AJAX | `web service`, `external`, `ajax` |

Sem correspondência, usam-se três dicas genéricas (ativar debug DEVELOPER, checar os logs de erro do PHP/Moodle, limpar caches).

**A mensagem 3 contém:** uma tabela (component, tipo, versão, versão do Moodle); o erro em um bloco de código; "When It Occurs" (apenas quando `context` está definido); as dicas; depois, quando gerados, os primeiros 2.000 bytes de `PLUGIN_RUNTIME_FLOW.md`, os primeiros 1.500 bytes de `PLUGIN_DB_TABLES.md` e os primeiros 3.000 bytes de `PLUGIN_AI_CONTEXT.md`; e a tarefa (`## Root Cause`, `## Evidence`, `## Fix`, `## Prevention`).

→ Exemplos práticos: [Prompts de Depuração](../prompts.md#depurar-um-erro-de-plugin)

---

## Como chamar um prompt por cliente

| Cliente | Como invocar |
|---------|-------------|
| **Claude Code** | Em linguagem natural: _"Use o scaffold_plugin para..."_ ou com a sintaxe de parâmetros diretamente no chat |
| **Gemini Code Assist (Agent Mode)** | Slash command: `/scaffold_plugin`, `/review_plugin`, `/debug_plugin` — com autocomplete de parâmetros |
| **OpenAI Codex** | Em linguagem natural no chat, citando o nome do prompt: _"Execute o scaffold_plugin com type='local'..."_ |
| **OpenCode** | Em linguagem natural no chat, citando o nome do prompt: _"Use o review_plugin com foco em segurança..."_ |
| **Antigravity CLI** | A sintaxe de slash command também funciona: `/scaffold_plugin type="local" name="..."` |

> No Gemini Code Assist, os slash commands só estão disponíveis no **Agent Mode**. No chat padrão, use linguagem natural.

---

## Contribuindo com novos prompts

Se você quiser adicionar prompts customizados ao servidor (em `internal/prompts/`), siga as diretrizes em [CONTRIBUTING.md](https://github.com/oito2/mcp-build82/blob/main/CONTRIBUTING.md). Boas práticas:

- **Verifique a versão do Moodle:** injete `AI_CONTEXT.md` e oriente a IA a verificar a versão antes de sugerir Hooks (4.3+) ou APIs com `@since`.
- **Exija localização:** instrua o prompt a sugerir strings de tradução em `lang/en/` em vez de texto fixo no código.
- **Valide o schema:** prompts de banco de dados devem seguir o formato do `install.xml` e incluir os atributos `NOTNULL`, `SEQUENCE` e `NEXT`.
- **Valide argumentos obrigatórios:** chame `requireArgs` no início do handler — o SDK Go não valida sozinho as declarações `Required` de um Prompt.

---

## Veja também

- [Referência de Tools](./tools.md) — ações executáveis pela IA
- [Referência de Resources](./resources.md) — dados lidos passivamente
- [Exemplos de uso](../guides/workflows/examples.md) — cenários reais com os três prompts em ação

---

[🏠 Voltar ao Índice](../index.md)
