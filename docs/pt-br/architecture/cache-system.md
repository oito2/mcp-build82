🌐 [English](../../en/architecture/cache-system.md) | **Português** | 🏠 [Índice](../index.md)

---

# Sistema de Cache

O `build82` usa um sistema de cache baseado em **mtime** (data de modificação do arquivo) para evitar regenerar arquivos de contexto desnecessariamente. Sem cache, toda chamada a `update_indexes` ou `plugin_batch` reprocessaria todo arquivo PHP e XML da instalação — independente de ter mudado ou não.

O cache do build82 é **persistido em disco**, em `{raiz_moodle}/.build82/.cache.json`, para que reiniciar o servidor (um evento real para uma ferramenta CLI que os clientes MCP iniciam do zero a cada sessão) não force uma nova varredura completa.

---

## Como funciona

O cache compara a data de modificação (`mtime`) de cada arquivo-fonte PHP/XML com a do arquivo `.md` de contexto correspondente:

```
Arquivo PHP/XML (fonte)         Arquivo .md (saída)
│                                 │
│  mtime: 2024-04-20 10:00        │  mtime: 2024-04-21 09:00
│                                 │
└───────────── comparação ────────┘
              │
    fonte mais antiga que a saída?
     │                  │
    SIM                 NÃO
     │                  │
    PULA              REGENERA
 (Skipped: true)      (escreve .md)
```

**Três resultados possíveis por arquivo, contados como totais acumulados do processo:**

| Resultado | Condição | Contado como |
| --- | --- | --- |
| **Hit** | A saída foi marcada como fresca nesta sessão/persistida, o conjunto de fontes é o mesmo, e nenhuma fonte mudou desde então | `Hits` |
| **Skip** | A saída existe e toda fonte é mais antiga que ela (comparação de mtime, sem marca prévia) | `Skips` |
| **Miss** | A saída foi invalidada explicitamente, não existe, seu conjunto de fontes mudou, ou alguma fonte é mais nova | `Misses` |

---

## Implementação

O cache é implementado como `MtimeCache` em `internal/cache/cache.go`. Uma única instância compartilhada e protegida por mutex — `cache.Global` — é usada por todo orquestrador de generator no processo.

```go
type MtimeCache struct {
	mu    sync.Mutex
	roots map[string]*rootMarks // raiz Moodle vinculada -> suas marcas ("" = não persistidas)
	stats CacheStats            // só em memória, sempre reseta ao reiniciar

	// Saídas invalidadas explicitamente (force: true, eventos do watcher): reportadas como
	// desatualizadas incondicionalmente até serem regeneradas e marcadas de novo. Só em
	// memória, indexado pelo caminho absoluto da saída, e não é tocado pelo EnsureLoaded.
	forced map[string]struct{}

	loadedRoot string // moodlePath ao qual este cache está vinculado atualmente
}

type mark struct {
	At  time.Time // quando as fontes foram lidas para a geração
	Set string    // SHA-256 dos caminhos das fontes ordenados ("" para Mark sem conjunto)
}
```

As marcas são guardadas **por raiz Moodle**: cada raiz vinculada com `EnsureLoaded` mantém suas próprias marcas, e uma marca vai para a raiz vinculada que contém o caminho da saída. Assim, uma execução ainda em andamento numa raiz nunca grava suas marcas no arquivo de cache de outra raiz quando uma segunda sessão troca o cache para outra instalação.

Em disco o arquivo é um JSON com um campo `version` (atualmente `2`) e um mapa `entries` de caminho de saída → `{"at": <horário>, "set": <hash>}`. Um arquivo da versão 1 é ignorado, então a primeira execução após a atualização regenera todas as saídas uma vez.

Os generators chamam `IsStale` via `genutil.RunCached`. Após uma geração bem-sucedida, o `RunCached` chama `MarkAt(output, readAt, sources)`: o `readAt` é tomado pelo orquestrador **antes** de listar e ler as fontes, então uma fonte salva enquanto os generators rodam é mais nova que a marca e a saída é regenerada na próxima execução. O `genutil.Write` só grava o arquivo.

### Lógica do `IsStale()`

A decisão de regeneração segue exatamente esta sequência:

```
0. A saída foi invalidada explicitamente (Invalidate / InvalidateAll) e não
   foi regenerada desde então?
   SIM → miss (regenera), independentemente dos mtimes
   NÃO → continua

1. O arquivo .md de saída existe como arquivo regular (não um link simbólico plantado no lugar)?
   NÃO → miss (regenera)
   SIM → continua

2. A saída foi marcada como fresca (nesta sessão, ou carregada do
   .cache.json persistido) com um conjunto de fontes diferente do atual
   (uma fonte adicionada, removida ou renomeada, qualquer que seja o mtime)?
   SIM → apaga a marca, miss (regenera)
   NÃO → continua

3. A saída foi marcada como fresca?
   SIM → compara o mtime de cada arquivo-fonte contra o timestamp da marca.
         Nada mudou desde a marca → hit (pula).
         Algo mudou → apaga a marca, miss (regenera).
   NÃO → continua para o passo 4

4. Algum arquivo-fonte é mais novo que o mtime da própria saída?
   SIM → miss (regenera)
   NÃO → skip (pula)
```

Sem marca (passo 4) só os mtimes são comparados, então uma mudança no conjunto de fontes não é detectada; toda saída gerada via `RunCached` tem marca.

### Arquivos-fonte monitorados

**Para arquivos globais** (`cache.GetMoodleSourceFiles`, usado pelas saídas de API/dev-rules/plugin-guide/ctags; as demais saídas globais são controladas por globs de `db/events.php`, `db/tasks.php`, `db/services.php`, `db/install.xml`, `db/access.php`, pelo `version.php` de cada plugin ou — no `MOODLE_CLASSES_INDEX.md` — por esses três arquivos mais todo arquivo `.php` dentro de um diretório `classes/`):

```
{moodlePath}/version.php
{moodlePath}/lib/moodlelib.php
{moodlePath}/lib/accesslib.php
```

**Para arquivos de plugin:** todo arquivo regular do diretório do plugin, ignorando links simbólicos e os diretórios `vendor/`, `node_modules/`, `.git/` e `.build82/` em qualquer profundidade. As saídas do plugin leem classes, módulos AMD e todo arquivo PHP, então cada uma depende do plugin inteiro.

O file watcher (`watch_plugins`) observa o mesmo conjunto: todo diretório de cada plugin dev, exceto os ignorados acima (`generators.IsSkippedPluginDir`), então uma mudança em qualquer arquivo-fonte regenera o plugin.

Um arquivo-fonte que não existe é simplesmente ignorado na comparação — nunca causa um erro.

---

## Persistência: `EnsureLoaded` / `Save`

- **`EnsureLoaded(moodlePath)`** vincula o cache a uma raiz Moodle e carrega o `.build82/.cache.json`, mas só na primeira vez que essa raiz é vinculada no processo — um no-op enquanto ela continua vinculada. Raízes vinculadas antes mantêm suas marcas em memória. Vincular a outra raiz primeiro salva as marcas ainda não salvas da anterior (ou as descarta quando essa raiz não existe mais) e zera as estatísticas em memória. Um arquivo de cache ausente, corrompido ou com versão incompatível degrada para um cache vazio; isso nunca é um erro fatal.
- **`Save()`** persiste as marcas de toda raiz vinculada via escrita atômica (`fsutil.WriteAtomic` — escreve num arquivo temporário, grava em disco, renomeia e sincroniza o diretório; um `.build82` que seja link simbólico é recusado), mas só se algo de fato mudou desde a última carga ou salvamento (`dirty`). Uma queda no meio da escrita nunca pode deixar um arquivo de cache pela metade.

**Operações em lote carregam e salvam uma vez por lote, não uma vez por plugin.** O `plugin_batch` e o caminho `include_plugins` do `update_indexes` chamam `EnsureLoaded` antes do loop e `Save` uma vez depois dele, delegando para `GenerateAllForPluginCore` (a metade sem o bracket de cache do `GenerateAllForPlugin`) dentro do próprio loop. Chamar o `Save()` completo — que re-serializa o mapa *inteiro* de arquivos marcados — uma vez por plugin num lote grande, do contrário, serializaria o I/O através de um pool de workers `parallel` e anularia boa parte do seu benefício.

---

## Invalidação de cache

### Invalidação forçada

`Invalidate(outputFile)` descarta a marca do arquivo **e** o adiciona ao conjunto `forced`, em memória. Enquanto uma saída está nesse conjunto, o `IsStale` a reporta como desatualizada (passo 0 acima) independentemente de qualquer comparação de mtime, de modo que a próxima execução do generator a reescreve. O `MarkAt`, chamado após a regeneração gravar o arquivo, a remove do conjunto. O conjunto nunca é persistido e o `EnsureLoaded` não o limpa, então uma invalidação feita antes de o cache ser vinculado (ou revinculado) a uma raiz Moodle continua valendo depois que ele carrega. O `InvalidateAll()` faz o mesmo para toda saída atualmente registrada em qualquer raiz vinculada (chame `EnsureLoaded` antes).

### Por `force=true`

- O `plugin_batch force=true` chama `Invalidate` para cada um dos 12 arquivos de saída de cada plugin antes de gerá-lo.
- O `update_indexes force=true` (`forceGlobalRegeneration` em `internal/tools/update.go`) chama `EnsureLoaded` e depois `Invalidate` para os 13 arquivos Markdown globais e o `.build82/tags`, antes de regenerar os arquivos globais. Com `include_plugins`, também invalida os 12 arquivos de saída de cada plugin dev.

O resultado é uma regeneração real: as saídas forçadas são reescritas mesmo quando todo arquivo-fonte é mais antigo que a saída.

### Por arquivo individual

`cache.Global.Invalidate(outputFile)` em um único caminho — usado pelo watcher de arquivos (veja abaixo) e pelo `force` na lista de arquivos de um plugin específico.

---

## Estatísticas do cache

`cache.Global.Stats()` acumula contagens de hit/miss/skip durante a vida do processo (só em memória — essa parte não persiste, diferente das próprias marcas). A tool `doctor` reporta isso:

```
## Cache

  Hits: 42, Misses: 7, Skips: 18
  Cache file: /var/www/moodle/.build82/.cache.json (3421 bytes)
```

**Como interpretar:**

- **Muitos hits:** o modo watch está ativo, ou a mesma tool foi chamada repetidamente nesta sessão
- **Muitos skips:** a instalação está estável — poucos arquivos PHP mudaram desde que o cache persistido foi escrito pela última vez
- **Muitos misses:** primeira execução contra esta instalação, ou muitos arquivos mudaram (ex: um upgrade do Moodle)

---

## Interação com o modo watch

O `internal/watcher` usa `fsnotify` para monitorar os arquivos-fonte de plugin listados acima (os que existem) em todo plugin marcado `.indevelopment`, até 20 plugins.

O watcher registra no `fsnotify` os diretórios pais desses arquivos e filtra os eventos para os arquivos monitorados, de modo que editores que salvam via arquivo temporário e renomeação continuam sendo observados. Uma mudança que chega enquanto o mesmo plugin está sendo regenerado dispara mais uma regeneração depois.

Quando um arquivo correspondente é salvo, criado, renomeado ou removido:

1. O timer de debounce de 500 ms do watcher dispara depois que a mudança se assenta
2. Ele chama `cache.Global.Invalidate(...)` para cada um dos 12 arquivos de saída do plugin, o que força a regeneração independentemente dos mtimes
3. Ele roda `GenerateAllForPlugin` para aquele plugin e depois atualiza o `MOODLE_AI_INDEX.md`

O modo watch em si é só em memória e não sobrevive a uma reinicialização do servidor — precisa ser iniciado de novo com `watch_plugins action="start"` em cada sessão.

---

## Veja também

- [Extractors](./extractors.md) — os pacotes cujos resultados o cache evita recalcular
- [Generators](./generators.md) — onde o `IsStale()` e a marcação pós-escrita acontecem
- [Referência de Tools](../reference/tools.md) — `update_indexes`/`plugin_batch` (`force`) e `doctor` (stats)

---

[🏠 Voltar ao Índice](../index.md)
