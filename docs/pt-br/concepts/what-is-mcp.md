🌐 [English](../../en/concepts/what-is-mcp.md) | **Português** | 🏠 [Índice](../index.md)

---

# O que é MCP?

**Model Context Protocol (MCP)** é um padrão aberto criado pela Anthropic que define como assistentes de IA se comunicam com ferramentas e fontes de dados externas. Pense nele como um adaptador universal: qualquer cliente de IA compatível com MCP pode se conectar a qualquer servidor MCP usando o mesmo protocolo.

---

## O problema que o MCP resolve

Sem o MCP, cada integração de IA é construída de forma customizada:

- O Cursor tem seu próprio sistema de plugins
- O VS Code tem sua própria API de extensões
- O Claude Desktop tem seu próprio formato de tool

Com o MCP, você constrói **um único servidor** e qualquer cliente compatível pode usá-lo.

---

## Como o MCP funciona

```
Cliente de IA (Claude Code, Gemini, Cursor...)
       │
       │  JSON-RPC via stdio ou HTTP
       │
  Servidor MCP (build82)
       │
       │  lê do disco, faz parse do PHP, escreve .md de contexto
       │
  Sua instalação Moodle
```

Um servidor MCP expõe três tipos de capacidades:

| Capacidade    | Descrição                           | Como a IA usa                                                                        |
| -------------- | -------------------------------------- | ----------------------------------------------------------------------------------------- |
| **Tools**     | Funções que a IA pode chamar          | Invocadas explicitamente (`init_moodle_context`, `search_api`, `get_plugin_info`...)      |
| **Resources** | URIs que expõem dados                 | Lidos passivamente — o cliente os busca como contexto sem ação explícita                  |
| **Prompts**   | Templates de prompt pré-construídos   | Invocados por nome (`scaffold_plugin`, `review_plugin`...), injetam contexto automaticamente |

---

## Transportes MCP

O MCP suporta dois mecanismos de transporte:

| Transporte                  | Como funciona                                       | Quando usar                                                             |
| ----------------------------- | ------------------------------------------------------ | ---------------------------------------------------------------------------- |
| **stdio**                    | O servidor roda como subprocesso do cliente de IA      | Uso local — Moodle na mesma máquina que seu editor/CLI (o padrão)            |
| **HTTP (Streamable HTTP)**   | O servidor roda como serviço HTTP independente         | Uso remoto — Moodle em servidor separado ou ambiente de produção isolado     |

Para a maioria dos desenvolvedores rodando Moodle localmente, o transporte **stdio** é o padrão (`build82` sem flags) e não requer nenhuma configuração adicional.

---

## Leitura adicional

- [Especificação do Model Context Protocol](https://modelcontextprotocol.io)
- [Por que build82?](./why-build82.md)
- [Como o build82 funciona](./how-build82-works.md)
- [Glossário](./glossary.md)

---

[🏠 Voltar ao Índice](../index.md)
