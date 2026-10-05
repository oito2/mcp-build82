# Documentação — build82 🎓

🌐 [English](../en/index.md) | **Português** | 🏠 [Voltar ao README](../../README.md)

---

O **build82** é um servidor MCP (Model Context Protocol) projetado para expor a estrutura interna do Moodle a assistentes de IA, permitindo um desenvolvimento de plugins mais rápido, seguro e padronizado. É distribuído como um único binário estático.

Use este índice para navegar por toda a documentação.

---

## 🚀 Primeiros Passos

Configure seu ambiente e coloque o servidor rodando em minutos.

- [Instalação](./getting-started/installation.md) — Requisitos do sistema e métodos de instalação (binário de release ou `go install`).
- [Início Rápido](./getting-started/quickstart.md) — Seu primeiro comando e como sincronizar a IA com o seu Moodle.
- [Criando seu Primeiro Plugin](./getting-started/first-plugin.md) — Usando o scaffold para começar um projeto do zero.
- [Desinstalação](./getting-started/uninstallation.md) — Removendo registros nos clientes, arquivos gerados, config e o binário.

---

## 🧠 Conceitos

Entenda o que é o MCP, por que este servidor existe e como ele funciona internamente.

- [O que é MCP?](./concepts/what-is-mcp.md) — Introdução ao Model Context Protocol.
- [Por que build82?](./concepts/why-build82.md) — O problema que o servidor resolve e quando usá-lo.
- [Como o servidor funciona](./concepts/how-build82-works.md) — O fluxo entre Extractors, Generators e sua IA.
- [Arquitetura](./concepts/architecture.md) — Visão geral dos componentes e fluxo de dados.
- [Glossário](./concepts/glossary.md) — Termos e conceitos usados nesta documentação.

---

## 📖 Guias

### Clientes MCP

Configure o servidor no seu assistente de IA preferido.

- [Claude Code](./guides/clients/claude-code.md) — Registro via CLI com escopos local, project e user.
- [Claude Desktop](./guides/clients/claude-desktop.md) — `build82 install claude-desktop` ou configuração do `claude_desktop_config.json`.
- [Antigravity](./guides/clients/antigravity.md) — IDE e CLI, via o `mcp_config.json` compartilhado.
- [OpenAI Codex](./guides/clients/codex.md) — `codex mcp add` ou `~/.codex/config.toml`.
- [OpenCode](./guides/clients/opencode.md) — Configuração do OpenCode.
- [Cursor](./guides/clients/cursor.md) — Configuração MCP do Cursor.
- [Zed](./guides/clients/zed.md) — Configuração de context servers do Zed.
- [Cline](./guides/clients/cline.md) — Configuração MCP do Cline (extensão do VS Code e CLI).

### Ambientes

- [Uso com Docker](./guides/environments/docker.md) — Rodando o build82 junto de um Moodle containerizado.

### Workflows

- [Exemplos de uso](./guides/workflows/examples.md) — Casos de uso reais e fluxos de desenvolvimento.

---

## 💬 Prompts de Exemplo

- [Prompts de Exemplo](./prompts.md) — Pedidos em linguagem natural para digitar ao seu agente de IA, por categoria, com os parâmetros de cada um e o resultado esperado.

---

## 🛠️ Referência Técnica

Consulte os recursos e comandos disponíveis no servidor.

- [Tools](./reference/tools.md) — Ações que a IA pode executar (ex: `get_plugin_info`, `search_api`, `explain_plugin`).
- [Resources](./reference/resources.md) — Contexto que a IA lê passivamente (ex: índices de API, plugins, banco de dados e eventos).
- [Prompts](./reference/prompts.md) — Templates de prompt pré-configurados para tarefas comuns.
- [Arquivos Gerados](./reference/generated-files.md) — Os arquivos `.md` criados sob `.build82/` na sua instalação Moodle.
- [CLI](./reference/cli.md) — Cada subcomando e flag, e os transportes stdio / HTTP.
- [Configuração](./reference/configuration.md) — O arquivo `~/.build82` e cada variável de ambiente `BUILD82_*`.

---

## 🔬 Arquitetura Interna

Para quem quer entender ou contribuir com o código do servidor.

- [Extractors](./architecture/extractors.md) — Como o servidor lê e analisa a instalação Moodle, incluindo a divisão entre os backends regex e tree-sitter.
- [Generators](./architecture/generators.md) — Como o conteúdo de contexto é gerado para a IA.
- [Sistema de Cache](./architecture/cache-system.md) — O cache mtime persistido e a estratégia de invalidação.

---

## 🆘 Solução de Problemas

- [Problemas Comuns](./troubleshooting/common-issues.md) — Erros de conexão, erros de permissão e cache desatualizado.

---

> 💡 **Dica:** Se você está usando o Claude Code ou outro assistente com suporte a MCP, tente perguntar diretamente: _"Quais tools o build82 oferece?"_ — a IA consultará o servidor em tempo real e responderá com a lista atualizada.
