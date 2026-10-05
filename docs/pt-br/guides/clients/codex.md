🌐 [English](../../../en/guides/clients/codex.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com OpenAI Codex

O **OpenAI Codex** é o agente de desenvolvimento da OpenAI, disponível como CLI (`codex`) e como extensão para VS Code. Diferente dos outros clientes, o Codex usa o formato **TOML** para configuração e um arquivo **`AGENTS.md`** como contexto persistente — em vez de JSON e `CLAUDE.md`/`GEMINI.md`.

> **Importante:** A CLI e a extensão VS Code compartilham o mesmo arquivo de configuração `~/.codex/config.toml`. Configurar em um lugar ativa para os dois.

---

## 🛠️ Configuração do Servidor MCP

### Opção 1 — Deixe o build82 configurar

```bash
build82 install codex
```

Exige o comando `codex` no `PATH` (caso contrário imprime `Skipped: OpenAI Codex CLI not detected.` e não altera nada). Ele pergunta a raiz do Moodle e então executa exatamente:

```bash
codex mcp add build82 --env BUILD82_MOODLE_PATH=<raiz-do-moodle> -- <caminho-absoluto-do-build82>
```

Ou seja, é o próprio Codex que grava a entrada em `~/.codex/config.toml`, o mesmo resultado da Opção 2. Imprime `OpenAI Codex CLI... configured.` Executar o comando de novo substitui o registro existente: quando o `codex mcp get build82` informa um registro, o instalador primeiro executa `codex mcp remove build82` e depois o adiciona de novo com o caminho atual do binário e o `BUILD82_MOODLE_PATH`, imprimindo `OpenAI Codex CLI... updated.`

### Opção 2 — Pela CLI do Codex

```bash
codex mcp add build82 \
  --env BUILD82_MOODLE_PATH=/home/usuario/workspace/www/html/moodle \
  -- /usr/local/bin/build82
```

Isso grava a entrada em `~/.codex/config.toml`. Verifique:

```bash
codex mcp list
```

Você também pode executar `/mcp` dentro da TUI do Codex para ver os servidores ativos.

### Opção 3 — Editando o config.toml diretamente

Global (todos os projetos) — crie ou edite `~/.codex/config.toml`:

```toml
[mcp_servers.build82]
command = "/usr/local/bin/build82"
args    = []
env     = { BUILD82_MOODLE_PATH = "/home/usuario/workspace/www/html/moodle" }
```

Escopo de projeto — a mesma tabela em `.codex/config.toml` na raiz do workspace (carregado apenas em projetos confiáveis).

> **Atenção ao TOML:** um erro de sintaxe no `config.toml` quebra **tanto** o CLI quanto a extensão VS Code simultaneamente. Valide com:
> ```bash
> python3 -c "import tomllib; tomllib.load(open('/home/usuario/.codex/config.toml', 'rb'))"
> ```

Use o caminho absoluto em `command` se o Codex não herdar o `PATH` do seu shell (`which build82`).

### Remoção

```bash
build82 uninstall codex
```

Isso executa `codex mcp get build82` para verificar se existe um registro e, somente se existir, `codex mcp remove build82`. Imprime `OpenAI Codex CLI... removed.`, ou `OpenAI Codex CLI... not registered.` (código de saída 0) quando nada está registrado, e imprime `Skipped: OpenAI Codex CLI not detected.` se o `codex` não estiver no `PATH`. Você também pode executar `codex mcp remove build82` por conta própria, ou apagar a tabela `[mcp_servers.build82]` do `config.toml`.

Referência oficial: [documentação de MCP do Codex](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

---

## 📄 Turbinando com AGENTS.md

O `AGENTS.md` é o equivalente do `CLAUDE.md` e do `GEMINI.md` no Codex. É lido automaticamente a cada sessão — do escopo global ao mais específico:

| Escopo | Localização |
|--------|-------------|
| Global | `~/.codex/AGENTS.md` |
| Raiz do projeto | `AGENTS.md` na raiz do workspace (Git root) |
| Subdiretório | `AGENTS.md` em qualquer subpasta do projeto |

O Codex carrega os arquivos em cascata — do global ao mais específico — e o mais próximo ao diretório atual tem precedência.

Crie o arquivo na raiz da sua instalação Moodle:

```bash
touch /home/usuario/workspace/www/html/moodle/AGENTS.md
```

**Template recomendado:**

```markdown
# Contexto de Desenvolvimento Moodle

## Ambiente
- Versão do Moodle: 4.4 (ajuste conforme sua instalação)
- Caminho: /home/usuario/workspace/www/html/moodle
- Stack: Docker com Nginx + PHP-FPM + MariaDB

## build82
O servidor MCP build82 está configurado.
Use get_plugin_info para carregar contexto antes de analisar um plugin.
Use search_api para encontrar funções do core antes de sugerir alternativas.
Após mudanças significativas em um plugin, execute generate_plugin_context.
Após instalar novos plugins no Moodle, execute update_indexes.

## Plugins em desenvolvimento
- local_myplugin — descreva brevemente o propósito

## Convenções
- Padrão de código: Moodle Coding Style (PSR-12 + Frankenstyle)
- Todo acesso ao banco via $DB — nunca SQL direto
- Todo output via $OUTPUT ou renderers — nunca echo direto
- Capabilities sempre verificadas com require_capability() ou has_capability()
```

---

## 💡 Fluxos de Trabalho Recomendados

### Iniciando uma sessão

```bash
# Entrar no diretório do Moodle antes de iniciar o Codex
cd /home/usuario/workspace/www/html/moodle
codex
```

Iniciar a partir do diretório do Moodle garante que o `AGENTS.md` do projeto seja carregado e que o Codex entenda o contexto do workspace.

Na primeira sessão após configurar:

```
Inicialize o contexto do build82 para esta instalação Moodle.
```

### Carregando contexto de um plugin

```
Carregue o contexto do plugin local_myplugin e me dê um resumo
da arquitetura, banco de dados e funções principais.
```

### Buscando na API do core

```
Use a tool search_api para encontrar funções da API do core Moodle
relacionadas a enrollment que não estejam depreciadas.
```

### Criando um novo plugin

```
scaffold_plugin
  type="local"
  name="audit_log"
  description="Histórico de auditoria das ações dos usuários"
  features="database tables, scheduled tasks, capabilities, event observers"
```

### Revisão antes do commit

```
/review_plugin plugin="local/myplugin" focus="security"
```

---

## ⚠️ Troubleshooting

### Servidor não aparece após adicionar

O Codex lê o `config.toml` na inicialização. Após editar o arquivo, reinicie a sessão:

```bash
exit
codex
```

Na extensão VS Code, recarregue a janela: `Ctrl+Shift+P` → **Developer: Reload Window**.

### Erro de sintaxe TOML quebra CLI e VS Code simultaneamente

Esta é uma característica da configuração compartilhada. Se ambos pararem de funcionar após uma edição, o problema quase certamente é sintaxe TOML inválida. Verifique:

- Strings devem usar aspas duplas: `"valor"`, não `'valor'`
- Arrays usam colchetes: `args = []`
- O nome da seção deve ser exato: `[mcp_servers.build82]`

### SSE não é suportado

O `build82` roda via stdio por padrão, que é o que todas as opções acima registram. O modo `--http` do servidor (Streamable HTTP + SSE) é um modo separado e opcional, e não é configurado por nenhum passo desta página.

### Contexto desatualizado após mudanças

- **Mudanças em um plugin:** _"Regenere o contexto do plugin local_myplugin."_
- **Novo plugin instalado:** _"Regenere todos os índices globais do Moodle."_

---

## ➡️ Próximos Passos

- [Claude Desktop](./claude-desktop.md) — app desktop da Anthropic (macOS, Windows e Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — CLI da Anthropic, `build82 install claude`
- [Antigravity (IDE and CLI)](./antigravity.md) — IDE e agente de terminal do Google
- [OpenCode](./opencode.md) — agente open source com interface TUI
- [Cursor](./cursor.md) — editor Cursor
- [Zed](./zed.md) — editor Zed
- [Cline](./cline.md) — extensão Cline para VS Code e Cline CLI
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
