🌐 [Português](../../pt-br/reference/prompts.md) | **English** | 🏠 [Index](../index.md)

---

# Prompts Reference (MCP)

**MCP Prompts** are specialized templates that combine precise instructions with real-time context from your Moodle installation. Unlike a tool — which performs an action — a prompt guides the AI on **how** to perform a complex task, automatically injecting coding standards, few-shot examples, and the relevant installation indexes.

---

## How Prompts Work

Each prompt returns a fixed sequence of three messages:

| # | Role | Content |
|---|------|---------|
| 1 | `user` | A short few-shot example request |
| 2 | `assistant` | The few-shot example answer, showing the expected output format |
| 3 | `user` | The assembled request: a field table, injected context (truncated file excerpts), task-specific criteria/hints and an output-format instruction |

There is no system message. Context is read from the generated files at request time; each excerpt is cut to the byte limit listed per prompt.

**Common behavior:**

| Aspect | Detail |
|--------|--------|
| Argument types | All arguments are strings |
| Required arguments | Validated by the server. A missing or blank required argument returns a JSON-RPC `invalid params` error: `missing required argument(s): <names>` |
| Free-text limit | `description`, `features`, `files`, `error` and `context` are truncated at 20,000 bytes, with `...(truncated)` appended |
| Not initialized | Prompts never require initialization. Without a configuration, the Moodle version and the Moodle-root context (e.g. `MOODLE_DEV_RULES.md`) are omitted. For `review_plugin` and `debug_plugin`, the `plugin` argument is then used as a filesystem path as given (absolute, or relative to the server's working directory): if that directory has `.build82/PLUGIN_*.md` files they are read and injected, and its plugin metadata is detected, with no Moodle-root check |
| Plugin resolution | `review_plugin` and `debug_plugin` resolve `plugin` as a component (`local_mytools`), a relative path or an absolute path; if the result is outside the Moodle root it is not read |
| Side effects | None (read-only) |
| Panics | Returned as an error: `internal error while rendering this prompt: <cause>` |

---

## Available Prompts

### `scaffold_plugin`

Builds a request to design a complete new Moodle plugin.

**Arguments:**

| Argument | Type | Required | Description |
|----------|------|:--------:|-------------|
| `type` | string | ✅ | Plugin type (`local`, `mod`, `block`, `auth`, `tool`, `enrol`, `theme`, `report`, `format`, `filter`, `qtype`, ...). Not validated against a list; an unknown type uses its own name as the directory |
| `name` | string | ✅ | Plugin name (lowercase, letters and underscores). Not validated by the prompt |
| `description` | string | ✅ | What the plugin does |
| `features` | string | ❌ | Comma-separated. Each item is matched case-insensitively by substring: `database`/`table`, `task`, `service`/`api`, `event`, `capabilit`/`permission`, `setting`. Other items (including `hooks`) are ignored |

**How to use:**

```
scaffold_plugin
  type="local"
  name="audit_log"
  description="Records an audit history of user actions"
  features="database tables, scheduled tasks, capabilities, event observers"
```

In clients that expose MCP prompts as slash commands (e.g. Gemini Code Assist's Agent mode):

```
/scaffold_plugin type="local" name="audit_log" description="Audit history" features="database tables, capabilities"
```

**Message 3 contains:** a table (component, type, target directory, Moodle version, description); a "Features to Implement" list (only when `features` is non-empty); type-specific notes (`mod`, `local`, `block`, `auth`, `tool`; a generic sentence otherwise); the first 2,000 bytes of `MOODLE_DEV_RULES.md` (or a built-in list of six coding standards when the file is unavailable); the "Files to Generate" list (`version.php`, `lang/en/{component}.php`, the type's entry files, plus one group per recognized feature); and the output-format instruction.

→ Practical examples: [Plugin Scaffold](../prompts.md#scaffold-a-complete-plugin-with-ai-drafting)

---

### `review_plugin`

Builds a code-review request for an existing plugin.

**Arguments:**

| Argument | Type | Required | Description |
|----------|------|:--------:|-------------|
| `plugin` | string | ✅ | Component (`local_mytools`), relative path or absolute path |
| `focus` | string | ❌ | Review focus; default `all` (also used when empty or blank). The value is trimmed and must be one of the values below, exactly as written (case-sensitive); anything else is rejected with a JSON-RPC invalid-params error (`-32602`), `invalid focus "<value>": must be one of all, security, performance, standards, database, apis` |
| `files` | string | ❌ | Comma-separated files to review; when set, the instructions ask to review those files instead of the whole plugin |

**`focus` values:**

| Value | Criteria included |
|-------|-------------------|
| `all` | The five sections below, in this order |
| `security` | `require_login()`/`require_capability()` before protected actions; `required_param()`/`optional_param()` with `PARAM_*` types instead of raw superglobals; `sesskey()` on state changes; output escaping (`s()`, `format_string()`, `format_text()`); parameterised SQL |
| `performance` | No queries in loops (N+1); selecting only needed columns; pagination; MUC for repeated lookups; streaming large files; task timeout/retry handling |
| `standards` | `snake_case` functions prefixed with the component, `PascalCase` classes matching namespaces, `UPPER_CASE` constants; 4-space indentation; PHPDoc; `MOODLE_INTERNAL` guard |
| `database` | XMLDB column types, indexes and foreign keys; parameterised queries and `MUST_EXIST`/`IGNORE_MISSING`; transactions for multi-step writes; version-gated `db/upgrade.php` matching `version.php` |
| `apis` | Event system instead of legacy handlers; tasks extending `\core\task\scheduled_task`; `external_function_parameters`/`external_value`; legacy callbacks and Hook API migration; renderers/Mustache and `moodleform` |

**How to use:**

```
/review_plugin plugin="local_mytools" focus="security"
```

```
Run review_plugin on plugin local_mytools focusing on security
and Moodle coding standards.
```

**Message 3 contains:** a table (component, Moodle version, type, version, focus; empty values omitted); the first 3,000 bytes of the plugin's `PLUGIN_AI_CONTEXT.md` (if generated); the first 1,500 bytes of `MOODLE_DEV_RULES.md` (if available); the review criteria; instructions; and the output format (`## Issue N — {Severity}` with Critical/High/Medium/Low, then a `## Summary`).

→ Practical examples: [Review Prompts](../prompts.md#review-a-plugin)

---

### `debug_plugin`

Builds a diagnosis request for a Moodle error in a plugin.

**Arguments:**

| Argument | Type | Required | Description |
|----------|------|:--------:|-------------|
| `plugin` | string | ✅ | Component (`local_mytools`), relative path or absolute path |
| `error` | string | ✅ | Full error message or stack trace |
| `context` | string | ❌ | When or how the error occurs |

**How to use:**

```
/debug_plugin
  plugin="local_mytools"
  error="Table 'moodle.mdl_local_mytools_sessions' doesn't exist"
  context="Occurs when opening the main page for non-admin users"
```

```
Use debug_plugin to analyze this error in plugin local_mytools:
"Cannot find class 'local_mytools\output\renderer'"
The error appears only on the reports page.
```

**Hint categories** (case-insensitive substring match on the error text; several categories can match and their hints are concatenated):

| Category | Keywords |
|----------|----------|
| Capabilities | `capability`, `access denied` |
| Database | `table`, `column`, `sql` |
| Autoloading | `class not found`, `autoload`, `namespace` |
| Events | `event`, `observer` |
| Tasks | `task`, `cron` |
| Web services / AJAX | `web service`, `external`, `ajax` |

With no match, three generic hints are used (enable DEVELOPER debugging, check the PHP/Moodle error logs, purge caches).

**Message 3 contains:** a table (component, type, version, Moodle version); the error in a code block; "When It Occurs" (only when `context` is set); the hints; then, when generated, the first 2,000 bytes of `PLUGIN_RUNTIME_FLOW.md`, the first 1,500 bytes of `PLUGIN_DB_TABLES.md` and the first 3,000 bytes of `PLUGIN_AI_CONTEXT.md`; and the task (`## Root Cause`, `## Evidence`, `## Fix`, `## Prevention`).

→ Practical examples: [Debugging Prompts](../prompts.md#debug-a-plugin-error)

---

## How to Invoke a Prompt by Client

| Client                              | How to invoke                                                                                      |
| ----------------------------------- | --------------------------------------------------------------------------------------------------- |
| **Claude Code**                     | Natural language: _"Use scaffold_plugin to..."_ or parameter syntax directly in the chat           |
| **Gemini Code Assist (Agent Mode)** | Slash command: `/scaffold_plugin`, `/review_plugin`, `/debug_plugin` — with parameter autocomplete |
| **OpenAI Codex**                    | Natural language in chat mentioning the prompt name: _"Run scaffold_plugin with type='local'..."_  |
| **OpenCode**                        | Natural language in chat mentioning the prompt name: _"Use review_plugin focusing on security..."_ |
| **Antigravity CLI**                 | Slash command syntax also works: `/scaffold_plugin type="local" name="..."`                        |

> In Gemini Code Assist, slash commands are only available in **Agent Mode**. In the standard chat, use natural language.

---

## Contributing New Prompts

If you want to add custom prompts to the server (in `internal/prompts/`), follow the guidelines in [CONTRIBUTING.md](https://github.com/oito2/mcp-build82/blob/main/CONTRIBUTING.md). Best practices:

- **Check the Moodle version:** inject `AI_CONTEXT.md` and instruct the AI to verify the version before suggesting Hooks (4.3+) or APIs with `@since`.
- **Require localization:** instruct the prompt to suggest translation strings in `lang/en/` instead of fixed text in the code.
- **Validate the schema:** database prompts must follow the `install.xml` format and include the attributes `NOTNULL`, `SEQUENCE`, and `NEXT`.
- **Validate required arguments:** call `requireArgs` at the top of the handler — the Go SDK doesn't validate a Prompt's `Required` argument declarations for you.

---

## See also

- [Tools Reference](./tools.md) — executable actions for the AI
- [Resources Reference](./resources.md) — passively read data
- [Usage Examples](../guides/workflows/examples.md) — real scenarios using the three prompts in action

---

[🏠 Back to Index](../index.md)
