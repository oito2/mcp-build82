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
├── cmd/build82/      ← main.go — dispatch da CLI: servidor stdio (padrão), install, self-update, uninstall
└── internal/
    ├── server/           ← montagem do servidor MCP (tools + resources + prompts)
    ├── tools/            ← 15 MCP tools
    ├── resources/        ← 13 globais + 1 agregado + 12 templates de resource por plugin
    ├── prompts/          ← 4 templates de MCP prompt
    ├── extractors/       ← lê e faz parse dos arquivos PHP/XML do Moodle (backend regex + tsbackend/ tree-sitter)
    ├── generators/       ← escreve arquivos .md de contexto sob .build82/ (globais + por plugin) e migra arquivos legados
    ├── cache/            ← cache mtime, persistido em .build82/.cache.json
    ├── watcher/          ← regeneração automática via fsnotify (com debounce, só plugins dev)
    ├── config/           ← carregador de config: variáveis de ambiente → ~/.build82
    ├── installer/        ← `install`/`uninstall` — configura/remove o registro no cliente MCP
    ├── selfupdate/       ← `self-update` — consulta GitHub Releases, verifica assinatura (cosign) e checksum, troca atômica do binário
    ├── moodletype/       ← mapa tipo de plugin ↔ diretório, resolução de path e checagens de containment
    ├── phpcs/            ← detecta o phpcs com o padrão moodle e o executa (relatório JSON) para o check_coding_style e o doctor
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

**Ponto de entrada.** `cmd/build82/main.go` faz o dispatch pelo primeiro argumento: sem argumentos (ou com uma flag desconhecida) inicia o servidor MCP via **stdio**; `install`, `self-update` e `uninstall` executam seus próprios fluxos pontuais e encerram. O servidor chama `server.NewServer()`, que registra as 15 tools, os resources (13 globais + 1 agregado + 12 templates por plugin) e os 4 prompts, e informa um ícone PNG 64×64 no `serverInfo` como data URI embutido. Todo handler é envolvido por um wrapper de recuperação de panic (`withRecover`, `withRecoverResource` e o equivalente de prompts), de modo que um panic causado por código-fonte de plugin malformado vira um resultado de erro em vez de derrubar o processo.

**Efeitos colaterais.** O `build82` só escreve: os arquivos `.md` gerados, o marcador `.indevelopment` e o `.cache.json` sob `.build82/` (raiz do Moodle e raízes de plugins), `~/.build82` (config), o arquivo `tags` (quando o `ctags` está disponível), o ZIP criado por `release_plugin`, o esqueleto criado por `create_plugin_skeleton`, as edições de `version.php` e `db/upgrade.php` feitas por `bump_plugin_version` e — via `install`/`uninstall`/`self-update` — arquivos de configuração de clientes MCP e o próprio binário. Os extractors nunca escrevem em disco.

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
   └─ toda regeneração por tool ou pelo watcher → notifications/resources/updated às sessões que assinaram os resources regravados

5. Cliente de IA usa Prompts
   └─ scaffold_plugin → injeta contexto Moodle + template few-shot
   └─ review_plugin → injeta contexto do plugin + checklist de revisão
   └─ debug_plugin → injeta contexto + dicas de debugging por palavra-chave
   └─ upgrade_plugin → injeta chamadas deprecated, callbacks legados e achados de upgrade + passos de migração
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

## Transporte

O build82 roda somente via **stdio**: o cliente de IA inicia o `build82` como subprocesso e troca mensagens MCP pela entrada e saída padrão dele. Nenhuma porta de rede é aberta. Isso combina com o uso como ferramenta local de desenvolvimento, ao lado do código do Moodle e do cliente de IA; com o Moodle em Docker, o build82 roda no host e lê os arquivos montados (veja [Docker](../guides/environments/docker.md)). O modo HTTP (`--http`) das versões anteriores foi removido na v1.2.0.

---

## Principais dependências

| Dependência | Por que é usada |
| --- | --- |
| `github.com/modelcontextprotocol/go-sdk` | SDK oficial de MCP para Go: tratamento do protocolo, o transporte stdio, registro de tools/resources/prompts |
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
