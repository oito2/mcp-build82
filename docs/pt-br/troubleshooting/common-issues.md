🌐 [English](../../en/troubleshooting/common-issues.md) | **Português** | 🏠 [Índice](../index.md)

---

# Solução de Problemas

Encontrou algum problema ao usar o `build82`? Esta página cobre os erros mais comuns com soluções diretas.

---

## 🔍 Diagnóstico inicial

Antes de investigar qualquer problema específico, execute estes dois passos — eles resolvem a maioria dos casos:

**Passo 1 — Verificar se o servidor está conectado:**

No chat do seu cliente de IA, execute:
```
/mcp
```

Se `build82` aparecer como conectado com 13 tools, o servidor está funcionando. O problema está na configuração do contexto, não na conexão.

Se não aparecer, o problema está na configuração do servidor — veja a seção de [Erros de Conexão](#-erros-de-conexão-e-path) abaixo.

**Passo 2 — Verificar a saúde do ambiente:**

```
Execute o doctor do build82.
```

A IA chamará a tool `doctor` e retornará um relatório com: localização do arquivo de config, caminho e versão do Moodle, atualidade dos índices, arquivos legados pendentes de migração, consistência entre plugins e estatísticas de cache. Qualquer problema de configuração aparecerá aqui.

---

## 🧭 Índice de Sintomas

| Sintoma | Seção |
| :--- | :--- |
| O cliente diz que não consegue conectar, ou o servidor nunca sai de "Connecting..." | [O cliente de IA não encontra o `build82`](#o-cliente-de-ia-não-encontra-o-build82), [Servidor travado em "Connecting..."](#servidor-travado-em-connecting) |
| `❌ build82 is not initialized. Run init_moodle_context first.` | [build82 não está inicializado](#build82-não-está-inicializado) |
| `❌ Invalid Moodle path: ...` | [Caminho do Moodle não encontrado](#caminho-do-moodle-não-encontrado) |
| `command not found: build82` | [command not found após compilar do código-fonte](#command-not-found-build82-após-compilar-do-código-fonte) |
| O macOS ou o Windows se recusa a executar o binário baixado | [O sistema operacional bloqueia o binário baixado](#o-sistema-operacional-bloqueia-o-binário-baixado) |
| Erros de permissão ao gerar arquivos `.md` | [Permissão negada ao criar arquivos `.md`](#permissão-negada-ao-criar-arquivos-md) |
| `build82 self-update` falha ou a nova versão está quebrada | [self-update falha](#build82-self-update-falha) |
| A IA usa campos ou funções desatualizados | [A IA sugere código de uma versão antiga do plugin](#a-ia-sugere-código-de-uma-versão-antiga-do-plugin) |
| Versão antiga do Moodle reportada após um upgrade | [Contexto desatualizado após um upgrade do Moodle](#contexto-desatualizado-após-um-upgrade-do-moodle) |
| O monitoramento automático parou | [O modo watch para após reiniciar o servidor](#o-modo-watch-para-após-reiniciar-o-servidor) |
| `.build82/` aparece no `git status` | [`.build82/` aparecendo no `git status`](#build82-aparecendo-no-git-status) |
| Diretório vazio ou inexistente quando o Moodle roda no Docker | [build82 no host não enxerga o Moodle no Docker](#build82-no-host-não-enxerga-o-moodle-no-docker) |
| HTTP `401 Unauthorized`, `403 Forbidden` ou erro de inicialização sobre o token | [Erros do transporte HTTP](#-erros-do-transporte-http) |
| `scaffold_plugin` não está na lista de tools | [`scaffold_plugin` não aparece como tool no `/mcp`](#scaffold_plugin-não-aparece-como-tool-no-mcp) |
| A IA não conhece o `build82` | [A IA diz que não conhece o build82](#a-ia-diz-que-não-conhece-o-build82) |

---

## 🚫 Erros de Conexão e PATH

### O cliente de IA não encontra o `build82`

**Sintoma:** o cliente de IA reporta que não consegue conectar ao servidor, ou o servidor aparece como "Connecting..." e nunca completa.

**Causa:** diferente de servidores lançados via `npx`, o `build82` é um binário único — o cliente só precisa do caminho absoluto quando ele não está no PATH que o cliente herda (IDEs e algumas CLIs nem sempre herdam o PATH do seu shell interativo).

**Solução:** encontre o caminho absoluto e use-o explicitamente na configuração do servidor.

```bash
# Encontre o caminho correto
which build82
# → /usr/local/bin/build82
```

**Claude Code (`~/.claude.json`):**
```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "env": {
        "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle"
      }
    }
  }
}
```

**Antigravity CLI (`~/.gemini/config/mcp_config.json`, global):**
```json
{
  "mcpServers": {
    "build82": {
      "command": "/usr/local/bin/build82",
      "args": [],
      "env": {
        "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle"
      }
    }
  }
}
```

**OpenAI Codex (`~/.codex/config.toml`):**
```toml
[mcp_servers.build82]
command = "/usr/local/bin/build82"
env     = { BUILD82_MOODLE_PATH = "/home/usuario/workspace/www/html/moodle" }
```

**OpenCode (`opencode.json` na raiz do Moodle):**
```json
{
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

> Correção mais fácil de todas: rode `build82 install [target]` de novo — ele mesmo escreve o caminho absoluto. Note que, para alguns clientes (Codex, Antigravity, OpenCode), o arquivo que o `install` escreve difere do que a documentação oficial do cliente lista; se o servidor ainda não aparecer, configure o cliente manualmente seguindo o seu [guia](../guides/clients/claude-code.md) (cada cliente tem sua própria página nessa pasta).

---

### Servidor travado em "Connecting..."

**Sintoma:** o servidor aparece mas nunca sai do estado "Connecting...". Nenhuma tool é listada.

**Em clientes CLI (Antigravity CLI, OpenCode):**

Se o servidor foi configurado mas não aparece, reinicie a sessão:

```bash
Ctrl+C
agy   # ou: opencode
```

Depois verifique com `/mcp` dentro da sessão.

---

### Caminho do Moodle não encontrado

**Sintoma:** o `init_moodle_context` retorna `❌ Invalid Moodle path: <motivo>`, onde o motivo é `directory does not exist`, `version.php not found — this doesn't look like a Moodle root`, `lib/ directory not found` ou `neither config.php nor config-dist.php found`. O `build82 install` reporta uma mensagem parecida, "does not look like a Moodle root".

**Causa:** `BUILD82_MOODLE_PATH` (ou o caminho passado ao `init_moodle_context`) aponta para um subdiretório, para um diretório que não existe, ou usa um caminho relativo.

**Solução:**
- O caminho deve apontar para a **raiz do Moodle** — o diretório que contém `version.php`, `lib/` e `config.php` (ou `config-dist.php`)
- Use um **caminho absoluto** (ex: `/home/usuario/workspace/www/html/moodle`), nunca relativo (ex: `./moodle`)

### build82 não está inicializado

**Sintoma:** uma tool retorna `❌ build82 is not initialized. Run `init_moodle_context` first.`, ou o `doctor` reporta a config como `not initialized`.

**Causa:** não existe o arquivo de config `~/.build82` e `BUILD82_MOODLE_PATH` não está definida no ambiente do servidor.

**Solução:** peça à IA para executar `init_moodle_context` com o caminho da raiz do Moodle, ou defina `BUILD82_MOODLE_PATH` na configuração do servidor no cliente (o `build82 install` faz isso por você). Veja a [Referência de Configuração](../reference/configuration.md).

---

## 🛠️ Erros de Build e Runtime

### `command not found: build82` após compilar do código-fonte

**Sintoma:** `go build -o build82 ./cmd/build82` funciona, mas rodar `build82` falha.

**Causa:** o binário foi escrito no diretório atual, que geralmente não está no `PATH`.

**Solução:**
```bash
sudo mv build82 /usr/local/bin/
# ou, se você usou `go install`:
export PATH="$(go env GOPATH)/bin:$PATH"
```

---

### O sistema operacional bloqueia o binário baixado

**Sintoma:** o macOS diz que o binário "não pode ser aberto porque o desenvolvedor não pode ser verificado", ou o SmartScreen do Windows exibe "O Windows protegeu o computador".

**Causa:** os binários de release não são notarizados (macOS) nem assinados digitalmente (Windows).

**Solução:** remova o atributo de quarentena (`xattr -d com.apple.quarantine /usr/local/bin/build82`) no macOS, ou use **Mais informações → Executar assim mesmo** / `Unblock-File` no Windows. Detalhes no [Guia de Instalação](../getting-started/installation.md).

---

### `build82 self-update` falha

**Sintoma:** o comando termina com um erro como `checksum verification failed, refusing to replace the running binary`, `release <tag> has no asset named <asset>` ou `smoke test failed`.

**Causa e solução:** a atualização é verificada contra o `checksums.txt` do release e passa por um smoke test antes de o binário em execução ser substituído; portanto, uma falha deixa seu binário atual intocado. Tente novamente mais tarde, ou baixe o asset manualmente e verifique-o como descrito no [Guia de Instalação](../getting-started/installation.md). Se uma nova versão foi instalada mas se comporta mal, rode `build82 self-update --rollback`; `no backup found at <caminho> — nothing to roll back` significa que não há arquivo `.bak` ao lado do binário.

---

### Permissão negada ao criar arquivos `.md`

**Sintoma:** o servidor conecta, mas ao gerar contexto retorna um erro de permissão. Os arquivos `.build82/PLUGIN_*.md` não são criados.

**Causa:** o usuário que roda o `build82` não tem permissão de escrita no diretório do Moodle ou dos plugins.

**Solução no Linux (instalação direta):**
```bash
sudo chown -R $USER:$USER /caminho/para/seu/moodle/local/
```

**Solução com Docker (cenário sidecar):**

Certifique-se de que o volume não está montado como `:ro` (somente leitura). O servidor precisa escrever os arquivos `.md` de contexto:

```yaml
volumes:
  - ./www/html/moodle:/var/www/moodle  # sem :ro
```

---

## 🔄 Problemas de Contexto e Cache

### A IA sugere código de uma versão antiga do plugin

**Sintoma:** você modificou `db/install.xml` ou um arquivo PHP, mas a IA ainda referencia a versão anterior dos campos ou funções.

**Causa:** o cache mtime pode não ter detectado a mudança, ou o contexto do plugin não foi regenerado após as alterações.

**Solução — para um plugin específico:**
```
Regenere o contexto do local_myplugin ignorando o cache.
```
(A IA chamará `plugin_batch mode="list" plugins=["local_myplugin"] force=true` — o próprio `generate_plugin_context` sempre respeita o cache e não tem parâmetro `force`.)

**Solução — para todos os índices globais:**
```
Regenere todos os índices do Moodle ignorando o cache.
```

A IA chamará `update_indexes` com `force=true`.

---

### Contexto desatualizado após um upgrade do Moodle

**Sintoma:** após atualizar o Moodle para uma nova versão, a IA ainda reporta a versão antiga e funções que foram depreciadas ou removidas.

**Solução:**
```
Reinicialize o contexto do build82 para a instalação Moodle.
```

A IA chamará `init_moodle_context`, detectará a nova versão e regenerará todos os 13 índices globais.

---

### O modo watch para após reiniciar o servidor

**Sintoma:** o monitoramento automático de plugins para de funcionar após reiniciar o VS Code, Claude Code ou o servidor MCP.

**Causa:** o modo watch é **in-memory** — não persiste entre reinicializações do servidor (diferente do próprio cache mtime, que é persistido em `.build82/.cache.json`).

**Solução:** reative o monitoramento após cada reinicialização:
```
Inicie o monitoramento do local_myplugin para alterações de arquivo.
```

---

### `.build82/` aparecendo no `git status`

**Sintoma:** `git status` mostra um novo diretório `.build82/` não rastreado na instalação Moodle ou em um plugin.

**Solução:** adicione uma linha ao `.gitignore` na raiz do Moodle:

```gitignore
# arquivos de contexto do build82
.build82/
```

Como todo arquivo gerado vive sob esse único diretório, essa é a lista de exclusão inteira.

---

## 🐳 Docker e Ambientes Virtuais

### build82 no host não enxerga o Moodle no Docker

**Sintoma:** o servidor reporta que a pasta está vazia ou não existe, mesmo com o Moodle rodando em containers.

**Causa:** `BUILD82_MOODLE_PATH` deve ser o caminho **no host** (sua máquina), não o caminho interno do container.

**Solução:** use o caminho do lado do host na configuração do seu cliente:

```json
"env": {
  "BUILD82_MOODLE_PATH": "/home/usuario/workspace/www/html/moodle"
}
```

O build82 rodando no host lê diretamente dos arquivos do volume montado no disco — nunca precisa acessar o container.

---

## 🌐 Erros do Transporte HTTP

Relevante apenas ao rodar `build82 --http` (veja [Docker](../guides/environments/docker.md) e a [Referência da CLI](../reference/cli.md)).

### `401 Unauthorized`

**Sintoma:** requisições a `/mcp` ou `/sse` retornam `Valid Bearer token required. Set Authorization: Bearer <token> header.`

**Solução:** envie `Authorization: Bearer <token>` com exatamente o token com o qual o servidor foi iniciado (`--token` ou `BUILD82_TOKEN`). O `/health` nunca exige token.

### `403 Forbidden`: Host não permitido

**Sintoma:** a resposta diz `Host "<nome>" is not allowed. Pass --allowed-host to permit it.`

**Causa:** o servidor só aceita `localhost`, `127.0.0.1`, `::1`, o valor de `--host` e valores passados com `--allowed-host`.

**Solução:** reinicie com `--allowed-host <nome>` para cada hostname ou IP que os clientes usam para alcançá-lo.

### A inicialização falha com erro de token

**Sintoma:** código de saída 1 com `--token was passed with an empty value; ...` ou `BUILD82_TOKEN is set but empty; ...`.

**Solução:** remova a variável por completo (ou passe um valor não vazio) para escolher entre nenhuma autenticação e um token de verdade. Veja a [Referência de Configuração](../reference/configuration.md#semântica-de-build82_token).

### Erro `listen on <host>:<port>`

**Sintoma:** o servidor termina com `Fatal error: listen on 127.0.0.1:3000: ...`.

**Solução:** a porta já está em uso ou não é permitida; escolha outra com `--port <n>`.

---

## ❓ Perguntas Comuns

### `scaffold_plugin` não aparece como tool no `/mcp`

**Isso é esperado.** `scaffold_plugin` é um **Prompt MCP**, não uma Tool. Não aparece na lista de tools do `/mcp` — é invocado diretamente no chat como um prompt ou slash command:

```
/scaffold_plugin type="local" name="mytools" description="..."
```

Para a lista completa de tools vs. prompts, veja a [Referência de Tools](../reference/tools.md) e a [Referência de Prompts](../reference/prompts.md).

---

### A IA diz que não conhece o build82

**Sintoma:** a IA responde que não tem acesso ao servidor ou não sabe o que é o `build82`.

**Solução:** seja mais explícito na sua instrução:

```
Use a tool get_plugin_info para carregar o contexto do local_myplugin.
```

```
Use a tool search_api para encontrar funções relacionadas a enrollment.
```

Nomear a tool explicitamente garante que a IA a use em vez de responder com conhecimento genérico.

---

## 📝 Reportando um Novo Bug

Se seu problema não estiver listado aqui:

1. Peça ao seu assistente: _"Execute o doctor do build82"_ e copie a saída completa
2. Anote qual cliente de IA você está usando (Claude Code, Antigravity CLI, OpenAI Codex, OpenCode) e seu sistema operacional/plataforma
3. Abra uma **Issue** no GitHub: [github.com/oito2/mcp-build82/issues](https://github.com/oito2/mcp-build82/issues)
4. Descreva os passos para reproduzir o erro e inclua a saída do `doctor`

---

> **Dica:** reiniciar a IDE ou o processo do cliente de IA resolve a maioria dos problemas de travamento do servidor MCP — especialmente após editar arquivos de configuração como `~/.claude.json`, `~/.gemini/antigravity/mcp_config.json`, `~/.codex/config.toml` ou `opencode.json`.

---

[🏠 Voltar ao Índice](../index.md)
