🌐 [English](../../../en/guides/clients/claude-code.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Usando com Claude Code

O **Claude Code** é o CLI da Anthropic para desenvolvimento assistido por IA. É um dos clientes MCP mais eficientes para desenvolvimento Moodle, pois permite um fluxo de trabalho baseado inteiramente em terminal, com suporte nativo ao protocolo MCP via stdio.

---

## 🛠️ Configuração Inicial

### 1. Adicione o servidor

**Automática** — deixe o build82 configurar:

```bash
build82 install claude
```

Exige o comando `claude` no `PATH` (caso contrário imprime `Skipped: Claude Code not detected.` e não altera nada), pergunta a raiz do Moodle e então executa exatamente:

```bash
claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=<raiz-do-moodle> -- <caminho-absoluto-do-build82>
```

O servidor é registrado no escopo **user** do Claude Code: disponível em todos os seus projetos e guardado de forma privada em `~/.claude.json`. Use os comandos manuais abaixo para o escopo `local` ou `project`. Este alvo configura apenas o Claude Code; o app Claude Desktop tem seu próprio alvo (veja [Claude Desktop](./claude-desktop.md)).

Executar o comando de novo substitui o registro existente, atualizando também o caminho do binário e o `BUILD82_MOODLE_PATH`. Antes do `claude mcp add`, o instalador executa `claude mcp get build82` e remove o que ele informar:

| Registro encontrado | O que o instalador faz |
|---------------------|------------------------|
| nenhum | Adiciona o registro e imprime `Claude Code... configured.` |
| escopo `user` ou `local` | Remove-o com `claude mcp remove --scope <escopo> build82`, verifica de novo, adiciona o registro no escopo user e imprime `Claude Code... updated.` |
| escopo `project` | Deixa o `.mcp.json` inalterado (ele é compartilhado com o projeto), remove um registro `user`, se houver, adiciona o registro no escopo user e imprime um aviso com o comando manual `claude mcp remove --scope project build82` |

O `claude mcp get` informa apenas o registro que tem precedência (local, depois project, depois user), e os escopos `local` e `project` pertencem a um diretório; por isso, eles só são detectados para o diretório de onde você executa o instalador. Se o escopo na saída não puder ser reconhecido, o instalador não altera nada e imprime `Claude Code... failed: ...`.

**Manual** — escolha o escopo desejado (ajuste os caminhos para a sua máquina):

| Escopo | Disponível em | Compartilhado | Armazenado em |
|--------|---------------|---------------|---------------|
| `local` (padrão) | apenas o projeto atual | não | `~/.claude.json` |
| `project` | apenas o projeto atual | sim, via controle de versão | `.mcp.json` na raiz do projeto |
| `user` | todos os seus projetos | não | `~/.claude.json` |

```bash
# local (padrão): este projeto, privado
claude mcp add build82 \
  -e BUILD82_MOODLE_PATH=/home/usuario/workspace/www/html/moodle \
  -- /usr/local/bin/build82

# user: todos os projetos
claude mcp add --scope user build82 \
  -e BUILD82_MOODLE_PATH=/home/usuario/workspace/www/html/moodle \
  -- /usr/local/bin/build82

# project: compartilhado por meio de um .mcp.json versionado
claude mcp add --scope project build82 \
  -e BUILD82_MOODLE_PATH=/home/usuario/workspace/www/html/moodle \
  -- /usr/local/bin/build82
```

O escopo de projeto cria o `.mcp.json` na raiz do projeto:

```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": { "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle" }
    }
  }
}
```

O Claude Code pede a cada usuário que aprove um servidor de escopo de projeto na primeira vez. Evite versionar um `BUILD82_MOODLE_PATH` específico da sua máquina. Quando o mesmo nome de servidor existe em vários escopos, local prevalece sobre project, que prevalece sobre user.

### 2. Verifique a conexão

```bash
claude mcp list
```

O `build82` deve aparecer como conectado (`claude mcp get build82` mostra os detalhes). Dentro de uma sessão, execute `/mcp` para vê-lo com as 13 tools disponíveis. Se não aparecer, veja [Troubleshooting](#️-troubleshooting).

### 3. Remoção

```bash
build82 uninstall claude
```

Isso executa `claude mcp get build82`, lê a linha `Scope:` da saída e remove esse registro com `claude mcp remove --scope <escopo> build82`, repetindo até não restar nada nos escopos `user` e `local`. Imprime `Claude Code... removed.`, ou `Claude Code... not registered.` (código de saída 0) quando nada está registrado, e imprime `Skipped: Claude Code not detected.` se o `claude` não estiver no `PATH`.

Um registro no escopo `project` (`.mcp.json`, compartilhado com todos no projeto) nunca é modificado. O uninstall ainda remove o registro `user`, se houver, e imprime um aviso com o comando para você executar a partir do diretório do projeto:

```bash
claude mcp remove --scope project build82
```

Quando o registro `project` era o único, imprime `Claude Code... nothing removed.` seguido desse aviso e termina com código de saída 0. Os escopos `local` e `project` são por diretório: execute `build82 uninstall claude` a partir do diretório do projeto para encontrá-los.

### 4. Inicialize o contexto do Moodle

Na primeira sessão após a configuração, peça ao Claude:

```
Inicialize o contexto do build82 para a minha instalação do Moodle.
```

O Claude chamará `init_moodle_context`, detectará a versão do Moodle e gerará todos os índices globais. Esse passo só precisa ser feito uma vez por instalação.

Referência oficial: [documentação de MCP do Claude Code](https://code.claude.com/docs/en/mcp).

---

## 📄 Turbinando com CLAUDE.md

O Claude Code lê automaticamente o arquivo `CLAUDE.md` na raiz do projeto ao iniciar cada sessão — eliminando a necessidade de re-explicar o ambiente toda vez.

Crie o arquivo na raiz da sua instalação Moodle:

```bash
touch /home/usuario/workspace/www/html/moodle/CLAUDE.md
```

**Template recomendado:**

```markdown
# Contexto de Desenvolvimento Moodle

## Ambiente
- Versão do Moodle: 4.4 (ajuste conforme sua instalação)
- Caminho: /home/usuario/workspace/www/html/moodle
- Stack: Docker com Nginx + PHP-FPM + MariaDB

## build82
O servidor MCP build82 está conectado e os índices foram gerados.
Tools disponíveis: init_moodle_context, generate_plugin_context, plugin_batch,
update_indexes, watch_plugins, search_plugins, search_api, get_plugin_info,
list_dev_plugins, doctor, explain_plugin, release_plugin.

## Plugins em desenvolvimento
- local_myplugin — descreva brevemente o propósito

## Convenções
- Padrão de código: Moodle Coding Style (PSR-12 + Frankenstyle)
- Todo acesso ao banco via $DB — nunca SQL direto
- Todo output via $OUTPUT ou renderers — nunca echo direto
- Capabilities sempre verificadas com require_capability() ou has_capability()

## Fluxo de trabalho
1. Antes de trabalhar em um plugin, carregue o contexto com get_plugin_info.
2. Use search_api antes de sugerir funções do core — prefira APIs documentadas.
3. Após adicionar novos plugins à instalação, execute update_indexes.
4. Após mudanças significativas em um plugin, execute generate_plugin_context.
```

---

## 💡 Fluxos de Trabalho Recomendados

### Iniciando uma sessão de desenvolvimento

No início de cada sessão, carregue o contexto do plugin em que vai trabalhar:

```
Estou trabalhando no plugin local_myplugin. Carregue o contexto completo.
```

O Claude chamará `get_plugin_info` e passará a conhecer a arquitetura, banco de dados, funções, eventos e padrões de código do plugin.

### Pesquisa na API do core

Em vez de abrir o navegador para consultar a documentação oficial:

```
Quais funções da API do core devo usar para lidar com persistência
de notas (grades)? Prefira funções públicas e não depreciadas.
```

O Claude usará `search_api` e retornará as funções com assinaturas, arquivos fonte e indicação de versão (`@since`).

### Criação de novos plugins

Use o prompt `scaffold_plugin` para criar a estrutura completa de um plugin:

```
scaffold_plugin
  type="block"
  name="monitor_alunos"
  description="Exibe um resumo de atividade dos alunos para professores"
  features="capabilities, caching"
```

Após criar os arquivos, gere o contexto para que o Claude passe a conhecer o novo plugin:

```
Gere o contexto de IA para o plugin block_monitor_alunos e me explique
a estrutura de classes gerada.
```

### Debugging de erros

Cole o erro diretamente no chat:

```
Estou recebendo este erro no Moodle:

[COLE O ERRO AQUI]

Carregue o contexto do plugin local_myplugin e ajude a identificar
a causa raiz e a correção.
```

### Ativando watch mode durante o desenvolvimento

Para que o contexto se atualize automaticamente enquanto você codifica:

```
Inicie o monitoramento do plugin local_myplugin para alterações.
```

O Claude chamará `watch_plugins action="start"`. Qualquer arquivo `db/*.php` ou `version.php` salvo no plugin dispara uma atualização de contexto em background.

### Empacotando um plugin para distribuição

```
Empacote o plugin local_myplugin como um ZIP pronto para distribuir.
```

O Claude chamará `release_plugin`, excluindo do pacote os arquivos gerados pelo próprio build82.

---

## ⚠️ Troubleshooting

### Primeiro passo: verificar a conexão

Antes de qualquer outra investigação, execute `/mcp` dentro da sessão do Claude. Se `build82` não aparecer como conectado, o problema está na configuração do servidor, não no seu prompt.

### `build82: command not found`

O Claude Code nem sempre herda o PATH do seu shell. Encontre o caminho absoluto e forneça-o explicitamente:

```bash
# Encontre o caminho correto
which build82
# → /usr/local/bin/build82

# Re-adicione o servidor com o caminho absoluto
claude mcp remove build82
claude mcp add build82 \
  -e BUILD82_MOODLE_PATH=/home/usuario/workspace/www/html/moodle \
  -- /usr/local/bin/build82
```

### Erros de permissão

Se o Claude Code reclamar que não consegue executar o servidor, verifique se o binário tem permissão de execução:

```bash
ls -l $(which build82)
chmod +x $(which build82)
```

### Servidor conectado mas tools não respondem

Execute o diagnóstico pedindo ao Claude:

```
Execute o doctor do build82.
```

O Claude chamará a tool `doctor` e retornará um relatório com o status do servidor, versão do Moodle, atualidade dos índices e stats de cache.

### Contexto desatualizado após mudanças

- **Novo plugin instalado no Moodle:** _"Regenere todos os índices globais do Moodle."_ → `update_indexes`
- **Mudanças significativas em um plugin:** _"Regenere o contexto do plugin local_myplugin."_ → `generate_plugin_context`

---

## ➡️ Próximos Passos

- [Claude Desktop](./claude-desktop.md) — app desktop da Anthropic (macOS, Windows e Linux beta), `build82 install claude-desktop`
- [OpenAI Codex](./codex.md) — CLI da OpenAI com configuração TOML
- [Antigravity (IDE and CLI)](./antigravity.md) — IDE e agente de terminal do Google
- [OpenCode](./opencode.md) — agente open source com interface TUI
- [Cursor](./cursor.md) — editor Cursor
- [Zed](./zed.md) — editor Zed
- [Cline](./cline.md) — extensão Cline para VS Code e Cline CLI
- [Exemplos de workflows](../workflows/examples.md) — casos de uso reais e prompts prontos
- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Problemas Comuns](../../troubleshooting/common-issues.md) — troubleshooting detalhado
- [Voltar ao Índice](../../index.md)
