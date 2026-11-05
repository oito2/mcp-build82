🌐 [English](../../en/architecture/extractors.md) | **Português** | 🏠 [Índice](../index.md)

---

# Extractors

**Extractors** são os pacotes Go responsáveis por ler e interpretar arquivos PHP e XML da instalação Moodle. Cada extractor é especializado em um tipo específico de arquivo e produz dados estruturados que os [Generators](./generators.md) transformam em arquivos `.md` de contexto.

---

## Visão geral

```
internal/extractors/
├── api.go            ← lib/*.php
├── capabilities.go   ← db/access.php
├── classes.go         ← classes/**/*.php + db/renamedclasses.php
├── events.go          ← db/events.php
├── hooks.go           ← db/hooks.php + classes/hook/*.php + callbacks legados do lib.php
├── moodledetect.go     ← version.php (raiz do Moodle)
├── plugin.go           ← version.php (plugin) + lang/
├── schema.go           ← db/install.xml
├── services.go         ← db/services.php
├── tasks.go            ← db/tasks.php
├── upgrade.go          ← db/upgrade.php
├── settings.go         ← settings.php (declarações admin_setting_*)
├── subplugins.go       ← db/subplugins.json (legado: db/subplugins.php)
├── deprecated.go       ← doctor: chamadas a funções do core marcadas @deprecated no PHP do plugin
├── capabilityusage.go  ← doctor: checagens has_capability()/require_capability() vs capabilities declaradas
├── langstrings.go      ← doctor: chamadas get_string() vs strings declaradas em lang/en
├── shared.go           ← readFileCapped (limite de leitura de 8 MiB) e varredura de diretório segura contra symlinks
├── backend.go          ← dispatch de useTreesitter(), lido a cada chamada, nunca cacheado
└── tsbackend/          ← o backend tree-sitter opcional, pacote espelho (veja abaixo)
```

Todo extractor é uma função pura: recebe um caminho de arquivo ou diretório, retorna dados estruturados, e **nunca escreve em disco** — isso é responsabilidade exclusiva dos Generators.

---

## Dois backends, um contrato só

Os extractors que fazem parsing de PHP estruturado — `api.go`, `capabilities.go`, `classes.go`, `events.go`, `hooks.go`, `services.go`, `tasks.go`, `upgrade.go`, mais a leitura do `version.php` em `plugin.go` — têm **dois backends intercambiáveis**. Os demais usam apenas regex/XML e ignoram `BUILD82_EXTRACTOR_BACKEND`: `moodledetect.go` e `schema.go` (linhas de cabeçalho e XML — nada a ganhar com um parser PHP de verdade), `settings.go`, `subplugins.go` e as varreduras de diagnóstico do `doctor` (`deprecated.go`, `capabilityusage.go`, `langstrings.go`).

| Backend | Pacote | Padrão | Trade-off |
| --- | --- | --- | --- |
| **regex** | `internal/extractors/*.go` | ✅ Sim | Rápido, baixo consumo de memória, alguns casos extremos que não consegue expressar (aspas escapadas dentro de strings, cláusulas `implements` multi-linha, atributos PHP 8 entre um docblock e uma função...) |
| **tree-sitter** | `internal/extractors/tsbackend/*.go` | Opcional via `BUILD82_EXTRACTOR_BACKEND=treesitter` | ~250x mais lento e ~100x mais memória por chamada (medido contra um `moodlelib.php` real de ~10 mil linhas — veja `tsbackend/BENCHMARKS.md`), mas trata corretamente todos os casos extremos acima |

O `useTreesitter()` do `backend.go` lê a variável de ambiente a cada chamada (nunca cacheado), então testes e chamadores podem trocar de backend no meio do processo. Os dois backends divergem em alguns casos de borda, todos a favor do tree-sitter; os testes de paridade entre backends em `internal/extractors/*_test.go` registram em log cada diferença esperada.

O `internal/extractors` importa o `tsbackend` para o dispatch, então o `tsbackend` não pode importar de volta os tipos do extractor sem criar um ciclo de import. Por isso os dois backends devolvem os mesmos tipos Go, definidos uma única vez no pacote neutro `internal/phptypes` (apenas definições de tipo); o `internal/extractors` os reexporta como type aliases (ex.: `type ApiFunction = phptypes.ApiFunction`). Duas peças de lógica que precisam ficar idênticas entre os backends — o mapa de sufixo-de-callback-legado→substituto-na-Hook-API, e a classificação de visibilidade do PHPDoc — vivem em pacotes compartilhados próprios (`internal/legacyhooks`, `internal/phpdoc`) que nenhum dos dois backends importa do outro.

---

## Tabela de referência

| Extractor | Arquivo-fonte | Dados produzidos |
| --- | --- | --- |
| `api.go` | `lib/*.php` (só funções de nível superior) | Funções com nome, linha, visibilidade PHPDoc, `@since`, `@deprecated` |
| `capabilities.go` | `db/access.php` | Capabilities com nome, riskbitmask, captype, archetypes |
| `classes.go` | `classes/**/*.php`, `db/renamedclasses.php` | Classes/interfaces/traits/enums com namespace, FQN, kind, extends/implements; mapa de autoload de classes renomeadas |
| `events.go` | `db/events.php` | Observers com eventname, callback, priority, flag internal |
| `hooks.go` | `db/hooks.php` + `classes/hook/*.php` | Callbacks da Hook API + definições de hook + avisos de migração de callbacks legados do `lib.php` |
| `moodledetect.go` | `version.php` (raiz do Moodle) | Versão do Moodle (legível e numérica) |
| `plugin.go` | `version.php` do plugin + `lang/en/<component>.php` | Component, tipo, nome, versão, requires, display name, maturity |
| `schema.go` | `db/install.xml` | Tabelas, campos, chaves, índices |
| `services.go` | `db/services.php` | Funções de web service com classname, methodname, type, ajax, capabilities |
| `tasks.go` | `db/tasks.php` | Tasks agendadas com classname e cron schedule |
| `upgrade.go` | `db/upgrade.php` | Upgrade steps com versão e descrição inferida |
| `settings.go` | `settings.php` | Configurações de admin: nome da classe `admin_setting_*` e (quando é string literal) nome da configuração |
| `subplugins.go` | `db/subplugins.json`, com fallback para o legado `db/subplugins.php` | Tipos de subplugin declarados pelo plugin (prefixo do tipo → caminho) |
| `deprecated.go` | `*.php` do plugin (recursivo) + nomes deprecated do índice de API | Pontos de chamada de funções do core `@deprecated` (usado pelo `doctor`) |
| `capabilityusage.go` | `*.php` do plugin | Chamadas `has_capability()`/`require_capability()` que citam as capabilities do próprio plugin (usado pelo `doctor`) |
| `langstrings.go` | `lang/en/<component>.php` + `*.php` do plugin | Strings de idioma declaradas e chamadas `get_string()` (usado pelo `doctor`) |

---

## Detalhes por extractor

### `api.go` — Funções do core

**Arquivo-fonte:** `lib/*.php`, só funções de nível superior (não métodos dentro de classes).

**Estratégia de parsing (regex):** varredura linha a linha pareando um bloco `/** ... */` de PHPDoc precedente (tolerando até 3 linhas em branco e atributos `#[...]` do PHP 8 no meio, de uma ou várias linhas, e um fechamento `**/`) com a declaração `function` seguinte.

**Classificação de visibilidade** (compartilhada com o backend tree-sitter via `internal/phpdoc`):

| Tag do PHPDoc | Visibilidade atribuída |
| --- | --- |
| `@deprecated` | `deprecated` |
| `@internal` | `internal` |
| `@access private` ou `@private` | `private` |
| Nenhuma tag restritiva | `public` |
| Nenhum PHPDoc | `unverified` |

O `ExtractMoodleApi` varre os arquivos `.php` diretamente sob `lib/` (ordenados por nome, sem recursão), conta cada classe de visibilidade e devolve apenas as funções `public` e `deprecated`.

**Alimenta:** `MOODLE_API_INDEX.md`.

---

### `schema.go` — Banco de dados

**Arquivo-fonte:** `db/install.xml` de qualquer plugin.

**Estratégia de parsing:** o `encoding/xml` padrão do Go para decodificar o XMLDB; um arquivo que não decodifica por inteiro não gera schema (não há recuperação por tabela). Os atributos string `NOTNULL`/`SEQUENCE`/`NEXT` do XMLDB (`"true"`/`"false"`) são normalizados para booleanos de verdade.

**Alimenta:** `PLUGIN_DB_TABLES.md` e `MOODLE_DB_TABLES_INDEX.md`.

---

### `events.go` — Observers de eventos

**Arquivo-fonte:** `db/events.php` de qualquer plugin.

**Estratégia de parsing (regex):** parsing de array literal PHP via o pacote compartilhado `internal/phparray`, que entende sintaxe de string/comentário PHP o suficiente para encontrar corpos de array balanceados e extrair valores string/int/bool sem um parser completo.

**Alimenta:** `PLUGIN_EVENTS.md` e `MOODLE_EVENTS_INDEX.md`.

---

### `hooks.go` — Hook API (Moodle 4.3+)

**Arquivo-fonte:** `db/hooks.php` (callbacks registrados) + `classes/hook/*.php` (definições de hook) + `lib.php` do plugin (detecção de callback legado — o Moodle só carrega o `lib.php` para chamar os callbacks de um plugin, então uma função no `locallib.php` não é um callback; o `DetectLegacyCallbacksInFile` aplica as mesmas regras a outro arquivo, e o `PLUGIN_CALLBACK_INDEX.md` o usa para listar à parte, como aviso, as declarações do `locallib.php`).

**Estratégia de parsing:** três sub-varreduras — o array de callbacks em `db/hooks.php`, metadados de classe em `classes/hook/*.php`, e um regex sobre `lib.php` comparando com qualquer um dos 11 sufixos de callback legado (`before_footer`, `before_http_headers`, `standard_footer_html`, ...) que o core do Moodle declara como substituídos por um hook (o atributo `replaces_callbacks` da classe do hook ou `get_deprecated_plugin_callbacks()`), listados com o hook em `internal/legacyhooks`; os callbacks de um módulo de atividade são reconhecidos tanto como `mod_<nome>_` quanto como `<nome>_`, e os nomes são comparados sem diferenciar maiúsculas de minúsculas, como faz o PHP. Os dois backends montam os avisos com `legacyhooks.Warnings`, e a tabela do lib.php no `PLUGIN_CALLBACK_INDEX.md` lista exatamente esses avisos. Uma declaração indentada dentro de um guard `if (!function_exists(...))` conta (com chaves ou na forma `if (): ... endif;`); uma comentada não. Registros e observers de eventos podem citar classes como strings entre aspas ou como `X::class` (sozinho, `X::class . '::metodo'` ou `[X::class, 'metodo']`, que dão `X::metodo`), uma classe de hook pode ser declarada com quaisquer modificadores `abstract`, `final` e `readonly` (a mesma regra de declaração do `classes.go`), e os metadados do hook são lidos dos atributos `#[\core\attribute\label]`, `#[\core\attribute\tags]` e `#[\core\attribute\hook\replaces_callbacks]` antes dos métodos `get_hook_description()`/`get_hook_tags()` e do docblock da classe.

**Alimenta:** `PLUGIN_CALLBACK_INDEX.md` e `PLUGIN_DEPENDENCIES.md`.

---

### `capabilities.go` — Capabilities

**Arquivo-fonte:** `db/access.php` de qualquer plugin.

**Estratégia de parsing:** parsing de array literal sobre o array `$capabilities`, com chave `tipo/nome:capabilityname`. O `riskbitmask` pode ser uma única constante nomeada (ex: `RISK_SPAM`) ou uma expressão composta com `|` entre várias — o backend tree-sitter resolve a expressão completa; o backend regex só captura o primeiro token (documentado como divergência conhecida, não um bug a corrigir, já que é um limite inerente da abordagem regex).

**Alimenta:** `PLUGIN_DEPENDENCIES.md` e `MOODLE_CAPABILITIES_INDEX.md`.

---

### `classes.go` — Classes PHP

**Arquivo-fonte:** `classes/**/*.php` de qualquer plugin (também o glob restrito `**/classes/**/*.php` do `MOODLE_CLASSES_INDEX.md` em toda a instalação) e `db/renamedclasses.php`.

**Estratégia de parsing:** regex linha a linha para declarações `class`/`interface`/`trait`/`enum` (com quaisquer modificadores `abstract`, `final` e `readonly`), com um pequeno look-ahead para juntar uma declaração multi-linha até o `{` de abertura. Pula arquivos symlink durante a varredura do diretório — um arquivo real plantado como symlink apontando para fora da árvore varrida não deve ter o conteúdo do seu alvo interpretado.

**Alimenta:** `MOODLE_CLASSES_INDEX.md` e `PLUGIN_ARCHITECTURE.md`.

---

### `services.go` — Web services

**Arquivo-fonte:** `db/services.php` de qualquer plugin.

**Estratégia de parsing:** parsing de array literal sobre o array `$functions`, com chave por nome de função, extraindo `classname`, `methodname`, `description`, `type`, `ajax`, `capabilities`. Um valor escrito como concatenação `.` de strings (comum nas descrições longas do core) é lido inteiro pelo backend tree-sitter; o backend regex fica só com o primeiro fragmento (`tsbackend/KNOWN_DIVERGENCES.md` §3). Campos booleanos (`ajax` aqui, `internal`, `blocking`, `defaultenabled` nos outros) aceitam `true`/`false` em qualquer caixa e `1`/`0` nos dois backends; qualquer outro valor mantém o padrão do campo.

**Alimenta:** `PLUGIN_ENDPOINT_INDEX.md` e `MOODLE_SERVICES_INDEX.md`.

---

### `tasks.go` — Tasks agendadas

**Arquivo-fonte:** `db/tasks.php` de qualquer plugin.

**Estratégia de parsing:** parsing de array literal sobre o array `$tasks`, extraindo `classname` e os campos de agendamento estilo cron (minute/hour/day/month/dayofweek), além da flag `blocking`.

**Alimenta:** `PLUGIN_DEPENDENCIES.md` e `MOODLE_TASKS_INDEX.md`.

---

### `upgrade.go` — Histórico de upgrade

**Arquivo-fonte:** `db/upgrade.php` de qualquer plugin.

**Estratégia de parsing:** varredura de fluxo de controle (não só arrays literais) por blocos `if ($oldversion < V)`, em que V é uma versão de dez dígitos com parte decimal opcional (`2024010100` ou `2024010100.01`; qualquer outro número não é um passo), passos ordenados numericamente, com um fallback de descrição em 3 níveis: comentário na mesma linha, depois a primeira referência `new xmldb_table('literal')` cujo argumento de fato resolve para uma string literal, depois o primeiro comentário `//` solto em qualquer lugar do bloco. O backend regex localiza os passos com comentários e corpos de heredoc apagados, então um passo comentado não é reportado e uma aspa dentro de um heredoc não esconde os passos seguintes; as descrições são lidas do texto original. Os passos são localizados pela mesma função que o `ParseUpgradeOperations` usa (`upgradeops.go`, que alimenta a verificação de consistência de upgrade do `doctor`), e o backend tree-sitter aplica a mesma regra de versão (`moodletype.IsUpgradeVersion`).

**Alimenta:** `PLUGIN_DEPENDENCIES.md`.

---

### `moodledetect.go` — Versão do Moodle

**Arquivo-fonte:** `version.php` na raiz do Moodle.

**Estratégia de parsing:** regex sobre `$version` (numérico, ex: `2024042200`), `$release` (string, ex: `4.4 (Build: 20240422)`) e `$branch`. Também usado para detectar se um diretório é uma raiz Moodle (para o `doctor` e o instalador da CLI).

**Alimenta:** a configuração `~/.build82` e o cabeçalho do `AI_CONTEXT.md`.

---

### `plugin.go` — Metadados do plugin

**Arquivo-fonte:** `version.php` do plugin + `lang/en/<component>.php`.

**Estratégia de parsing:** regex sobre `$plugin->component`, `$plugin->version`, `$plugin->requires`, `$plugin->maturity`, com os comentários apagados antes (uma atribuição comentada é ignorada). Quando um campo é atribuído mais de uma vez, vale a última atribuição, como quando o PHP executa o arquivo, e uma versão decimal (`2024010100.01`) mantém a parte decimal; o backend tree-sitter lê os mesmos valores. Usa o mapa `PluginTypeToDir` (os tipos de plugin do core do `lib/components.json` mais os tipos de subplugin comuns) do `internal/moodletype` como única fonte de verdade para resolver o tipo do plugin a partir do caminho do diretório.

**Alimenta:** `PLUGIN_CONTEXT.md` e `MOODLE_PLUGIN_INDEX.md`.

---

### `settings.go`, `subplugins.go` — metadados extras do plugin

O `settings.go` varre o `settings.php` com regex atrás de declarações `new admin_setting_*(` (um nome não literal é mantido com nome vazio). O `subplugins.go` lê o `db/subplugins.json` (os caminhos de `plugintypes`, relativos à raiz do Moodle; um tipo presente só na chave `subplugintypes`, relativa ao plugin, é juntado ao diretório do plugin) e só recorre ao `db/subplugins.php` quando o JSON não existe. Nenhum dos dois tem equivalente em tree-sitter.

**Alimentam:** `PLUGIN_SETTINGS.md` (settings); `PLUGIN_DEPENDENCIES.md` e `PLUGIN_AI_CONTEXT.md` (subplugins; settings também no contexto de IA).

---

### `deprecated.go`, `capabilityusage.go`, `langstrings.go` — diagnósticos do `doctor`

Varreduras somente com regex usadas exclusivamente pela tool `doctor`, e por nenhum generator: chamadas diretas a funções marcadas `@deprecated` no índice de API do core (chamadas de método e estáticas são ignoradas), checagens de capability contra o prefixo das capabilities do próprio plugin (`tipo/nome`, a forma que o Moodle exige para todo tipo de plugin) e chamadas `get_string()` que citam o plugin (`mod_forum` ou `forum` num módulo) versus as chaves declaradas no seu arquivo de idioma inglês (`lang/en/<component>.php`, `lang/en/<nome>.php` num módulo).

---

### `shared.go` — acesso seguro a arquivos

O `readFileCapped` se recusa a carregar em memória arquivos maiores que 8 MiB e só lê arquivos regulares (`fsutil.ReadRegular`: um link simbólico no arquivo não é seguido, e um FIFO ou dispositivo é recusado sem bloquear), e a varredura de diretório compartilhada ignora arquivos que são symlinks, de modo que analisar código de plugins de terceiros não confiáveis não esgota a memória, não lê arquivos fora da árvore do plugin nem trava num FIFO plantado. Um arquivo que cresce além do limite durante a leitura também é recusado. O backend tree-sitter aplica a mesma regra de arquivo regular com um limite menor, de 2 MiB, já que um parse aloca centenas de bytes por byte de fonte, e interrompe um parse após 10 segundos; um parse interrompido antes do fim (timeout ou orçamento de memória do parser) é um erro, nunca uma árvore parcial. Os números de linha das correspondências de regex vêm de um índice pré-calculado das quebras de linha, então um arquivo com muitas correspondências é analisado em tempo linear.

---

## Adicionando um novo extractor

Para adicionar suporte a um novo tipo de arquivo do Moodle:

**1. Crie o arquivo em `internal/extractors/`:**

```go
// internal/extractors/meuextractor.go
package extractors

type MeuDado struct {
	Campo string
}

func ExtractMeuDado(filePath string) *MeuDado {
	// leia o arquivo, faça o parse, retorne dados estruturados (nil se o arquivo não existir)
	return nil
}
```

**2. Se precisar de uma contraparte tree-sitter**, adicione o espelho em `internal/extractors/tsbackend/meuextractor.go` (com sua própria struct, nunca importando `extractors`), e faça o dispatch a partir da função do lado regex via `useTreesitter()` — veja `backend.go` para o padrão já existente.

**3. Conecte ao generator relevante em `internal/generators/`:** chame o extractor dentro de `moodle.go` (índices globais) ou `plugin.go` (contexto de plugin).

**4. Escreva testes reais:** uma fixture bem formada, um caso de arquivo ausente e — se houver dois backends — um teste de paridade comparando os dois contra a mesma fixture, mais (quando uma instalação Moodle real estiver disponível no ambiente) uma varredura de paridade contra todo arquivo real correspondente.

**5. Formate, compile, valide e teste o módulo inteiro:**

```bash
gofmt -w . && go build ./... && go vet ./... && go test -race ./...
```

**6. Documente o novo arquivo em [Arquivos Gerados](../reference/generated-files.md).**

---

## Veja também

- [Generators](./generators.md) — como os dados dos extractors viram arquivos `.md`
- [Sistema de Cache](./cache-system.md) — quando o extractor é chamado e quando é pulado
- [Arquivos Gerados](../reference/generated-files.md) — os arquivos `.md` que cada extractor alimenta

---

[🏠 Voltar ao Índice](../index.md)
