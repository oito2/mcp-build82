🌐 [English](../../../en/guides/clients/opencode.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com OpenCode

O **OpenCode** é um agente de desenvolvimento de código aberto que roda no terminal, com suporte nativo a servidores MCP. Sua interface TUI (Text User Interface) permite um fluxo de trabalho totalmente baseado em terminal, similar ao Claude Code.

---

## 🛠️ Instalação do OpenCode

### Verificar se já está instalado

Antes de instalar, verifique se o OpenCode já está disponível no seu sistema:

```bash
which opencode && opencode --version
```

Se um caminho e uma versão forem exibidos, o OpenCode já está instalado — pule para a seção de [Configuração do Servidor MCP](#️-configuração-do-servidor-mcp).

### Instalar via npm (recomendado)

```bash
npm install -g opencode-ai
```

Verifique a instalação:

```bash
opencode --version
```

### Instalar via script oficial

```bash
curl -fsSL https://opencode.ai/install | bash
```

---

## ⚙️ Configuração do Servidor MCP

### Configuração automática

```bash
build82 install opencode
```

O que ele faz, exatamente:

1. Verifica se OpenCode foi detectado (o comando `opencode` está no `PATH`); caso contrário imprime `Skipped: OpenCode not detected.` e não altera nada.
2. Pergunta a raiz do Moodle (sugere o diretório atual se parecer uma raiz do Moodle).
3. Mescla uma entrada `build82` em `~/.config/opencode/opencode.json` (ou o `~/.config/opencode/opencode.jsonc` existente, quando não há `opencode.json`) (objeto `mcp` no nível raiz), preservando as demais entradas, a ordem das chaves e cada valor como foi escrito (só a indentação é normalizada para 2 espaços), as permissões do arquivo e um link simbólico nesse caminho (o arquivo apontado é que é gravado). Uma entrada `build82` nova vai para o fim do objeto. Se o arquivo não existir, ele é criado.
4. Registra o caminho absoluto do binário `build82` em execução como comando (stdio) e define uma única variável de ambiente, `BUILD82_MOODLE_PATH`.

Se o arquivo tiver comentários ou vírgulas finais (JSONC), o build82 o lê, mas nunca o reescreve: se ele já tiver a mesma entrada `build82`, a ferramenta conta como instalada; caso contrário, imprime `OpenCode... manual step needed: ...` com o trecho `build82` exato para colar manualmente no objeto `mcp`, o arquivo permanece inalterado e o `install` sai com código 1. Um BOM UTF-8 é tolerado. Um arquivo que não seja JSON válido, ou cuja chave `mcp` não seja um objeto, é reportado como `failed` e fica inalterado.

Entrada resultante:

```json
{
  "mcp": {
    "build82": {
      "type": "local",
      "command": ["/usr/local/bin/build82"],
      "environment": { "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle" },
      "enabled": true
    }
  }
}
```

Como arquivos `opencode.jsonc` costumam ter comentários, o instalador se recusará a reescrevê-los e imprimirá o trecho para você colar. Arquivos de projeto (`opencode.json` na raiz do projeto) nunca são escritos.

### Configuração manual

O OpenCode aceita JSON ou JSONC e mescla configurações: primeiro a global, depois a do projeto (a do projeto prevalece).

| Escopo | Arquivo |
|--------|---------|
| Global | `~/.config/opencode/opencode.json` |
| Projeto | `opencode.json` na raiz do projeto |

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "build82": {
      "type": "local",
      "command": ["/usr/local/bin/build82"],
      "environment": {
        "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle"
      }
    }
  }
}
```

> **Nota:** `command` deve ser um **array** (não uma string) e a chave de variáveis é `environment`. Um campo `args` separado não é suportado; coloque argumentos extras no array `command`.

Use o caminho absoluto do `build82` como primeiro elemento de `command` se o OpenCode não herdar o `PATH` do seu shell (`which build82`).

### Verifique

```bash
opencode mcp list
```

O `build82` deve aparecer na lista. Você também pode iniciar o `opencode` no diretório do Moodle e conferir os servidores MCP ativos dentro da sessão.

### Remoção

```bash
build82 uninstall opencode
```

Isso apaga apenas a chave `build82` de `~/.config/opencode/opencode.json`, `~/.config/opencode/opencode.jsonc` e do `~/.config/opencode/config.json` legado. As outras entradas não são tocadas e um arquivo inexistente não gera erro. Imprime `OpenCode... removed.`, ou `OpenCode... not registered.` (código de saída 0) quando não havia nada a remover. Um arquivo com comentários ou vírgulas finais (JSONC) nunca é reescrito: o comando imprime `OpenCode... manual step needed: ...` pedindo que você remova a entrada manualmente, e sai com código 1. Um arquivo que não pode ser lido ou interpretado é reportado como `OpenCode... failed: ...`, nunca como não registrado. Execute `build82 uninstall` sem alvo para localizar e remover todos os registros do build82 de uma vez (pede confirmação).

Entradas adicionadas à mão em um `opencode.json` de projeto devem ser apagadas manualmente.

Referências oficiais: [MCP servers](https://opencode.ai/docs/mcp-servers/), [Config](https://opencode.ai/docs/config/).

---

## 📄 Turbinando com AGENTS.md

O OpenCode lê automaticamente o arquivo `AGENTS.md` para carregar o contexto do projeto a cada sessão.

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
# Entrar no diretório do Moodle antes de iniciar o OpenCode
cd /home/usuario/workspace/www/html/moodle
opencode
```

Iniciar a partir do diretório do Moodle garante que o `AGENTS.md` do projeto seja carregado.

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

### Servidor não aparece após configurar

O OpenCode lê o `opencode.json` na inicialização. Após editar o arquivo, reinicie a sessão.

Verifique também se o JSON está bem formado — um erro de sintaxe impede o carregamento da configuração inteira:

```bash
python3 -c "import json; json.load(open('opencode.json'))" && echo "JSON válido"
```

### `build82` não encontrado

Se o OpenCode não herdar o PATH do shell, use o caminho absoluto no campo `command` (veja a seção de configuração acima).

```bash
# Encontre o caminho correto
which build82
# → /usr/local/bin/build82
```

### Contexto desatualizado após mudanças

- **Mudanças em um plugin:** _"Regenere o contexto do plugin local_myplugin."_
- **Novo plugin instalado:** _"Regenere todos os índices globais do Moodle."_

---

## ➡️ Próximos Passos

- [Claude Desktop](./claude-desktop.md) — app desktop da Anthropic (macOS, Windows e Linux beta), `build82 install claude-desktop`
- [Claude Code](./claude-code.md) — CLI da Anthropic, `build82 install claude`
- [OpenAI Codex](./codex.md) — CLI da OpenAI com configuração TOML
- [Antigravity (IDE and CLI)](./antigravity.md) — IDE e agente de terminal do Google
- [Cursor](./cursor.md) — editor Cursor
- [Zed](./zed.md) — editor Zed
- [Cline](./cline.md) — extensão Cline para VS Code e Cline CLI
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
