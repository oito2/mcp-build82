🌐 [English](../../en/getting-started/installation.md) | **Português** | 🏠 [Índice](../index.md)

---

# Guia de Instalação

O **build82** é distribuído como um único binário estático — sem runtime, sem gerenciador de dependências, sem `node_modules`. Baixe um release, ou compile a partir do código-fonte se tiver Go instalado.

---

## 📋 Pré-requisitos

| Componente                        | Versão mínima          | Observação                              |
| :--------------------------------- | :---------------------- | :--------------------------------------- |
| Go (só para build do código-fonte) | 1.26                    | Desnecessário se você baixar um binário de release |
| Moodle                             | 4.1                     | Hook API requer Moodle 4.3+              |
| Sistema operacional                | Linux, macOS, Windows   | Binários prontos: Linux e macOS em amd64/arm64, Windows somente em amd64 |

Você também precisará de um **cliente MCP compatível** para interagir com o servidor. O comando `install` do build82 pode configurar automaticamente qualquer um destes que encontrar:

- [Claude Code](../guides/clients/claude-code.md)
- [Claude Desktop](../guides/clients/claude-desktop.md) (macOS, Windows e Linux beta)
- [Antigravity (IDE e CLI)](../guides/clients/antigravity.md)
- [OpenAI Codex](../guides/clients/codex.md)
- [OpenCode](../guides/clients/opencode.md)
- [Cursor](../guides/clients/cursor.md), [Zed](../guides/clients/zed.md) e [Cline](../guides/clients/cline.md) (extensão do VS Code e CLI)

---

## 🚀 Opção 1: Baixar um Binário de Release (Recomendado)

Não requer toolchain Go. Cada release publica um asset por plataforma, chamado `build82_<os>_<arch>` (com `.exe` no Windows):

| Asset | Plataforma |
| :--- | :--- |
| `build82_linux_amd64` | Linux, x86-64 |
| `build82_linux_arm64` | Linux, ARM64 |
| `build82_darwin_amd64` | macOS, Intel |
| `build82_darwin_arm64` | macOS, Apple Silicon |
| `build82_windows_amd64.exe` | Windows, x86-64 |
| `build82_windows_arm64.exe` | Windows, ARM64 |
| `build82.mcpb` | Bundle de extensão do Claude Desktop: macOS (universal), Windows x86-64, Linux x86-64 e arm64 — veja [Claude Desktop](../guides/clients/claude-desktop.md#extensão-desktop-mcpb) |

Todo release também traz um `checksums.txt` (SHA-256, uma linha `<hash>  <nome do asset>` por asset) e
um descritor `server.json` do MCP Registry. Sempre verifique o download contra o `checksums.txt` antes
de confiar nele. Escolha seu sistema
operacional abaixo.

### 🐧 Linux

```bash
# amd64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_amd64

# arm64
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_linux_arm64
```

Verifique o checksum **antes** de instalar — o `checksums.txt` lista o nome do arquivo do release, então rode isto na pasta do download enquanto o arquivo ainda tem esse nome (deve imprimir `OK` para o seu arquivo):

```bash
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

Somente se imprimiu `OK`, instale o binário que você baixou:

```bash
# amd64
chmod +x build82_linux_amd64
sudo mv build82_linux_amd64 /usr/local/bin/build82

# arm64
chmod +x build82_linux_arm64
sudo mv build82_linux_arm64 /usr/local/bin/build82
```

`/usr/local/bin` já está no `PATH` por padrão em praticamente toda distribuição, então confirme
que a instalação funcionou:

```bash
build82 --version
```

Se preferir não usar `sudo`, mova o binário para qualquer diretório já presente no seu `PATH` de
usuário (ex.: `~/.local/bin`, `~/bin`).

### 🍎 macOS

Os binários são publicados separadamente para Apple Silicon e Intel — verifique seu chip com
`uname -m` (`arm64` = Apple Silicon M1/M2/M3/M4, `x86_64` = Intel):

```bash
# Apple Silicon (M1/M2/M3/M4)
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_darwin_arm64

# Intel
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/build82_darwin_amd64
```

Verifique o checksum **antes** de instalar (o macOS traz `shasum`, não `sha256sum`) — o `checksums.txt` lista o nome do arquivo do release, então rode isto na pasta do download enquanto o arquivo ainda tem esse nome (deve imprimir `OK` para o seu arquivo):

```bash
curl -LO https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
```

Somente se imprimiu `OK`, instale o binário que você baixou:

```bash
# Apple Silicon (M1/M2/M3/M4)
chmod +x build82_darwin_arm64
sudo mv build82_darwin_arm64 /usr/local/bin/build82

# Intel
chmod +x build82_darwin_amd64
sudo mv build82_darwin_amd64 /usr/local/bin/build82
```

> **Aviso do Gatekeeper:** os binários de release não são notarizados pela Apple, então o macOS vai
> recusar rodar o binário na primeira vez, informando que ele "não pôde ser aberto porque o
> desenvolvedor não pôde ser verificado". Remova o atributo de quarentena antes de rodar:
> ```bash
> xattr -d com.apple.quarantine /usr/local/bin/build82
> ```
> Se isso for bloqueado pela política de MDM da sua organização, você pode aprovar manualmente uma
> vez em **Ajustes do Sistema → Privacidade e Segurança → Segurança**, na mensagem sobre o app
> bloqueado.

Confirme que a instalação funcionou:

```bash
build82 --version
```

### 🪟 Windows

Baixe o binário `amd64` com o PowerShell (no Windows em Arm, use `build82_windows_arm64.exe` em todos os comandos):

```powershell
Invoke-WebRequest -Uri "https://github.com/oito2/mcp-build82/releases/latest/download/build82_windows_amd64.exe" -OutFile "build82_windows_amd64.exe"
```

Verifique o checksum **antes** de instalar — o `Get-FileHash` nativo do PowerShell substitui o `sha256sum`:

```powershell
Invoke-WebRequest -Uri "https://github.com/oito2/mcp-build82/releases/latest/download/checksums.txt" -OutFile "checksums.txt"
Get-FileHash .\build82_windows_amd64.exe -Algorithm SHA256
```

Compare o valor `Hash` impresso com a linha correspondente de `build82_windows_amd64.exe` no
`checksums.txt`; continue somente se forem iguais.

> **Aviso do SmartScreen:** os binários de release não são assinados digitalmente, então o Windows
> Defender SmartScreen pode bloquear a primeira execução com "O Windows protegeu o computador".
> Clique em **Mais informações → Executar assim mesmo**, ou desbloqueie o arquivo antes para que o
> aviso nem apareça:
> ```powershell
> Unblock-File .\build82_windows_amd64.exe
> ```

Mova para um local permanente e adicione essa pasta ao seu `PATH` de usuário (não precisa de
privilégios de administrador):

```powershell
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\build82"
Move-Item -Force .\build82_windows_amd64.exe "$env:LOCALAPPDATA\build82\build82.exe"
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$env:LOCALAPPDATA\build82", "User")
```

Feche e reabra o terminal (para o `PATH` atualizado ter efeito) e confirme que a instalação
funcionou:

```powershell
build82 --version
```

> **Usuários de WSL:** se você roda o Moodle dentro do WSL, siga as instruções de **Linux** acima
> dentro da sua distribuição WSL — um `.exe` do Windows não roda lá.

---

### 🔏 Verificando a assinatura e a proveniência (opcional)

A partir da v1.1.0, toda release também traz o `checksums.txt.sigstore.json`, uma assinatura
[Sigstore](https://www.sigstore.dev/) sem chave do `checksums.txt` feita pelo workflow de release
deste repositório, e atestações de proveniência de build do GitHub para cada binário e para o
`build82.mcpb`. Com o [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 ou
mais novo, confira que o `checksums.txt` foi gerado pelo workflow de release daquela tag exata
(troque `vX.Y.Z`):

```bash
curl -LO https://github.com/oito2/mcp-build82/releases/download/vX.Y.Z/checksums.txt
curl -LO https://github.com/oito2/mcp-build82/releases/download/vX.Y.Z/checksums.txt.sigstore.json
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-identity https://github.com/oito2/mcp-build82/.github/workflows/release.yml@refs/tags/vX.Y.Z \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Com o [GitHub CLI](https://cli.github.com/), confira a proveniência de um binário baixado:

```bash
gh attestation verify build82_linux_amd64 --repo oito2/mcp-build82
```

O `build82 self-update` faz a mesma verificação com o cosign automaticamente quando ele está no
`PATH`.

---

## 🛠️ Opção 2: Compilar a Partir do Código-Fonte (Go 1.26+)

Use este método se pretende contribuir com o projeto ou quer o código ainda não lançado em release. Requer Go 1.26 ou mais recente (a diretiva `go` do `go.mod`). O caminho do módulo é `github.com/oito2/mcp-build82` e o ponto de entrada é `cmd/build82`.

```bash
go install github.com/oito2/mcp-build82/cmd/build82@latest
```

Isso instala `build82` em `$(go env GOPATH)/bin` — garanta que esse diretório esteja no seu `PATH`.

Ou clone e compile manualmente:

```bash
git clone https://github.com/oito2/mcp-build82.git
cd mcp-build82
go build -o build82 ./cmd/build82
```

`go build` já produz o binário final, pronto para executar (no Windows, use `-o build82.exe`). Rode a suíte de testes com:

```bash
go test ./...
```

Veja [Contribuindo](../contribuindo.md) para o fluxo de desenvolvimento completo.

---

## ⚙️ Configurando o Caminho do Moodle

O build82 persiste sua configuração num arquivo pequeno em `~/.build82` assim que você roda `init_moodle_context` — você não precisa definir nada manualmente no dia a dia. Variáveis de ambiente continuam disponíveis e têm precedência, o que é útil em CI ou para sobrepor a configuração pontualmente. As três variáveis abaixo cobrem a localização do Moodle; a lista completa (incluindo `BUILD82_EXTRACTOR_BACKEND` e `BUILD82_TOKEN`) está na [Referência de Configuração](../reference/configuration.md):

| Variável                       | Obrigatória | Descrição                                                                          |
| :------------------------------ | :---------: | :----------------------------------------------------------------------------------- |
| `BUILD82_MOODLE_PATH`          |   ❌ Não    | Caminho absoluto para a raiz do Moodle. Sobrepõe o arquivo de config salvo quando definida. |
| `BUILD82_MOODLE_VERSION`       |   ❌ Não    | Versão do Moodle (ex: `4.5`), lida só com `BUILD82_MOODLE_PATH`. Ver observação abaixo. |
| `BUILD82_MOODLE_FULLVERSION`   |   ❌ Não    | Número de build numérico do Moodle: o `$version` do `version.php` (ex: `2024100700`). |

### Sobre o `BUILD82_MOODLE_VERSION`

`init_moodle_context` e `update_indexes` sempre detectam a versão a partir do `version.php` para os índices que geram. As demais tools, resources e prompts leem a versão da configuração resolvida, e essa configuração vem de **uma única fonte**: quando `BUILD82_MOODLE_PATH` está definida (o `build82 install` sempre a define na entrada do cliente), caminho, versão e versão completa vêm todos do ambiente — `BUILD82_MOODLE_VERSION` e `BUILD82_MOODLE_FULLVERSION` ficam vazias quando não definidas, e os valores de `~/.build82` nunca são mesclados. Defina-as somente se quiser que essas tools conheçam a versão do Moodle ao rodar com `BUILD82_MOODLE_PATH`. Veja a [Referência de Configuração](../reference/configuration.md#precedência).

---

## 🔌 Registrando o Servidor MCP

O build82 pode configurar seu cliente MCP para você:

```bash
build82 install [target]
```

Execute sem um target para detectar todo cliente suportado encontrado na sua máquina (ele lista os clientes e pede confirmação), ou passe um explicitamente (`claude`, `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`); um target explícito que não seja detectado é ignorado. Ele pergunta o caminho do Moodle e escreve a configuração específica do cliente sozinho: para `claude` e `codex` executa o próprio comando `mcp add` da ferramenta, para os demais mescla uma entrada `build82` no arquivo de configuração JSON do cliente mantendo as demais chaves na ordem original (um arquivo com comentários ou vírgulas finais nunca é reescrito; o comando imprime `<Rótulo>... manual step needed:` com o trecho para colar manualmente e sai com código 1). **Feche cada cliente antes de rodar o `install`**: ele edita um arquivo que o cliente também grava, e um cliente que salva as configurações enquanto o comando roda pode sobrescrever a mudança. Cada cliente imprime `<Rótulo>... configured.`, ou `<Rótulo>... updated.` quando o build82 já estava registrado nele: executar `build82 install` de novo substitui o registro, atualizando o caminho do binário e o `BUILD82_MOODLE_PATH`. Os locais exatos e os detalhes por cliente estão nos [guias de cliente](../guides/clients/claude-code.md), que também cobrem a configuração manual.

Para remover o registro depois:

```bash
build82 uninstall [target]
```

Isso remove apenas o registro do `build82` no cliente. Com `--purge`, também apaga o arquivo de config `~/.build82`, os arquivos globais gerados, e os arquivos `PLUGIN_*.md` e o marcador `.indevelopment` dos plugins de desenvolvimento marcados, após uma confirmação separada. Ele deixa `.build82/tags` e `.build82/.cache.json` no lugar. O binário em si não é removido. Veja [Desinstalação](./uninstallation.md) para a lista completa e a limpeza manual.

---

## 🔍 Verificando a Instalação

Depois de instalar e registrar o cliente MCP, verifique se o servidor está funcionando pedindo ao seu assistente de IA:

```
Run the build82 doctor
```

A IA chamará a tool `doctor` via MCP e retornará um relatório: versão do Moodle detectada, caminho configurado, atualidade dos índices, arquivos legados pendentes de migração e estatísticas de cache.

> **Nota:** `doctor` é uma tool MCP, não um comando de CLI — só roda dentro de uma sessão ativa com um cliente compatível.

---

## ⬆️ Mantendo o build82 Atualizado

```bash
build82 self-update --check   # relata se existe um release mais novo (saída 10 se houver), sem instalar
build82 self-update           # baixa, verifica e instala o release mais recente
build82 self-update --require-signature # recusa atualizar se o cosign não puder verificar a assinatura do release
build82 self-update --rollback # restaura o binário anterior caso a nova versão se revele quebrada
```

Os downloads vêm apenas de `https://github.com/oito2/mcp-build82/releases/download/<tag>/`, e os redirecionamentos só vão para os hosts de assets de release do GitHub. Quando o [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 ou mais novo está no `PATH`, o `self-update` verifica primeiro a assinatura Sigstore do `checksums.txt` do release; sem ele, um aviso é impresso e só o checksum é conferido (o `--require-signature` transforma isso em erro, antes de qualquer download). Depois, todo download tem o checksum verificado contra o `checksums.txt` e passa por um smoke test antes que o binário em execução seja substituído; o novo binário mantém as permissões do que ele substitui, e o binário anterior é mantido ao lado como `<caminho>.bak`. A sequência completa está na [referência da CLI](../reference/cli.md#self-update).

Se uma nova versão passa nesse smoke test mas se revela quebrada no uso real depois, o `build82 self-update --rollback` faz um smoke test do backup `.bak` e, se ele rodar, o troca com o binário atual por meio de renames: o backup assume o lugar do binário e o binário substituído vira o novo `.bak`, então rodar `--rollback` de novo desfaz a troca. Se não houver backup `.bak`, ou se ele não rodar, falha com um erro claro e deixa o binário atual intocado.

---

## ➡️ Próximos passos

Com o servidor instalado, configure seu cliente MCP:

- [Configurar Claude Code](../guides/clients/claude-code.md)
- [Configurar Claude Desktop](../guides/clients/claude-desktop.md)
- [Configurar Antigravity (IDE e CLI)](../guides/clients/antigravity.md)
- [Configurar OpenAI Codex](../guides/clients/codex.md)
- [Configurar OpenCode](../guides/clients/opencode.md)

Ou pule direto para o uso:

- [Quickstart](./quickstart.md)
- [Voltar ao Índice](../index.md)
