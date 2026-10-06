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
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
go build ./...
go test -race ./...
BUILD82_EXTRACTOR_BACKEND=treesitter go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Os sete precisam passar (a suíte de testes roda uma vez por backend de extração) — [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) os roda (no patch mais recente do Go 1.26.x, `ubuntu-24.04`) a
cada push e pull request contra `main` e bloqueia o merge caso contrário. O `golangci-lint` usa o
[`.golangci.yml`](../../.golangci.yml) do repositório (linters padrão, com as exclusões padrão
para erros não checados de `Close`/`fmt.Fprint*`) e precisa reportar `0 issues`; o CI fixa a mesma
versão. A flag `-race` exige cgo, ou seja, um compilador C funcional na sua máquina; sem um, use `go test ./...` localmente e confie no CI para a execução com race. Os testes exercitam
comportamento real sempre que praticável — eventos reais de `fsnotify` para o watcher, um par real
de cliente/servidor MCP em memória para as tools, binários reais compilados para os checks de
install/self-update/CLI — em vez de mockar o SDK ou o filesystem.

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
- `internal/config/`, `internal/cache/`, `internal/watcher/` — fundacionais, sem dependência de
  MCP.

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
```

Isso compila o `build82` para cada `GOOS`/`GOARCH` suportado (linux/amd64, linux/arm64,
darwin/amd64, darwin/arm64, windows/amd64) dentro de `dist/`, com
`-ldflags "-s -w -X github.com/oito2/mcp-build82/internal/version.Current=vX.Y.Z"`: o `-s -w`
remove a tabela de símbolos e as informações de depuração DWARF para reduzir os binários (os stack
traces de panic são mantidos), e o `-X` faz o binário compilado reportar a versão correta
(`build82 --version`) para que a comparação semver do `self-update` funcione corretamente contra ele. Em seguida empacota o `dist/build82.mcpb`, um bundle
de extensão desktop [MCPB](https://github.com/modelcontextprotocol/mcpb) (manifest versão 0.3) com
um binário universal de macOS (os dois builds darwin unidos em um único Mach-O pelo próprio script,
já que o MCPB escolhe o binário por sistema operacional, mas não por arquitetura de CPU), os dois
binários Linux atrás de um script de inicialização
([`scripts/release/build82-linux.sh`](../../scripts/release/build82-linux.sh), que executa o que
corresponde ao `uname -m`), o binário windows/amd64 e os ícones de `docs/img/icons/`. A lista de
tools do manifesto é lida do próprio servidor. Por fim escreve `dist/checksums.txt`
(formato padrão `sha256sum`, cobrindo todos os binários e o bundle) e o `dist/server.json`, o
descritor do [MCP Registry](https://modelcontextprotocol.io/registry/) (schema `2025-12-11`) que
aponta para o bundle. O `server.json` embute o SHA-256 do bundle, então não é versionado no
repositório (está no `.gitignore`): a única cópia válida é a gerada junto do bundle que ela descreve
e publicada como asset da release.

Os assets da release se chamam `build82_<os>_<arch>` (`.exe` no Windows), mais o `build82.mcpb`. Depois, siga um dos caminhos:

1. **Automático (caminho normal):** envie uma tag `vX.Y.Z` (`git tag vX.Y.Z && git push --tags`).
   O [`release.yml`](../../.github/workflows/release.yml) dispara em tags que casam com `v*.*.*` e
   roda dois jobs:
   - `release`: configura o patch mais recente do Go 1.26.x e roda as mesmas verificações do CI (`gofmt`, `go vet`,
     `golangci-lint`, `go build`, `go test -race` com o backend regex e com o tree-sitter, `govulncheck`), executa
     `go run ./scripts/release <tag>`, valida o `dist/server.json` com `mcp-publisher validate`
     (qualquer verificação que falhe interrompe a release) e cria a release no GitHub com
     `gh release create`, anexando tudo em `dist/` (binários, `build82.mcpb`, `checksums.txt` e
     `server.json`).
   - `publish-registry`: depois que o `release` termina com sucesso, baixa o `server.json` anexado
     à release e o publica no MCP Registry (veja
     [Publicando no MCP Registry](#publicando-no-mcp-registry)).
2. **Manual:** marque a release com uma tag e depois crie a release no GitHub você mesmo, enviando cada arquivo em
   `dist/` (incluindo `build82.mcpb`, `checksums.txt` e `server.json`), e publique no MCP Registry
   manualmente como descrito abaixo.

Um binário compilado sem o `-ldflags` acima reporta `"dev"` como sua versão — o `self-update` trata
isso como sempre desatualizado, o que é correto para uma build de desenvolvimento local, mas
significa que binários de release **precisam** passar por `scripts/release`, não um `go build`
simples.

### Publicando no MCP Registry

O job `publish-registry` do [`release.yml`](../../.github/workflows/release.yml) publica cada
release com tag automaticamente, com o CLI oficial
[`mcp-publisher`](https://modelcontextprotocol.io/registry/github-actions) (versão fixada em
`MCP_PUBLISHER_VERSION` no workflow) autenticado via OIDC do GitHub Actions
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
não uma gerada localmente: um build local não é idêntico byte a byte ao do CI, então o `fileSha256`
dele não bateria com o bundle publicado. Confira o resultado com
`curl "https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.oito2/mcp-build82"`.

## Licença

Ao contribuir, você concorda que suas contribuições serão licenciadas sob a
[GNU General Public License v3.0](../../LICENSE), a mesma licença que cobre o restante do projeto.
