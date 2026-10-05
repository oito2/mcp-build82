🌐 [English](../../en/reference/configuration.md) | **Português** | 🏠 [Índice](../index.md)

---

# Referência de Configuração

Fonte da verdade: `internal/config/config.go`, `cmd/build82/main.go`, `internal/extractors/backend.go`, `internal/tools/release.go`, `internal/genutil/genutil.go`.

## O arquivo de configuração

| Propriedade | Valor |
| :--- | :--- |
| Caminho | `.build82` no diretório home do usuário (`os.UserHomeDir`): `~/.build82` no Linux e macOS, `%USERPROFILE%\.build82` no Windows. Se o diretório home não puder ser resolvido, os comandos que precisam dele falham com `resolve home directory: <causa>`. |
| Escrito por | A ferramenta `init_moodle_context` (somente quando ainda não inicializado, ou com `force: true`) e `update_indexes` (somente quando a versão detectada do Moodle difere da armazenada). Nunca escrito pelos subcomandos da CLI. |
| Lido por | Toda ferramenta, recurso e prompt que precisa da raiz do Moodle; `build82 uninstall --purge`. |
| Apagado por | `build82 uninstall --purge`. Veja [Desinstalação](../getting-started/uninstallation.md). |
| Método de escrita | Substituição atômica, modo `0644`. Sem trava entre processos; com escritores concorrentes, o último prevalece. |

### Formato

Texto simples, um `CHAVE=VALOR` por linha, sem aspas, sem seções. Exatamente três linhas são escritas, nesta ordem:

```text
MOODLE_PATH=/var/www/moodle
MOODLE_VERSION=4.5
MOODLE_FULLVERSION=2024100700
```

| Chave | Obrigatória | Descrição |
| :--- | :---: | :--- |
| `MOODLE_PATH` | Sim | Caminho absoluto da raiz do Moodle. Se ausente ou vazia, o arquivo inteiro é tratado como inexistente. |
| `MOODLE_VERSION` | Não | Versão do Moodle, por exemplo `4.5`, extraída do início de `$release` no `version.php` da raiz do Moodle. String vazia quando não detectada. |
| `MOODLE_FULLVERSION` | Não | Número de build numérico: o valor de `$version` no `version.php` da raiz do Moodle, por exemplo `2024100700`. String vazia quando não detectada. |

Regras de interpretação na leitura:

| Regra | Detalhe |
| :--- | :--- |
| Seleção de linhas | Linhas sem `=` são ignoradas; linhas cujo primeiro caractere não branco é `#` são ignoradas. |
| Divisão | No primeiro `=`; chave e valor têm os espaços das pontas removidos. |
| Duplicadas | A última ocorrência de uma chave prevalece. |
| Chaves desconhecidas | Ignoradas. |
| Arquivo ilegível | Um erro de leitura diferente de "não existe" imprime `build82: warning: could not read config file <caminho>: <erro>` no stderr e é tratado como ausência de configuração. |
| Escrita | Um valor contendo `\n` ou `\r` é recusado: `config value contains a newline, refusing to write`. |

## Variáveis de ambiente

| Variável | Usada por | Tipo | Padrão | Descrição |
| :--- | :--- | :--- | :--- | :--- |
| `BUILD82_MOODLE_PATH` | Servidor | string (caminho absoluto) | não definida | Raiz do Moodle. Quando definida com valor não em branco (espaços das pontas removidos), o arquivo de configuração nem é lido. O `build82 install` a grava no `env` de cada entrada de cliente. |
| `BUILD82_MOODLE_VERSION` | Servidor | string | vazia | Versão do Moodle. Lida somente quando `BUILD82_MOODLE_PATH` está definida. Espaços removidos. |
| `BUILD82_MOODLE_FULLVERSION` | Servidor | string | vazia | Número de build numérico (o `$version` do `version.php`, por exemplo `2024100700`). Lida somente quando `BUILD82_MOODLE_PATH` está definida. Espaços removidos. |
| `BUILD82_EXTRACTOR_BACKEND` | Servidor | string | não definida | Exatamente `treesitter` seleciona o backend de extração PHP baseado em tree-sitter. Qualquer outro valor, ou ausência, seleciona o backend regex. Diferencia maiúsculas de minúsculas; avaliada a cada chamada de extração. Veja [Extratores](../architecture/extractors.md). |
| `BUILD82_TOKEN` | `--http` | string | não definida | Token Bearer para `/mcp` e `/sse`. Veja a semântica abaixo. |
| `CLINE_DATA_DIR` | `install`, `uninstall` | string (caminho absoluto) | não definida | O diretório de dados da própria CLI do Cline, que substitui `~/.cline/data`. Quando definida com um valor não vazio, o alvo `cline` detecta a CLI do Cline por esse diretório e grava/remove `$CLINE_DATA_DIR/settings/cline_mcp_settings.json` em vez de `~/.cline/data/settings/cline_mcp_settings.json`. Veja [Cline](../guides/clients/cline.md). |

### Semântica de `BUILD82_TOKEN`

| Estado | Efeito |
| :--- | :--- |
| Não definida e sem flag `--token` | Autenticação desativada (aviso no stderr). |
| Definida com valor não vazio, sem flag `--token` | O valor é o token. |
| Definida porém vazia (`export BUILD82_TOKEN=`), sem flag `--token` | Erro na inicialização, código de saída 1: `BUILD82_TOKEN is set but empty; unset it entirely to run --http without authentication`. |
| Qualquer valor, com `--token <não vazio>` | A flag prevalece; a variável é ignorada. |
| Qualquer valor, com `--token ""` | Erro na inicialização, código de saída 1 (a flag é verificada primeiro). |

O valor não tem os espaços removidos. Lida somente com `--http`; ignorada no stdio. Fluxo completo: [Referência da CLI](./cli.md#resolução-do-token).

## Precedência

| Configuração | Ordem (maior prioridade primeiro) |
| :--- | :--- |
| Raiz do Moodle, versão, versão completa | 1. `BUILD82_MOODLE_PATH` (com suas `BUILD82_MOODLE_VERSION` e `BUILD82_MOODLE_FULLVERSION`) 2. `~/.build82`. |
| Token Bearer HTTP | 1. `--token` 2. `BUILD82_TOKEN` 3. nenhum (desativado). |
| Porta, host, hosts permitidos | Somente flags (`--port`, `--host`, `--allowed-host`); sem variável de ambiente nem arquivo. |
| Backend de extração | Somente `BUILD82_EXTRACTOR_BACKEND`. |

Comportamentos a observar:

| Caso | Resultado |
| :--- | :--- |
| `BUILD82_MOODLE_PATH` definida | A fonte é o ambiente como um todo. Os valores nunca são mesclados com o arquivo: `MOODLE_VERSION` do arquivo é ignorada, mesmo que `BUILD82_MOODLE_VERSION` não esteja definida. |
| Nenhuma fonte presente | Sem configuração; as ferramentas informam que o build82 não foi inicializado. |
| `init_moodle_context` com configuração já presente (de qualquer fonte) | Retorna "already initialized" e não altera nada, a menos que `force: true`. |
| `update_indexes` quando a versão detectada difere | Reescreve `~/.build82` com o `MoodlePath` configurado (o caminho do ambiente, quando a fonte é o ambiente) e as versões detectadas. |

## Diretório de saída: `.build82/`

Tudo o que o build82 gera fica sob um diretório `.build82/`, na raiz do Moodle (arquivos globais, `tags`, `.cache.json`) e na raiz de cada plugin (arquivos por plugin, `.indevelopment`). Nada é escrito diretamente em nenhuma das raízes. O nome do diretório é fixo e não configurável. Lista de arquivos e descrições: [Arquivos Gerados](./generated-files.md).

## Arquivos de marcação e de exclusão

| Arquivo | Local | Lido por | Formato e efeito |
| :--- | :--- | :--- | :--- |
| `.indevelopment` | `<raiz_plugin>/.build82/.indevelopment` | Ferramentas que listam ou processam em lote plugins em desenvolvimento, `doctor`, `update_indexes` (`include_plugins`), o file watcher e `uninstall --purge` | Marcador escrito por `generate_plugin_context` (sempre) e por `plugin_batch` (modo `dev`, ou com `mark_as_dev`). Conteúdo: um timestamp. Um plugin conta como em desenvolvimento se o arquivo existir dentro de um diretório `.build82`; o conteúdo não é interpretado. Excluído dos ZIPs do `release_plugin`. |
| `.buildignore` | `<raiz_plugin>/.buildignore` (opcional) | `release_plugin` | Um nome por linha; linhas em branco e linhas iniciadas por `#` são ignoradas; cada linha tem os espaços das pontas removidos. Os nomes são comparados exatamente com nomes-base de arquivos ou diretórios em qualquer profundidade (sem globs, sem caminhos). Soma-se ao conjunto fixo de exclusões. Arquivo ausente não é erro. O próprio arquivo nunca é empacotado. |

Exclusões fixas do `release_plugin` (nomes-base, em qualquer profundidade): `.build82`, `CLAUDE.md`, `GEMINI.md`, `AGENTS.md`, `.claudeignore`, `.geminiignore`, `.aiexclude`, `node_modules`, `.buildignore`, `.git`, `.indevelopment` e os 12 nomes de arquivo `PLUGIN_*.md`.

## Relacionado

- [Referência da CLI](./cli.md)
- [Arquivos Gerados](./generated-files.md)
- [Desinstalação](../getting-started/uninstallation.md)
