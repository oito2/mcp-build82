🌐 [English](../../en/architecture/generators.md) | **Português** | 🏠 [Índice](../index.md)

---

# Generators

Os **Generators** recebem os dados estruturados produzidos pelos [Extractors](./extractors.md) e os transformam em arquivos Markdown de contexto escritos sob `.build82/` na instalação Moodle. São eles que tornam o conteúdo disponível para o servidor MCP servir via Resources e Tools.

---

## Visão geral

```
internal/generators/
├── moodle.go     ← 13 generators globais (raiz do Moodle)
├── plugin.go     ← 12 generators por plugin (diretório do plugin)
├── migration.go  ← migração de arquivos legados soltos → .build82/, roda primeiro em toda passada
└── common.go     ← helpers compartilhados de varredura de arquivos (ignoram vendor/node_modules/.git), descoberta de plugins dev/diretórios de plugin, acessores nil-safe, cache de info de plugin
```

Todo generator segue o mesmo contrato:

- Recebe um caminho (`moodlePath` ou `extractors.PluginInfo`) como entrada
- Chama o(s) extractor(es) que precisa
- Constrói o conteúdo Markdown em memória (via `strings.Builder`)
- Verifica o [cache mtime](./cache-system.md) antes de escrever — se o arquivo `.md` já existe e é mais novo que as fontes PHP/XML rastreadas, a escrita é pulada (`Skipped: true`)
- Escreve o arquivo via `genutil.Write` (cria o diretório, `os.WriteFile` simples), que também marca o cache como fresco em caso de sucesso
- Retorna um `GeneratorResult{ File, Success, Skipped, Error }`
- Cada corpo de generator é envolto em uma **barreira de erro** via `genutil.Safely` — um panic ou erro em um generator nunca interrompe os outros

---

## Generators globais (`moodle.go`)

Geram arquivos sob `{raiz_moodle}/.build82/`. Chamados por `init_moodle_context` e `update_indexes`.

O ponto de entrada é `GenerateAll(moodlePath, moodleVersion)`, que migra qualquer arquivo legado solto primeiro, depois roda os 12 primeiros generators abaixo concorrentemente (cada um é independente — arquivos de saída diferentes, sem estado mutável compartilhado além do cache mtime, que por sua vez é protegido por mutex). O `GenerateAiIndex` roda depois, porque lista os arquivos que a primeira onda acabou de escrever, e o `GenerateCtags` roda por último. Cada generator é envolvido por `genutil.RunCached`, que faz a checagem `IsStale` e registra um resultado `Skipped: true` sem chamar o generator quando nada mudou.

| Função | Arquivo gerado | Extractors usados |
| --- | --- | --- |
| `GenerateAiContext` | `AI_CONTEXT.md` | — (conteúdo estático + a contagem de diretórios de plugin; versão informada pelo chamador) |
| `GenerateApiIndex` | `MOODLE_API_INDEX.md` | `api` |
| `GenerateEventsIndex` | `MOODLE_EVENTS_INDEX.md` | `events` |
| `GenerateTasksIndex` | `MOODLE_TASKS_INDEX.md` | `tasks` |
| `GenerateServicesIndex` | `MOODLE_SERVICES_INDEX.md` | `services` |
| `GenerateDbTablesIndex` | `MOODLE_DB_TABLES_INDEX.md` | `schema` |
| `GenerateClassesIndex` | `MOODLE_CLASSES_INDEX.md` | `classes` (glob restrito `**/classes/**/*.php`, resolvido antes do parsing — não uma varredura da instalação inteira) |
| `GenerateCapabilitiesIndex` | `MOODLE_CAPABILITIES_INDEX.md` | `capabilities` |
| `GeneratePluginIndex` | `MOODLE_PLUGIN_INDEX.md` | `plugin` |
| `GenerateDevRules` | `MOODLE_DEV_RULES.md` | — (conteúdo estático) |
| `GeneratePluginGuide` | `MOODLE_PLUGIN_GUIDE.md` | — (conteúdo estático + versão) |
| `GenerateAiWorkspace` | `MOODLE_AI_WORKSPACE.md` | `plugin` (via a lista de diretórios de plugin) |
| `GenerateAiIndex` | `MOODLE_AI_INDEX.md` | — (lista arquivos existentes) |
| `GenerateCtags` | `.build82/tags` | — (chama `ctags` externamente) |

---

## Generators de plugin (`plugin.go`)

Geram arquivos sob `{raiz_plugin}/.build82/`. Chamados por `generate_plugin_context` e `plugin_batch`.

O ponto de entrada é `GenerateAllForPlugin(pluginPath, moodlePath, markAsDev, existingInfo)`, que carrega o cache mtime, delega para `GenerateAllForPluginCore`, e depois persiste o cache. Orquestradores de lote (`plugin_batch`, o caminho `include_plugins` do `update_indexes`) chamam `GenerateAllForPluginCore` diretamente, envolvendo o loop *inteiro* deles num único par de carga/salvamento de cache — chamar `Save()` uma vez por plugin num lote grande, do contrário, serializaria o I/O e anularia o propósito do worker pool do `plugin_batch`.

Os 10 extractors de plugin (Classes, Hooks, Schema, Events, Tasks, Services, Capabilities, Upgrade, Subplugins, Settings) rodam concorrentemente antecipadamente para uma struct `PreloadedPluginData` compartilhada, então os generators abaixo leem dados já extraídos, em vez de reprocessar os arquivos do plugin. Quando `markAsDev` é verdadeiro, a execução termina escrevendo o marcador `.build82/.indevelopment` (uma falha é logada em stderr, não retornada).

| Função | Arquivo gerado | Campos usados de `PreloadedPluginData` |
| --- | --- | --- |
| `GeneratePluginContext` | `PLUGIN_CONTEXT.md` | `Schema`, `Events`, `Tasks`, `Services`, `Capabilities` |
| `GeneratePluginStructure` | `PLUGIN_STRUCTURE.md` | — (só leitura de diretório) |
| `GeneratePluginDbTables` | `PLUGIN_DB_TABLES.md` | `Schema` |
| `GeneratePluginEvents` | `PLUGIN_EVENTS.md` | `Events` |
| `GeneratePluginDependencies` | `PLUGIN_DEPENDENCIES.md` | `Tasks`, `Services`, `Capabilities`, `Hooks`, `Upgrade`, `Subplugins` |
| `GeneratePluginFunctionIndex` | `PLUGIN_FUNCTION_INDEX.md` | — (glob + leitura de arquivo, extractor `api` por arquivo) |
| `GeneratePluginCallbackIndex` | `PLUGIN_CALLBACK_INDEX.md` | `Hooks` (+ uma única leitura de `lib.php`/`locallib.php`, compartilhada entre as checagens de sufixo legado de `legacyhooks.Map`) |
| `GeneratePluginEndpointIndex` | `PLUGIN_ENDPOINT_INDEX.md` | `Services` |
| `GeneratePluginRuntimeFlow` | `PLUGIN_RUNTIME_FLOW.md` | `Classes`, `Events`, `Tasks`, `Services` |
| `GeneratePluginArchitecture` | `PLUGIN_ARCHITECTURE.md` | `Classes` |
| `GeneratePluginSettings` | `PLUGIN_SETTINGS.md` | `Settings` |
| `GeneratePluginAiContext` | `PLUGIN_AI_CONTEXT.md` | todos os campos de `PreloadedPluginData` exceto `Upgrade` |

---

## Detalhes de design

### Barreira de erro — `genutil.Safely`

Todo corpo de generator é envolto por `genutil.Safely`, que recupera de um panic e o converte (ou um erro retornado) num `GeneratorResult` de falha, em vez de derrubar o lote inteiro:

```go
func Safely(outputFile string, fn func() (GeneratorResult, error)) GeneratorResult {
	result, err := func() (r GeneratorResult, e error) {
		defer func() {
			if p := recover(); p != nil {
				e = fmt.Errorf("%v", p)
			}
		}()
		return fn()
	}()
	if err != nil {
		return GeneratorResult{File: outputFile, Success: false, Error: err.Error()}
	}
	return result
}
```

Se, por exemplo, o `db/install.xml` estiver malformado o suficiente para tropeçar em algo inesperado, `GeneratePluginDbTables` falha sozinho e os outros 11 generators de plugin continuam rodando normalmente.

---

### Cabeçalho padrão — `genutil.Header`

Todo arquivo `.md` gerado começa com um cabeçalho padronizado:

```
# Título do Arquivo

> Descrição do conteúdo

_Generated by build82 on 2024-04-22 10:30:00_

---
```

---

### Migração — arquivos legados soltos → `.build82/`

Antes de qualquer generator rodar, `MigrateLegacyGlobalFiles`/`MigrateLegacyPluginFiles` (em `migration.go`) movem qualquer um dos 13 nomes globais (mais `tags`) / 12 nomes de plugin (mais `.indevelopment`) que encontrarem soltos diretamente na raiz do Moodle/plugin para dentro de `.build82/` (o layout plano legado). Ambas delegam para `MigrateLegacyFiles`, que rejeita nomes absolutos ou com traversal, se recusa a mover symlinks e remove a cópia da raiz quando já existe uma cópia em `.build82/`. Isso roda incondicionalmente e de forma idempotente em toda passada (o `doctor` usa o `DetectLegacyFiles`, somente leitura); um arquivo que falhe ao migrar (permissão negada, um symlink, disco cheio) é logado em stderr em vez de ficar silenciosamente no lugar errado, mas nunca bloqueia o resto da execução.

---

### Filtrando arquivos gerados do `PLUGIN_STRUCTURE.md`

A árvore de diretórios do `GeneratePluginStructure` precisa excluir a própria saída do build82. Como tudo agora vive sob um único diretório `.build82/`, isso é uma única checagem de nome (`ContextDir`), não um conjunto por nome de arquivo — uma simplificação estrutural que o layout legado solto não permitia.

---

### `PLUGIN_AI_CONTEXT.md` — o consolidador

`GeneratePluginAiContext` consolida todos os campos de `PreloadedPluginData` num único arquivo otimizado para ser o ponto de entrada da IA. Roda junto com os outros 11 generators de plugin — todos compartilham os mesmos dados pré-extraídos, então não há custo extra em também construir este resumo.

---

### Generators globais e de plugin: ambos rodam concorrentemente

Os generators globais em `GenerateAll` rodam concorrentemente — são independentes entre si e escrevem em arquivos diferentes, então não há risco de race condition (o próprio cache mtime é protegido por mutex).

Os generators de plugin em `GenerateAllForPluginCore` também rodam concorrentemente — como todos os dados de extractor são pré-carregados antecipadamente em `PreloadedPluginData`, nenhum generator depende da saída de outro.

---

### `GenerateCtags` — geração opcional de ctags

Só chama `ctags` externamente (`ctags -R --languages=PHP`, excluindo `vendor` e `node_modules`) se um binário `ctags` for encontrado no `PATH`, e só se os arquivos-fonte rastreados forem mais novos que o `.build82/tags` existente (mesma checagem de cache mtime dos outros generators globais). Se `ctags` não estiver instalado, retorna um resultado de sucesso ignorado (`Skipped: true`) e nenhum arquivo `tags` é criado — o `doctor` reporta isso como uma ferramenta opcional, não obrigatória.

---

## Adicionando um novo generator

**1. Decida onde o arquivo gerado deve viver:**

- Raiz do Moodle → adicione em `moodle.go` e registre em `GenerateAll`
- Diretório do plugin → adicione em `plugin.go` e registre em `GenerateAllForPluginCore`

**2. Implemente a função seguindo o contrato:**

```go
func GenerateMyFile(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MY_FILE.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		data := extractors.ExtractMyData(filepath.Join(moodlePath, "lib", "myfile.php"))

		var b strings.Builder
		b.WriteString(genutil.Header("My File", "Content description"))
		// ... construa o conteúdo a partir de data ...

		return genutil.Write(output, b.String()), nil
	})
}
```

A checagem do cache mtime (`cache.Global.IsStale`) acontece no chamador (`genutil.RunCached`, usado por `GenerateAll`/`GenerateAllForPluginCore`), não dentro do próprio generator.

**3. Registre-o em `GenerateAll` ou `GenerateAllForPluginCore`**, fornecendo a lista de arquivos-fonte que devem controlar sua obsolescência.

**4. Adicione o nome do arquivo a `GlobalContextFilenames`/`PluginContextFiles` em `migration.go`** se for um arquivo novo, para que a lógica de migração e exclusão o reconheça automaticamente.

**5. Documente o novo arquivo em [Arquivos Gerados](../reference/generated-files.md).**

---

## Veja também

- [Extractors](./extractors.md) — os pacotes que alimentam os generators
- [Sistema de Cache](./cache-system.md) — quando um generator pula a escrita
- [Arquivos Gerados](../reference/generated-files.md) — lista completa dos arquivos `.md` produzidos

---

[🏠 Voltar ao Índice](../index.md)
