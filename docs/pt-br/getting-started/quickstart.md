🌐 [English](../../en/getting-started/quickstart.md) | **Português** | 🏠 [Índice](../index.md)

---

# Início Rápido (Quickstart)

Este guia ajudará você a conectar o `build82` ao seu assistente de IA e realizar sua primeira tarefa de desenvolvimento em menos de 5 minutos.

---

## 1. Conecte seu Assistente

A forma mais rápida: deixe o build82 configurar o cliente para você.

```bash
build82 install
```

Ele detecta automaticamente todo cliente suportado instalado na sua máquina (Claude Code, Claude Desktop, Antigravity, OpenAI Codex, OpenCode, Cursor, Zed, Cline), lista os clientes, pergunta o caminho do seu Moodle, pede confirmação e escreve a configuração sozinho. Passe um target explicitamente (ex: `build82 install claude`) para configurar só um.

### Configuração manual

Se preferir configurar um cliente à mão — ou quiser ver exatamente o que o `install` escreve — aqui está o equivalente para o Claude Code (o `install claude` o registra no escopo user do Claude Code, ou seja, para todos os seus projetos):

```bash
claude mcp add --scope user build82 \
  -e BUILD82_MOODLE_PATH=/var/www/html/moodle \
  -- build82
```

Verifique se o servidor foi registrado:

```bash
claude mcp list
# o build82 deve aparecer na lista
```

> Para os outros clientes — incluindo caminhos exatos de arquivo de config e trechos JSON/TOML — veja os guias completos:
> [Claude Code](../guides/clients/claude-code.md) · [Claude Desktop](../guides/clients/claude-desktop.md) · [Antigravity](../guides/clients/antigravity.md) · [OpenAI Codex](../guides/clients/codex.md) · [OpenCode](../guides/clients/opencode.md) · [Cursor](../guides/clients/cursor.md) · [Zed](../guides/clients/zed.md) · [Cline](../guides/clients/cline.md)

---

## 2. Inicialize o Contexto

Com o servidor conectado, abra um chat com a IA e peça para inicializar o ambiente. Este passo mapeia a versão do Moodle e gera todos os 13 arquivos de índice globais.

> **Antes de continuar:** confirme que o caminho do Moodle aponta para a raiz correta — o diretório que contém `version.php`.

**Se você registrou o servidor com `build82 install`** (ou definiu `BUILD82_MOODLE_PATH` por conta própria), o servidor já conhece o caminho do Moodle, então o `init_moodle_context` apenas responderia "already initialized" e não geraria nada. Peça os índices diretamente — **digite no chat:**

```
Gere os índices do build82 para a minha instalação do Moodle.
```

A IA executará a tool `update_indexes`, que detecta a versão a partir do `version.php` e gera os índices globais, informando a versão do Moodle e quantos índices foram regenerados, reaproveitados do cache ou falharam.

**Se ainda não há caminho do Moodle configurado** (configuração manual sem `BUILD82_MOODLE_PATH`), passe-o no prompt:

```
Inicialize o contexto do build82 para a instalação em /var/www/html/moodle.
```

A IA executará a tool `init_moodle_context`, salvará o caminho em `~/.build82` e confirmará a versão encontrada (ex: Moodle 4.5) e os índices gerados.

Em ambos os casos, isso leva de alguns segundos a alguns minutos, dependendo do número de plugins instalados.

---

## 3. Sua Primeira Tarefa: Explorar a API

Agora que a IA tem "olhos" dentro do seu Moodle, peça para ela buscar algo na API oficial.

**Tente este prompt:**

```
Busque na API do core funções relacionadas a "enrollment" (matrícula) que não estejam depreciadas.
```

A IA usará a tool `search_api` e listará as funções com suas respectivas assinaturas e arquivos onde estão definidas.

---

## 4. Analisando um Plugin Existente

Se você já tem um plugin em desenvolvimento, peça para a IA entendê-lo profundamente.

**Tente este prompt:**

```
Gere o contexto de IA para o meu plugin local_caedauth e me dê um resumo das tabelas de banco de dados que ele usa.
```

O servidor criará os arquivos `PLUGIN_*.md` dentro de `.build82/`, no diretório do plugin, e a IA explicará a estrutura completa para você.

---

## 🎯 Próximos Passos

Agora que você está conectado, explore o potencial máximo do servidor:

- **Criar do zero:** Use o guia [Meu Primeiro Plugin](./first-plugin.md) para fazer o scaffold de um novo componente.
- **Corrigir bugs:** Peça para a IA analisar um erro usando a tool `doctor` ou o prompt `debug_plugin`.
- **Revisar código:** Antes de um commit, peça: _"Faça um code review do meu plugin focado em segurança e padrões Moodle"_.
- [Voltar ao Índice](../index.md)

---

> 💡 **Dica de Ouro:** Se a IA disser que não conhece o comando, tente ser explícito: _"Use a tool MCP `search_api` para encontrar..."_.
