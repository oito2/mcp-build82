🌐 [English](../../en/concepts/why-build82.md) | **Português** | 🏠 [Índice](../index.md)

---

# Por que usar o build82?

Assistentes de IA são poderosos — mas sem contexto, eles trabalham no escuro. Este documento explica o problema que o `build82` resolve e quando faz sentido usá-lo.

---

## O problema: IA sem contexto do Moodle

Quando você pede para um assistente de IA ajudar com desenvolvimento Moodle sem nenhuma integração, ele depende exclusivamente do seu conhecimento de treinamento — que é genérico, pode estar desatualizado e não conhece nada sobre **a sua instalação**.

Na prática, isso significa:

**❌ Contexto manual e repetitivo**
A cada sessão, você precisa copiar e colar arquivos PHP no chat, explicar a estrutura do banco de dados, descrever quais plugins estão instalados e qual versão do Moodle está em uso. Qualquer mudança na instalação invalida o contexto que você construiu.

**❌ Código genérico e fora do padrão**
Sem conhecer a versão exata do Moodle, a IA pode sugerir APIs depreciadas, usar funções que não existem na sua versão ou gerar código que ignora convenções específicas da sua instalação — como prefixos de tabelas, namespaces de plugins já existentes ou capabilities já definidas.

**❌ Risco de regressões**
A IA não sabe o que já existe. Ela pode sugerir criar uma tabela que já foi criada, um evento que já tem um observer registrado ou uma capability com nome conflitante.

---

## A solução: contexto real, automático e persistente

O `build82` resolve esses problemas expondo a estrutura real da sua instalação Moodle diretamente ao assistente de IA via protocolo MCP.

**✅ A IA conhece a sua instalação, não um Moodle genérico**
O servidor detecta automaticamente a versão do Moodle, mapeia todos os plugins instalados, lê o schema real do banco de dados e indexa as APIs do core. O assistente trabalha com dados reais, não suposições.

**✅ Acesso direto à estrutura do banco de dados**
Antes de sugerir qualquer query ou alteração de schema, a IA consulta o `install.xml` real dos seus plugins. Ela sabe quais tabelas existem, quais campos têm e quais índices estão definidos.

**✅ Scaffolding alinhado com os padrões do Moodle**
Ao criar novos plugins, o servidor injeta automaticamente as convenções da sua versão do Moodle — namespaces corretos, estrutura de diretórios esperada, formato de `version.php` e padrões de nomenclatura de capabilities e eventos.

**✅ Revisão de código contextual**
Ao revisar um plugin existente, a IA lê o contexto completo gerado pelo servidor — funções definidas, callbacks registrados, dependências declaradas — e identifica problemas reais em vez de problemas hipotéticos.

**✅ Zero configuração no Moodle**
Diferente de Web Services, não é necessário criar tokens de API, configurar usuários de serviço ou expor endpoints. O servidor lê diretamente os arquivos locais do sistema de arquivos.

**✅ Um único binário estático, sem nada extra para instalar**
O build82 é escrito em Go e distribuído como um binário autocontido por plataforma. Sem runtime, sem `node_modules`, sem gerenciador de versão — baixe, ou instale com `go install`, e ele simplesmente roda. Ele até se atualiza sozinho (`build82 self-update`).

---

## Comparação direta

| Situação            | Sem build82                                          | Com build82                                                  |
| -------------------- | ----------------------------------------------------------- | --------------------------------------------------------------------- |
| Versão do Moodle    | IA não sabe — você precisa informar                          | Detectada automaticamente de `version.php`                            |
| Estrutura do banco  | Você cola o `install.xml` no chat                            | Indexada automaticamente em `PLUGIN_DB_TABLES.md`                     |
| APIs do core        | IA usa conhecimento de treinamento (pode estar desatualizado) | Indexadas em `MOODLE_API_INDEX.md` com visibilidade e `@since`        |
| Plugins existentes  | Você descreve manualmente                                    | Mapeados em `MOODLE_PLUGIN_INDEX.md`                                  |
| Padrões de código   | IA usa padrões genéricos PHP                                  | Contexto com as convenções específicas do Moodle e da sua instalação  |
| Dependência de runtime | —                                                          | Nenhuma — binário estático único, sem Node.js/npm                     |

---

## Quando usar

O `build82` é ideal para:

| Cenário                                | Por quê o servidor ajuda                                              |
| ---------------------------------------- | -------------------------------------------------------------------------- |
| **Desenvolvimento de novos plugins**    | Scaffold gerado com padrões reais da sua versão do Moodle                  |
| **Manutenção de plugins existentes**    | IA lê o contexto completo antes de sugerir alterações                     |
| **Debugging de integrações**            | IA conhece eventos, callbacks e dependências registradas                  |
| **Revisão de código antes de commits**  | Análise contextual com base na estrutura real do plugin                   |
| **Onboarding em projetos legados**      | IA gera resumo completo de arquitetura, banco e fluxo de execução         |
| **Busca na API do core**                | Pesquisa indexada com visibilidade, assinatura e arquivo fonte            |

---

## Quando não usar

O `build82` **não é a ferramenta certa** para:

- **Administração de instâncias Moodle** — para gerenciar cursos, usuários e matrículas via IA, existem servidores MCP baseados na REST API do Moodle, que são complementares a este.
- **Ambientes de produção** — o servidor escreve arquivos `.md` de contexto dentro dos diretórios de plugin (em `.build82/`). Use exclusivamente em ambientes de desenvolvimento.
- **Substituir testes automatizados** — o servidor dá contexto à IA, mas não substitui PHPUnit ou Behat para garantir a correção do código.

---

## Leitura adicional

- [O que é MCP?](./what-is-mcp.md)
- [Como o build82 funciona](./how-build82-works.md)
- [Glossário](./glossary.md)

---

[🏠 Voltar ao Índice](../index.md)
