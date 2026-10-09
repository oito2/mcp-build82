# Contribuindo com o build82

🌐 **Idioma:** [English](../../CONTRIBUTING.md) · Português

Obrigado por considerar contribuir. Este documento cobre os passos práticos: como compilar e
testar o projeto, o que se espera de um pull request, e onde tirar dúvidas.

Ao participar deste projeto, você concorda em seguir o [Código de Conduta](codigo-de-conduta.md).

## Antes de começar

Para qualquer coisa além de uma correção pequena (novas tools, novos comandos de CLI, mudanças em
extractors/generators, adição de dependências), abra uma issue primeiro para discutir a
abordagem. Este projeto tem uma filosofia de **dependências mínimas e justificadas** — veja
[Conceitos — Arquitetura](../en/concepts/architecture.md) e o bloco `require` em
[`go.mod`](../../go.mod) para a base atual (`go-sdk`, `fsnotify`, e o backend opcional puro-Go
`gotreesitter` — sem cgo em lugar nenhum, o que mantém a compilação cruzada com
`CGO_ENABLED=0` em [`scripts/release`](../../scripts/release) funcionando). Um PR que adiciona uma
nova dependência sem discussão prévia provavelmente será solicitado a removê-la.

## Configurando o ambiente

Requisito: um toolchain Go compatível com a versão em [`go.mod`](../../go.mod) (Go 1.26+ — o
projeto tem como alvo a última release estável; o `Server.Sessions()` do SDK MCP usa iteradores
range-over-func, que exigem pelo menos Go 1.23).

```bash
git clone https://github.com/oito2/mcp-build82
cd mcp-build82
go build -o build82 ./cmd/build82   # o binário; o ponto de entrada é cmd/build82
go test ./...
```

O caminho do módulo é `github.com/oito2/mcp-build82`.

Rode os mesmos checks que o CI roda antes de abrir um PR:

```bash
gofmt -l .        # não deve imprimir nada
go vet ./...
GOOS=windows go vet ./... && GOOS=darwin go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
GOOS=windows golangci-lint run ./... && GOOS=darwin golangci-lint run ./...   # com o mesmo binário fixado
go build ./...
go test -race ./...
BUILD82_EXTRACTOR_BACKEND=treesitter go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
go mod tidy -diff
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
go run ./scripts/linkcheck
```

Todos precisam passar (a suíte de testes roda uma vez por backend de extração). O
[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) os roda a cada push e pull request
contra `main` (no patch mais recente do Go 1.26.x) e bloqueia o merge caso contrário:

| Job do CI | Roda em | Verificações |
| :--- | :--- | :--- |
| `test` | `ubuntu-24.04`, `macos-latest`, `windows-latest` | vet, build, `go test -race` com os dois backends; no Linux também `gofmt`, golangci-lint e vet para `GOOS=windows`/`darwin` |
| `quality` | `ubuntu-24.04` | `go mod tidy -diff`, actionlint, o verificador de links da documentação, govulncheck, o piso de cobertura e a suíte de testes com `TMPDIR` atrás de um link simbólico (reproduz o `/var` → `/private/var` do macOS) |
| `release-contract` | `ubuntu-24.04` | `go run ./scripts/release -allow-prerelease v0.0.0-ci`, o teste de contrato da release do self-update contra esse `dist/` e `mcp-publisher validate` |

O `golangci-lint` usa o [`.golangci.yml`](../../.golangci.yml) do repositório (linters padrão, com
as exclusões padrão para erros não checados de `Close`/`fmt.Fprint*`) e precisa reportar
`0 issues`. A flag `-race` exige cgo, ou seja, um compilador C funcional na sua máquina; sem um, use
`go test ./...` localmente e confie no CI para a execução com race.

**Piso de cobertura:** o job `quality` mede a cobertura total de instruções com
`go test -race -coverprofile=cover.out -coverpkg=./... ./...` e falha quando ela fica abaixo do
`COVERAGE_FLOOR` do `ci.yml` (86,0% na v1.1.0, medido 86,7%). O piso só pode subir: aumente-o
quando uma release melhorar a cobertura.

**Schemas das tools:** os schemas de entrada e de saída e as anotações de cada tool MCP ficam como
arquivos golden em [`internal/server/testdata/tools/`](../../internal/server/testdata/tools/). Uma
mudança intencional no contrato de uma tool é registrada com
`go test ./internal/server -run TestTools_SchemasMatchGolden -update`.

Os testes exercitam comportamento real sempre que praticável — eventos reais de `fsnotify` para o
watcher, um par real de cliente/servidor MCP em memória para as tools, binários reais compilados
para os checks de install/self-update/CLI — em vez de mockar o SDK ou o filesystem. Testes que
dependem de comportamento só do Unix (FIFOs, `chmod` em diretórios, bits de permissão) se pulam no
Windows; os arquivos de texto são obtidos com fim de linha LF em todo sistema (veja o
[`.gitattributes`](../../.gitattributes)).

### Versões fixadas de ferramentas

Estas versões ficam fixadas nos workflows e **não** são atualizadas pelo Dependabot (que atualiza
semanalmente as actions fixadas por SHA e os módulos Go). Revise-as antes de cada release:

| Ferramenta | Versão | Onde |
| :--- | :--- | :--- |
| golangci-lint | v2.14.0 | `GOLANGCI_LINT_VERSION` no `ci.yml` e no `release.yml` |
| govulncheck | v1.8.0 | `GOVULNCHECK_VERSION` no `ci.yml` e no `release.yml` |
| actionlint | v1.7.12 | `ACTIONLINT_VERSION` no `ci.yml` |
| mcp-publisher | v1.8.1 (+ SHA-256 do tarball) | [`.github/actions/install-mcp-publisher`](../../.github/actions/install-mcp-publisher/action.yml) |
| cosign | v3.0.6 | `cosign-release` no `release.yml` |

## Organização do projeto

- `cmd/build82/` — ponto de entrada; só despacho de argv.
- `internal/extractors/` — parsing puro de PHP/XML, sem dependência de MCP. Inclui o subpacote
  opcional `tsbackend` (tree-sitter) ao lado do backend regex padrão.
- `internal/generators/` — saída em Markdown, migração de arquivos legados, integração com o
  cache persistente.
- `internal/tools/`, `internal/resources/`, `internal/prompts/` — a camada voltada para o MCP.
- `internal/server/` — junta tudo em um único `*mcp.Server`.
- `internal/installer/` — as tabelas de alvo de `install`/`uninstall`.
- `internal/selfupdate/` — o `self-update`.
- `internal/prompt/` — perguntas interativas da CLI que param no primeiro Ctrl-C.
- `internal/config/`, `internal/cache/`, `internal/watcher/`, `internal/fsutil/` — fundacionais,
  sem dependência de MCP (o `fsutil` também tem os leitores que só abrem arquivos regulares,
  usados para arquivos de plugin).
- `scripts/release/` — gera os artefatos da release; `scripts/linkcheck/` — o verificador de links
  da documentação; `scripts/smoke/` — o smoke test com cliente real.

## Fazendo uma mudança

1. Faça um fork do repositório e crie um branch a partir de `main`.
2. Mantenha a mudança focada — uma mudança lógica por PR. Limpezas não relacionadas tornam a
   revisão mais lenta, não mais rápida.
3. Siga o estilo de código e a organização de pacotes existente (veja
   [Organização do projeto](#organização-do-projeto) acima e
   [Conceitos — Arquitetura](../en/concepts/architecture.md)).
4. Adicione ou atualize testes para o comportamento que você mudou. Este projeto depende de
   `go test -race ./...` como rede de segurança — comportamento sem teste é considerado quebrado.
5. Atualize a documentação relevante em [`docs/en/`](../en/) e sua contraparte em
   [`docs/pt-br/`](.) (veja [Documentação](#documentação) abaixo) se você mudou os parâmetros de
   uma tool, um comando de CLI, ou a arquitetura.
6. Rode os checks de [Configurando o ambiente](#configurando-o-ambiente) localmente.

## Mensagens de commit

Escreva uma linha de resumo concisa explicando *por que* a mudança foi feita, não apenas o que
mudou — o diff já mostra o que mudou. Mantenha mudanças relacionadas em um único commit em vez de
uma sequência de commits "fix".

## Documentação

Documentos de nível raiz neste repositório (README, CONTRIBUTING, CODE_OF_CONDUCT) são publicados
em inglês (canônico, na raiz) e português (mesmo nome em minúsculo e traduzido, dentro de
[`docs/pt-br/`](.) — `leiame.md`, `contribuindo.md`, `codigo-de-conduta.md` —, mantido
sincronizado); o site de referência em `docs/` é dividido em árvores paralelas
[`docs/en/`](../en/) e [`docs/pt-br/`](.) do mesmo jeito. Se sua mudança afeta comportamento
descrito na [Referência de Tools](reference/tools.md),
[Referência de Arquivos Gerados](reference/generated-files.md),
[Guia de Instalação](getting-started/installation.md), [Arquitetura](../en/concepts/architecture.md),
ou no README, atualize as duas versões de idioma no mesmo PR — um PR que atualiza só uma será
solicitado a adicionar a outra.

## Pull requests

- Descreva o que mudou e por quê na descrição do PR; vincule a issue relacionada, se existir.
- Mantenha o PR restrito à mudança discutida — refatorações grandes e não solicitadas provavelmente
  serão recusadas mesmo que o código em si esteja correto, seguindo a preferência deste projeto por
  mudanças mínimas e precisas.
- Um mantenedor vai revisar, pedir ajustes se necessário, e fazer o merge quando o CI estiver verde
  e a discussão estiver resolvida.
- O [Dependabot](../../.github/dependabot.yml) abre pull requests semanais que atualizam as GitHub
  Actions usadas pelos workflows; eles passam pelas mesmas verificações do CI.

## Reportando bugs e sugerindo funcionalidades

Abra uma [issue no GitHub](https://github.com/oito2/mcp-build82/issues) com:

- Para bugs: o que você rodou (subcomando do `build82` ou chamada de tool MCP), o que esperava,
  o que aconteceu de fato, e a saída de `build82 --version`.
- Para funcionalidades: o problema que você está tentando resolver, não só a solução que você tem
  em mente — veja [Antes de começar](#antes-de-começar).

## Gerando uma release

As releases são compiladas com um pequeno programa Go dentro do próprio repositório em vez de uma
ferramenta de release de terceiros, para evitar uma dependência extra para algo que a build deste
projeto é simples o suficiente para fazer diretamente:

```bash
go run ./scripts/release vX.Y.Z
go run ./scripts/release -allow-prerelease v0.0.0-ci   # CI e simulações locais
```

Isso compila o `build82` para cada `GOOS`/`GOARCH` suportado (linux/amd64, linux/arm64,
darwin/amd64, darwin/arm64, windows/amd64, windows/arm64 — a lista fica em
`internal/selfupdate.ReleasePlatforms`) dentro de `dist/`, com `-trimpath` e
`-ldflags "-s -w -buildid= -X github.com/oito2/mcp-build82/internal/version.Current=vX.Y.Z"`, num
ambiente fixado (`CGO_ENABLED=0`, `GOAMD64=v1`, `GOARM64=v8.0`, `GOFLAGS` e `GOEXPERIMENT` vazios).
O `-s -w` remove a tabela de símbolos e as informações de depuração DWARF (os stack traces de panic
são mantidos), o `-buildid=` e o `-trimpath` tornam o build reproduzível — o mesmo patch do Go gera
binários idênticos byte a byte localmente e no CI — e o `-X` faz o binário reportar a versão correta
(`build82 --version`) para que a comparação semver do `self-update` funcione contra ele. Em seguida
empacota o `dist/build82.mcpb`, um bundle de extensão desktop
[MCPB](https://github.com/modelcontextprotocol/mcpb) (manifest versão 0.3) com um binário universal
de macOS (os dois builds darwin unidos em um único Mach-O pelo próprio script, já que o MCPB escolhe
o binário por sistema operacional, mas não por arquitetura de CPU), os dois binários Linux atrás de
um script de inicialização ([`scripts/release/build82-linux.sh`](../../scripts/release/build82-linux.sh),
que executa o que corresponde ao `uname -m`), o binário windows/amd64 (o Windows em Arm o executa
pela emulação x64; o binário nativo windows/arm64 é um asset separado) e os ícones de
`docs/img/icons/`. A lista de tools do manifesto é lida do próprio servidor. Por fim escreve
`dist/checksums.txt` (formato padrão `sha256sum`, cobrindo todos os binários e o bundle) e o
`dist/server.json`, o descritor do [MCP Registry](https://modelcontextprotocol.io/registry/)
(schema `2025-12-11`) que aponta para o bundle. O `server.json` embute o SHA-256 do bundle, então
não é versionado no repositório (está no `.gitignore`): a cópia a publicar é a gerada junto do
bundle que ela descreve e anexada à release.

Os assets da release se chamam `build82_<os>_<arch>` (`.exe` no Windows), mais o `build82.mcpb`, o
`checksums.txt`, a assinatura Sigstore dele, `checksums.txt.sigstore.json`, e o `server.json`.

### Checklist da release

1. Revise as [versões fixadas de ferramentas](#versões-fixadas-de-ferramentas) e mova as entradas
   de `[Unreleased]` do [`CHANGELOG.md`](../../CHANGELOG.md) para a nova versão.
2. Simule localmente: `go run ./scripts/release vX.Y.Z`, depois `./dist/build82_linux_amd64 --version`,
   `BUILD82_DIST_DIR="$PWD/dist" go test -run TestReleaseContract ./internal/selfupdate`,
   `mcp-publisher validate` (dentro de `dist/`) e `npx @anthropic-ai/mcpb validate` no bundle
   extraído.
3. Rode o smoke test com cliente real, [`scripts/smoke/claude.sh`](../../scripts/smoke/claude.sh):
   ele compila o build82, o registra numa única sessão `claude -p --strict-mcp-config` contra uma
   cópia de `testdata/moodle` (ou a raiz do Moodle passada como argumento), faz ao Claude Code
   perguntas em linguagem natural que exercitam as tools e falha quando uma resposta não traz o
   conteúdo esperado. Ele pega o que o cliente em memória não pega, como o que o Claude Code de fato
   mostra ao modelo.
4. Envie para a `main` **sem a tag** e espere o CI ficar verde nos três sistemas operacionais; se
   falhar, corrija com um commit novo (nada foi publicado).
5. Envie a tag `vX.Y.Z`. O [`release.yml`](../../.github/workflows/release.yml) roda cinco jobs:
   - `validate`: a tag precisa ser exatamente `vMAJOR.MINOR.PATCH`.
   - `checks`: as verificações do CI no Linux, no macOS e no Windows, sem cache do Go.
   - `build` (só leitura, sem token OIDC): `go run ./scripts/release <tag>`, a conferência da
     versão do binário Linux, o teste de contrato da release no `dist/` real,
     `mcp-publisher validate` e o upload do `dist/` como artefato do workflow (mantido por 7 dias).
   - `sign-publish` (o único job com `contents: write`, `id-token: write` e `attestations: write`;
     não faz checkout de código): baixa o artefato, roda `sha256sum --check --strict checksums.txt`,
     assina o `checksums.txt` sem chave com o cosign em `checksums.txt.sigstore.json`, verifica essa
     assinatura com a identidade exata que o `self-update` exige, registra atestações de
     proveniência para os binários e o bundle e cria a release no GitHub com
     `gh release create --generate-notes`.
   - `publish-registry`: publica o `server.json` anexado à release (veja abaixo).
6. Confira a release: `gh release view vX.Y.Z`, a busca no MCP Registry abaixo e, com os assets
   baixados, `sha256sum -c --ignore-missing checksums.txt`,
   `gh attestation verify build82_linux_amd64 --repo oito2/mcp-build82` e
   `cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json --certificate-identity https://github.com/oito2/mcp-build82/.github/workflows/release.yml@refs/tags/vX.Y.Z --certificate-oidc-issuer https://token.actions.githubusercontent.com`.
7. Rode o `build82 self-update` de ponta a ponta a partir da versão anterior, com e sem o cosign no
   `PATH`. Para exercitar um updater novo contra a release real, compile o código da tag carimbado
   com a versão anterior (`-ldflags "-X github.com/oito2/mcp-build82/internal/version.Current=vANTERIOR"`).

Um binário compilado sem o `-ldflags` de release reporta `"dev"` como sua versão — o `self-update`
trata isso como sempre desatualizado, o que é correto para uma build de desenvolvimento local, mas
significa que binários de release **precisam** passar por `scripts/release`, não um `go build`
simples.

### Publicando no MCP Registry

O job `publish-registry` do [`release.yml`](../../.github/workflows/release.yml) publica cada
release com tag automaticamente, com o CLI oficial
[`mcp-publisher`](https://modelcontextprotocol.io/registry/github-actions) (versão e SHA-256 do
tarball fixados em [`.github/actions/install-mcp-publisher`](../../.github/actions/install-mcp-publisher/action.yml)) autenticado via OIDC do GitHub Actions
(`mcp-publisher login github-oidc`, permissão `id-token: write`, sem segredo). Ele só roda depois
que a release existe, porque a entrada do registry aponta para o asset `build82.mcpb` da release e
para os ícones da tag. Se falhar, reexecute só esse job na página da execução do workflow.

Para publicar manualmente (por exemplo, depois de uma release manual), use a conta `oito2` do
GitHub, exigida pelo namespace `io.github.oito2/`:

```bash
gh release download vX.Y.Z --pattern server.json --dir /tmp/build82-vX.Y.Z
cd /tmp/build82-vX.Y.Z
mcp-publisher login github
mcp-publisher publish
```

O `mcp-publisher publish` lê o `server.json` do diretório atual. Publique a cópia anexada à release,
não uma gerada localmente: um build local só é idêntico byte a byte ao do CI quando usa o mesmo
patch do Go; caso contrário, o `fileSha256` dele não bateria com o bundle publicado. Confira o resultado com
`curl "https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.oito2/mcp-build82"`.

## Licença

Ao contribuir, você concorda que suas contribuições serão licenciadas sob a
[GNU General Public License v3.0](../../LICENSE), a mesma licença que cobre o restante do projeto.
