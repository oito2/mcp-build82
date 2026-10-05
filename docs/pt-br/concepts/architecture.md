🌐 [English](../../en/concepts/architecture.md) | **Português** | 🏠 [Índice](../index.md)

---

# Arquitetura

Visão geral dos componentes do `build82`, do fluxo de dados e dos modos de operação.

---

## Estrutura do projeto

```
mcp-build82/
├── go.mod, go.sum        ← definição do módulo e dependências
├── CONTRIBUTING.md        ← processo de release, cross-compilação, checksums
├── docs/                 ← documentação completa (en/ e pt-br/)
├── scripts/release/      ← cross-compila cada binário de release + build82.mcpb (bundle MCPB) + checksums.txt + server.json (manifesto do MCP Registry) em dist/
├── testdata/moodle/      ← árvore Moodle fixture usada pelos testes de integração
├── cmd/build82/      ← main.go — dispatch da CLI: stdio (padrão), --http, install, self-update, uninstall
└── internal/
    ├── server/           ← montagem do servidor MCP (tools + resources + prompts)
    ├── transport/        ← transporte Streamable HTTP + SSE
    ├── tools/            ← 13 MCP tools
    ├── resources/        ← 13 globais + 1 agregado + 12 templates de resource por plugin
    ├── prompts/          ← 3 templates de MCP prompt
    ├── extractors/       ← lê e faz parse dos arquivos PHP/XML do Moodle (backend regex + tsbackend/ tree-sitter)
    ├── generators/       ← escreve arquivos .md de contexto sob .build82/ (globais + por plugin) e migra arquivos legados
    ├── cache/            ← cache mtime, persistido em .build82/.cache.json
    ├── watcher/          ← regeneração automática via fsnotify (com debounce, só plugins dev)
    ├── config/           ← carregador de config: variáveis de ambiente → ~/.build82
    ├── installer/        ← `install`/`uninstall` — configura/remove o registro no cliente MCP
    ├── selfupdate/       ← `self-update` — consulta GitHub Releases, verifica checksum, troca atômica do binário
    ├── moodletype/       ← mapa tipo de plugin ↔ diretório, resolução de path e checagens de containment
    ├── phparray/         ← helpers compartilhados de parse de array/string literal PHP
    ├── phpdoc/           ← parse compartilhado de PHPDoc (visibilidade, @deprecated, @since...)
    ├── phptypes/         ← tipos de dados compartilhados pelos dois backends de extractors (só definições de tipo, sem lógica)
    ├── legacyhooks/      ← mapa callback legado → substituto na Hook API
    ├── genutil/          ← helpers compartilhados dos generators: escrita + marca no cache, cabeçalho, timestamp, barreira de erro Safely
    ├── binpath/, fsutil/, toolutil/, version/  ← utilitários compartilhados pequenos (caminho do binário, I/O atômico de arquivo, resultados de tool, string de versão)
    └── */                ← uma suíte `_test.go` por pacote (go test -race ./...)
```

---

## Fluxo de dados

**Ponto de entrada.** `cmd/build82/main.go` faz o dispatch pelo primeiro argumento: sem argumentos (ou com uma flag desconhecida) inicia o servidor MCP via **stdio**; `--http` inicia via Streamable HTTP + SSE; `install`, `self-update` e `uninstall` executam seus próprios fluxos pontuais e encerram. Os dois modos de servidor chamam `server.NewServer()`, que registra as 13 tools, os resources (13 globais + 1 agregado + 12 templates por plugin) e os 3 prompts, e informa um ícone PNG 64×64 no `serverInfo` como data URI embutido. Todo handler é envolvido por um wrapper de recuperação de panic (`withRecover`, `withRecoverResource` e o equivalente de prompts), de modo que um panic causado por código-fonte de plugin malformado vira um resultado de erro em vez de derrubar o processo.

**Efeitos colaterais.** O `build82` só escreve: os arquivos `.md` gerados, o marcador `.indevelopment` e o `.cache.json` sob `.build82/` (raiz do Moodle e raízes de plugins), `~/.build82` (config), o arquivo `tags` (quando o `ctags` está disponível), o ZIP criado por `release_plugin`, o esqueleto criado por `create_plugin_skeleton` e — via `install`/`uninstall`/`self-update` — arquivos de configuração de clientes MCP e o próprio binário. Os extractors nunca escrevem em disco.

```
1. Assistente de IA chama init_moodle_context
   └─ extractors/moodledetect.go detecta a versão do Moodle via version.php
   └─ config/config.go salva o caminho e a versão em ~/.build82

2. Generators chamam Extractors
   └─ cada extractor lê um tipo de arquivo PHP do Moodle
   └─ generator escreve .md sob .build82/ (raiz ou diretório do plugin)
   └─ cache/cache.go verifica mtime — pula arquivos inalterados
   └─ genutil.Write salva o arquivo e o marca como atual; o cache é persistido em .build82/.cache.json

3. Cliente de IA lê Resources (passivamente)
   └─ moodle://plugin/local_myplugin → PLUGIN_AI_CONTEXT.md
   └─ moodle://api-index → MOODLE_API_INDEX.md

4. Cliente de IA chama Tools (explicitamente)
   └─ get_plugin_info → retorna o PLUGIN_AI_CONTEXT.md do plugin (ou metadados ao vivo se ele nunca foi gerado)
   └─ search_api → pesquisa no MOODLE_API_INDEX.md
   └─ watch_plugins → ativa um watcher fsnotify nos diretórios de plugins dev (plugin dev = tem um marcador .build82/.indevelopment)

5. Cliente de IA usa Prompts
   └─ scaffold_plugin → injeta contexto Moodle + template few-shot
   └─ review_plugin → injeta contexto do plugin + checklist de revisão
   └─ debug_plugin → injeta contexto + dicas de debugging por palavra-chave
```

---

## Resolução de configuração

O servidor determina o caminho do Moodle seguindo esta ordem de prioridade:

```
Variável de ambiente BUILD82_MOODLE_PATH
         │ (maior prioridade)
         ▼
Arquivo ~/.build82 (escrito pelo init_moodle_context)
```

O próprio parâmetro `moodle_path` do `init_moodle_context` é o que *escreve* o arquivo `~/.build82` pela primeira vez — toda outra tool então lê qualquer uma das duas fontes acima que resolver.

> **Nota sobre `BUILD82_MOODLE_VERSION`:** as duas fontes nunca são mescladas. Quando `BUILD82_MOODLE_PATH` está definida (como faz o `build82 install`), a versão vem apenas de `BUILD82_MOODLE_VERSION` e `BUILD82_MOODLE_FULLVERSION` (o `$version` numérico do `version.php`, ex: `2024100700`), que ficam vazias quando não definidas; o `~/.build82` não é lido. `init_moodle_context` e `update_indexes` continuam detectando a versão a partir do `version.php` para os índices que geram. Veja detalhes em [Instalação](../getting-started/installation.md).

---

## Modos de transporte

| Modo      | Quando usar                                                  | Como iniciar                                                           |
| --------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------- |
| **stdio** | Moodle local — mesma máquina que seu editor/CLI (o padrão)       | `build82`                                                              |
| **HTTP**  | Moodle remoto — servidor separado ou ambiente isolado             | `build82 --http --port 3000 --host 0.0.0.0 --token seu-token`          |

No modo HTTP o servidor expõe `/mcp` (Streamable HTTP) e `/sse` (SSE), ambos protegidos por token Bearer quando um token é definido, além de um `/health` sem autenticação. Todas as rotas exceto `/health` passam pela validação do header Host.

Flags disponíveis no modo HTTP:

| Flag             | Padrão      | Descrição                                                            |
| ----------------- | ------------- | -------------------------------------------------------------------------- |
| `--port`         | `3000`      | Porta TCP                                                                  |
| `--host`         | `127.0.0.1` | Endereço de bind (`0.0.0.0` para todas as interfaces)                      |
| `--token`        | —           | Token Bearer para `/mcp` e `/sse` (alternativa: variável `BUILD82_TOKEN`). Não é obrigatório, mas um aviso é registrado quando ausente, e é fortemente recomendado em hosts não-loopback |
| `--allowed-host` | —           | Hostname extra permitido pela validação do header Host (repetível)         |

> Para uso com Docker, veja o guia [Docker](../guides/environments/docker.md).

---

## Principais dependências

| Dependência | Por que é usada |
| --- | --- |
| `github.com/modelcontextprotocol/go-sdk` | SDK oficial de MCP para Go: tratamento do protocolo, transportes stdio e Streamable HTTP/SSE, registro de tools/resources/prompts |
| `github.com/odvcencio/gotreesitter` | Runtime tree-sitter em Go puro (sem cgo) com a gramática PHP, usado apenas pelo backend opcional `tsbackend` dos extractors |
| `github.com/fsnotify/fsnotify` | Notificações do sistema de arquivos para o `watch_plugins` (regeneração automática ao salvar) |

Todo o resto (`net/http`, `encoding/xml`, `regexp`, `sync`, ...) vem da biblioteca padrão do Go; as demais entradas do `go.mod` são dependências indiretas do SDK.

---

## Decisões de design

- **Extractors são puros, generators controlam a escrita.** Extractors recebem um caminho e devolvem dados estruturados, sem dependência de MCP e sem escrever em disco, o que os mantém testáveis isoladamente e reutilizáveis por qualquer generator.
- **Dois backends de extractors sob um mesmo contrato.** O backend regex é o padrão (rápido, pouca memória); o tree-sitter (`BUILD82_EXTRACTOR_BACKEND=treesitter`) é opcional porque é bem mais lento e pesado, em troca de correção em casos-limite. Os tipos de dados compartilhados ficam em `internal/phptypes`, então ambos os backends devolvem os mesmos tipos Go.
- **Toda a saída fica sob `.build82/`.** Arquivos gerados nunca se misturam ao código do plugin; `MigrateLegacyFiles` move arquivos deixados por versões antigas para `.build82/` a cada passada.
- **Cache mtime persistido em disco.** Um servidor de CLI é reiniciado com frequência, então as marcas sobrevivem a reinícios em `.build82/.cache.json` e arquivos inalterados não são reprocessados.
- **Concorrência com um único cache compartilhado protegido por mutex.** Generators independentes (e extractors de plugin) rodam em paralelo; as tools em lote carregam e salvam o cache uma vez por lote.
- **Isolamento de falhas.** `genutil.Safely` isola cada generator e os wrappers de recuperação de panic isolam cada handler de tool, resource e prompt, então um plugin malformado nunca derruba o servidor.
- **Binário estático único.** Nenhum runtime a instalar; `install` o registra em 8 ferramentas de IA e `self-update` o substitui atomicamente após verificar um checksum.
- **Seguro por padrão via HTTP.** Bind em loopback por padrão, validação do header Host, comparação de token Bearer em tempo constante, e um token vazio é erro de configuração em vez de desabilitar a autenticação silenciosamente.

---

## Veja também

- [Como o build82 funciona](./how-build82-works.md) — pipeline completo de Extractors e Generators
- [Extractors](../architecture/extractors.md) — como os arquivos PHP são lidos e interpretados
- [Generators](../architecture/generators.md) — como os arquivos `.md` de contexto são gerados
- [Sistema de Cache](../architecture/cache-system.md) — estratégia de cache e invalidação
- [Glossário](./glossary.md) — termos do projeto (plugin dev, `.indevelopment`, backend, ...)

---

[🏠 Voltar ao Índice](../index.md)
