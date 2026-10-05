🌐 [English](../en/prompts.md) | **Português** | 🏠 [Índice](./index.md)

---

# Prompts de Exemplo

Exemplos de pedidos que você pode digitar para o seu agente de IA (Claude Code, Codex, Gemini, ...) para que ele use o build82. Escreva em linguagem natural: você nunca precisa citar o nome de uma ferramenta nem escrever JSON. O agente escolhe a ferramenta, o prompt MCP ou o recurso certo do build82 a partir do que você pedir.

Troque `local_myplugin`, `local_relatorios` e nomes parecidos pelos seus próprios plugins. A redação é livre; os exemplos só mostram quais informações incluir.

**Pré-requisito para quase tudo:** o build82 precisa ter sido inicializado uma vez para a sua instalação do Moodle (veja [Inicializar o build82](#inicializar-o-build82)). Os plugins acompanhados pelo watcher e pelo modo `dev` do lote precisam estar marcados com `.indevelopment`.

## Categorias

- [Configuração e Manutenção](#configuração-e-manutenção)
- [Análise e Refatoração](#análise-e-refatoração)
- [Automação e Geração](#automação-e-geração)
- [Testes e Validação](#testes-e-validação)
- [Trabalhando Entre Sessões](#trabalhando-entre-sessões)

---

## Configuração e Manutenção

### Inicializar o build82

- **Parâmetros esperados:** caminho absoluto da raiz do Moodle. Diga também se quer reinicializar uma instalação que já está configurada.
- **Exemplo:**
  > Configure o build82 para a minha instalação do Moodle em /var/www/moodle.
- **Saída esperada:** o agente chama `init_moodle_context`. Ele valida o caminho, detecta a versão do Moodle, salva a configuração e gera todos os arquivos de índice globais. Você recebe um resumo da versão detectada e dos arquivos criados. Com um pedido de "force" ("inicialize de novo do zero"), reinicializa mesmo que já exista configuração.

### Atualizar os índices globais

- **Parâmetros esperados:** nenhum. Opcionalmente, diga para incluir seus plugins em desenvolvimento ou para ignorar o cache.
- **Exemplo:**
  > Acabei de atualizar o Moodle. Atualize todos os índices do build82, incluindo meus plugins em desenvolvimento, e ignore qualquer cache.
- **Saída esperada:** o agente chama `update_indexes` (com regeneração de plugins e bypass do cache quando pedido). Ele redetecta a versão do Moodle e regenera os 13 arquivos de índice globais; a resposta em texto traz a versão do Moodle e as contagens de índices globais regenerados, reaproveitados do cache e com falha (mais uma linha por plugin em desenvolvimento, quando incluídos); peça JSON para obter as listas de arquivos. Quando você pede para ignorar o cache, todo arquivo global (e todo arquivo dos plugins em desenvolvimento, se pedido) é realmente reescrito, mesmo que seus fontes não tenham mudado.

### Acompanhar plugins enquanto você programa

- **Parâmetros esperados:** os plugins já devem estar marcados com `.indevelopment` (veja [Marcar um plugin como em desenvolvimento](#marcar-um-plugin-como-em-desenvolvimento)).
- **Exemplo:**
  > Comece a acompanhar meus plugins em desenvolvimento para que o contexto deles fique atualizado enquanto eu edito. Depois me diga se o watcher ainda está rodando e pare-o quando eu pedir.
- **Saída esperada:** o agente chama `watch_plugins` com a ação de iniciar, consultar status ou parar. Enquanto roda, mudanças nos plugins em desenvolvimento regeneram o contexto automaticamente, e todas as sessões conectadas são notificadas quando uma regeneração termina. Se nenhum plugin estiver marcado como `.indevelopment` (ou nenhum tiver arquivos monitoráveis), o watcher não é iniciado e você é avisado; marque um plugin antes e peça de novo.

### Marcar um plugin como em desenvolvimento

- **Parâmetros esperados:** o componente ou caminho do plugin.
- **Exemplo:**
  > Gere o contexto de local_myplugin e continue tratando-o como um plugin que estou desenvolvendo ativamente.
- **Saída esperada:** `generate_plugin_context` marca o plugin como `.indevelopment` como efeito colateral. A partir daí, `watch_plugins`, o modo `dev` do lote, `list_dev_plugins` e `doctor` passam a incluí-lo. Você também pode criar o marcador manualmente com `mkdir -p <moodle>/local/myplugin/.build82 && touch <moodle>/local/myplugin/.build82/.indevelopment`.

---

## Análise e Refatoração

### Encontrar um plugin

- **Parâmetros esperados:** qualquer termo de busca (parte do nome do componente, um tipo de plugin, um nome). Opcionalmente, um número máximo de resultados.
- **Exemplo:**
  > Quais plugins instalados têm "report" no nome? Mostre só os 10 primeiros.
- **Saída esperada:** o agente chama `search_plugins`. Você recebe os plugins encontrados com componente, tipo, nome, versão e caminho (usa busca aproximada quando não há correspondência exata).

### Consultar uma função da API do Moodle

- **Parâmetros esperados:** um nome de função ou uma palavra-chave da descrição. Diga se quer também as funções obsoletas, ou só elas.
- **Exemplo:**
  > Existe uma função do core para formatar uma data no fuso horário do usuário? Diga também se há algo parecido que esteja obsoleto.
- **Saída esperada:** o agente chama `search_api` (visibilidade `public`, `deprecated` ou `all`). Você recebe as funções de `lib/` encontradas, com resumo e arquivo de origem, para que o agente não invente APIs.

### Ver os detalhes de um plugin

- **Parâmetros esperados:** o plugin (componente, caminho relativo ou caminho absoluto).
- **Exemplo:**
  > Me dê os detalhes de local_myplugin: versão, dependências, o que ele declara.
- **Saída esperada:** o agente chama `get_plugin_info`. Você recebe o contexto de IA gerado do plugin, ou metadados detectados na hora se o contexto nunca foi gerado.

### Listar os plugins que estou desenvolvendo

- **Parâmetros esperados:** nenhum.
- **Exemplo:**
  > Quais plugins estou desenvolvendo agora?
- **Saída esperada:** o agente chama `list_dev_plugins` e lista todos os plugins marcados com `.indevelopment`.

### Entender um plugin rapidamente

- **Parâmetros esperados:** o plugin (componente como `local_myplugin`, caminho relativo à raiz do Moodle como `local/myplugin` ou caminho absoluto) e, opcionalmente, o aspecto que interessa: visão geral, banco de dados, eventos, classes, serviços ou fluxo.
- **Exemplo:**
  > Explique o local_myplugin para mim, mas só as tabelas do banco de dados.
  >
  > Me dê uma visão geral curta do mod_checklist e depois me conduza pelo fluxo de execução dele.
- **Saída esperada:** o agente chama `explain_plugin` com a seção correspondente (ou todas). Você recebe uma explicação compacta, mais barata de ler que os arquivos de índice completos.

### Revisar um plugin

- **Parâmetros esperados:** o plugin e, opcionalmente, um foco (segurança, desempenho, padrões de código, banco de dados, APIs do core ou tudo; um foco não reconhecido é rejeitado com um erro que lista os valores aceitos) e arquivos específicos.
- **Exemplo:**
  > Faça uma revisão completa de código do local_myplugin com foco em segurança: verificações de capability ausentes, saída sem escape, parâmetros sem validação e SQL direto.
- **Saída esperada:** o agente usa o prompt `review_plugin` (foco `security`) e carrega o contexto do plugin. Você recebe um relatório de achados organizado pela lista de critérios daquele foco, com uma correção sugerida para cada problema.

### Revisar apenas arquivos específicos

- **Parâmetros esperados:** o plugin e a lista de arquivos.
- **Exemplo:**
  > Revise apenas lib.php e classes/external/api.php do local_myplugin quanto aos padrões de código.
- **Saída esperada:** o prompt `review_plugin` com a lista `files` e o foco `standards`; a revisão fica restrita a esses arquivos.

### Revisar o uso do banco de dados

- **Parâmetros esperados:** o plugin.
- **Exemplo:**
  > Revise as interações com o banco de dados do local_myplugin. Procure índices ausentes, consultas ineficientes e lugares onde get_records_sql é usado no lugar de get_records.
- **Saída esperada:** `review_plugin` com foco `database`, usando o contexto de IA do plugin (`PLUGIN_AI_CONTEXT.md`, quando gerado), os critérios de revisão de banco de dados e o código que o agente lê. Você recebe os problemas agrupados por tabela ou consulta.

### Encontrar problemas de desempenho

- **Parâmetros esperados:** o plugin.
- **Exemplo:**
  > Analise o local_myplugin em busca de problemas de desempenho: consultas N+1, consultas IN sem get_in_or_equal, colunas de WHERE sem índice e consultas caras sem cache.
- **Saída esperada:** `review_plugin` com foco `performance`. Você recebe uma lista priorizada de achados com referências ao código.

### Verificar o uso das APIs do core

- **Parâmetros esperados:** o plugin.
- **Exemplo:**
  > Verifique se o local_myplugin usa corretamente as APIs do core do Moodle e se usa alguma função obsoleta.
- **Saída esperada:** `review_plugin` com foco `apis`, combinado com `search_api` (visibilidade obsoleta) quando necessário. Você recebe achados de uso incorreto e de obsolescência, com as substituições.

### Depurar um erro de plugin

- **Parâmetros esperados:** o plugin, a mensagem de erro completa ou o stack trace, e quando ele ocorre.
- **Exemplo:**
  > O local_myplugin lança "Table 'mdl_local_myplugin_data' doesn't exist" quando um professor abre a página de configurações. Qual é a causa raiz e como corrijo?
- **Saída esperada:** o agente usa o prompt `debug_plugin` com plugin, erro e contexto. As dicas específicas do Moodle são escolhidas por palavras-chave do erro (capability, banco de dados, classe não encontrada, evento/observer, tarefa/cron, web service/AJAX), ou dicas genéricas quando nada corresponde. Você recebe um diagnóstico e uma correção proposta.

### Depurar erro de tarefa, classe ou permissão

- **Parâmetros esperados:** o plugin e o texto exato do erro.
- **Exemplo:**
  > A tarefa agendada \local_myplugin\task\cleanup_task falha com "Permission denied" nas execuções do cron. Que capability ou permissão de arquivo pode estar causando isso?
  >
  > O local_myplugin dispara "Call to undefined method local_myplugin\output\renderer::render_summary()". Onde esse método deveria estar definido segundo a estrutura do plugin?
- **Saída esperada:** `debug_plugin` mais o contexto de estrutura e dependências do plugin (recursos `moodle://plugin/{component}/structure` e `/dependencies`). Você recebe o local provável do problema e como corrigi-lo.

### Rastrear o fluxo de execução

- **Parâmetros esperados:** o plugin e o cenário.
- **Exemplo:**
  > Rastreie o que acontece no local_myplugin, a partir dos pontos de entrada principais, quando um professor abre a página principal do plugin.
- **Saída esperada:** `explain_plugin` com a seção `flow`, ou o recurso `moodle://plugin/{component}/flow`. Você recebe os pontos de entrada e o caminho de execução passo a passo.

### Migrar callbacks legados para a Hook API

- **Parâmetros esperados:** o plugin e a versão alvo do Moodle (4.3 ou superior).
- **Exemplo:**
  > Verifique se o local_myplugin tem callbacks legados no lib.php que foram substituídos pela Hook API no Moodle 4.3+ e mostre os passos de migração de cada um.
- **Saída esperada:** o agente lê os callbacks e o contexto de hooks (recurso `/callbacks`), consulta o guia de plugins (`moodle://plugin-guide`) e `search_api` se preciso. Você recebe a lista de callbacks legados com o hook substituto e as mudanças de código.

### Ler os índices de referência do Moodle

- **Parâmetros esperados:** qual índice você quer (API, eventos, tarefas, serviços, tabelas, classes, capabilities, plugins, regras de código, guia de plugins).
- **Exemplo:**
  > Quais são as regras de código e de segurança do Moodle que devo seguir aqui?
  >
  > Quais tarefas agendadas e observers de eventos já existem na minha instalação?
- **Saída esperada:** o agente lê os recursos globais (`moodle://dev-rules`, `moodle://tasks-index`, `moodle://events-index` e os demais, como `moodle://api-index`, `moodle://db-tables`, `moodle://classes-index`, `moodle://capabilities-index`, `moodle://plugin-index`, `moodle://services-index`, `moodle://workspace`, `moodle://index`, `moodle://context`) e responde a partir deles.

### Listar os plugins que já têm contexto de IA

- **Parâmetros esperados:** nenhum; os plugins só aparecem depois que o contexto deles foi gerado (`.build82/PLUGIN_AI_CONTEXT.md` existe).
- **Exemplo:**
  > Quais plugins do meu Moodle já têm o contexto de IA do build82 gerado?
- **Saída esperada:** o agente lê o recurso `moodle://plugins/with-context`. Você recebe uma tabela com componente, tipo e caminho de cada plugin com contexto gerado, ordenada por componente.

### Consultar os metadados e as configurações administrativas de um plugin

- **Parâmetros esperados:** o componente do plugin (ex.: `local_myplugin`); o contexto dele já deve ter sido gerado.
- **Exemplo:**
  > Qual versão, requisito de Moodle e maturidade o local_myplugin declara, e quais configurações administrativas ele tem?
- **Saída esperada:** o agente lê `moodle://plugin/{component}/context` (metadados e contagens de recursos) e `moodle://plugin/{component}/settings` (configurações declaradas em `settings.php`). Você recebe os metadados do plugin e a lista das configurações administrativas.

### Explorar as classes e os endpoints de um plugin

- **Parâmetros esperados:** o componente do plugin; o contexto dele já deve ter sido gerado.
- **Exemplo:**
  > Como as classes do local_myplugin estão organizadas, e quais web services, endpoints AJAX e módulos AMD ele expõe?
- **Saída esperada:** o agente lê `moodle://plugin/{component}/architecture` (classes agrupadas por diretório) e `moodle://plugin/{component}/endpoints` (web services, endpoints AJAX e módulos AMD). Você recebe uma visão geral da organização das classes e dos pontos de entrada do plugin para clientes.

---

## Automação e Geração

### Criar o esqueleto de um plugin

- **Parâmetros esperados:** tipo do plugin (local, mod, block, ...), nome (letras minúsculas, dígitos e sublinhados; deve começar com letra). Opcionalmente um nome de exibição, as funcionalidades a criar como stubs (database, tasks, services, events, capabilities, settings), a versão mínima do Moodle e a maturidade. O diretório de destino não pode existir.
- **Exemplo:**
  > Crie o esqueleto de um plugin local chamado relatorios, com stubs para tabelas de banco, tarefas agendadas e capabilities. Dê o nome "Relatórios Personalizados" e marque como beta.
- **Saída esperada:** o agente chama `create_plugin_skeleton`. Ele escreve `version.php`, `lang/en/local_relatorios.php`, os arquivos de entrada obrigatórios do tipo e os stubs `db/*.php` das funcionalidades pedidas. Nenhuma lógica de negócio é gerada, e ele se recusa a sobrescrever um plugin existente.

### Criar um plugin completo com rascunho da IA

- **Parâmetros esperados:** tipo, nome, o que o plugin faz e as funcionalidades necessárias (tabelas, tarefas agendadas, web services, eventos, capabilities, configurações). Funciona melhor com um esqueleto criado antes.
- **Exemplo:**
  > Crie o scaffold completo de um plugin local do Moodle 4.4 chamado local_relatorios, que gera relatórios de frequência e participação. Precisa de uma tabela de configuração por curso e de uma tabela de cache com expiração, uma tarefa agendada que reconstrói o cache expirado a cada 6 horas, a capability local/relatorios:viewreports para professores, um web service que devolve o relatório em JSON e um observer para eventos course_viewed. Siga os padrões de código do Moodle.
- **Saída esperada:** o agente usa o prompt `scaffold_plugin` (tipo, nome, descrição, funcionalidades), que injeta o contexto do Moodle e um exemplo prático, e muitas vezes executa `create_plugin_skeleton` antes. Você recebe todos os arquivos necessários escritos com a estrutura e as convenções corretas do Moodle.

### Criar um módulo de atividade

- **Parâmetros esperados:** nome do módulo, finalidade, tabelas, capabilities e as páginas necessárias.
- **Exemplo:**
  > Crie o scaffold de um módulo de atividade do Moodle 4.4 chamado mod_checklist, em que professores criam checklists que os alunos completam. Inclua os callbacks padrão do lib.php, uma tabela checklist_item, capabilities para alunos enviarem e professores gerenciarem, e uma página de visualização com acompanhamento de conclusão.
- **Saída esperada:** `scaffold_plugin` com tipo `mod`, mais `create_plugin_skeleton` para os arquivos obrigatórios de mod. Você recebe a estrutura completa do módulo.

### Criar um bloco

- **Parâmetros esperados:** nome do bloco, finalidade, formatos onde se aplica e capabilities.
- **Exemplo:**
  > Crie o scaffold de um bloco do Moodle 4.4 chamado block_coursestats que mostra estatísticas do curso em um bloco lateral nas páginas de curso, com uma capability de visualização.
- **Saída esperada:** `scaffold_plugin` com tipo `block` (e, opcionalmente, `create_plugin_skeleton`). Você recebe a classe do bloco, a capability e as strings de idioma.

### Gerar o contexto de um plugin

- **Parâmetros esperados:** o plugin (componente como `local_myplugin`, caminho relativo à raiz do Moodle como `local/myplugin` ou caminho absoluto).
- **Exemplo:**
  > Gere o contexto de IA do local_myplugin para você entendê-lo antes de começarmos.
- **Saída esperada:** o agente chama `generate_plugin_context`. Ele cria os 12 arquivos de contexto `PLUGIN_*` (metadados, estrutura, tabelas, eventos, dependências, funções, callbacks, endpoints, fluxo de execução, arquitetura, configurações e o contexto de IA combinado) e marca o plugin como `.indevelopment`.

### Gerar contexto dos meus plugins em desenvolvimento

- **Parâmetros esperados:** nenhum; os plugins devem estar marcados com `.indevelopment`. Opcionalmente, diga para forçar a regeneração ou rodar em paralelo.
- **Exemplo:**
  > Regenere o contexto de todos os meus plugins em desenvolvimento, ignorando o cache, com 4 workers.
- **Saída esperada:** o agente chama `plugin_batch` com modo `dev` (padrão), `force` e `parallel`. Você recebe um resumo de sucesso ou falha por plugin.

### Gerar contexto de todos os plugins

- **Parâmetros esperados:** nenhum. Diga se também quer marcar todos como em desenvolvimento.
- **Exemplo:**
  > Gere o contexto de todos os plugins da instalação e marque todos como em desenvolvimento.
- **Saída esperada:** `plugin_batch` com modo `all` e `mark_as_dev`. Você recebe um resumo por plugin; o processamento é sequencial por padrão, então instalações grandes podem demorar.

### Gerar contexto de uma lista escolhida de plugins

- **Parâmetros esperados:** os plugins (componentes, caminhos relativos ou caminhos absolutos).
- **Exemplo:**
  > Gere o contexto de local_relatorios, mod_checklist e block_coursestats.
- **Saída esperada:** `plugin_batch` com modo `list` e os identificadores dos plugins. Identificadores que não puderem ser resolvidos são informados pelo nome.

### Obter saída legível por máquina

- **Parâmetros esperados:** qual operação, e que você quer saída estruturada (JSON).
- **Exemplo:**
  > Faça uma verificação de saúde e me dê o resultado em JSON para eu passar para um script.
- **Saída esperada:** o agente passa o formato `json` às ferramentas que o suportam (`doctor`, `init_moodle_context`, `update_indexes`, `search_plugins`, `search_api`, `get_plugin_info`, `list_dev_plugins`, `generate_plugin_context`, `plugin_batch`) e devolve uma resposta estruturada em vez de Markdown.

### Gerar PHPDoc

- **Parâmetros esperados:** o plugin e, opcionalmente, os arquivos ou classes.
- **Exemplo:**
  > Carregue o local_myplugin e escreva um bloco PHPDoc para cada função e método público que ainda não tenha um, com tags @param, @return e @throws corretas.
- **Saída esperada:** o agente carrega o contexto do plugin (`generate_plugin_context` / `moodle://plugin/{component}/functions`) e edita os arquivos-fonte. Você recebe o código documentado.

### Gerar uma atualização de banco de dados

- **Parâmetros esperados:** o plugin e as mudanças de esquema (novas colunas, índices, renomeações).
- **Exemplo:**
  > O local_myplugin precisa de uma atualização de banco: adicionar a coluna "status" (tinyint, padrão 0) em local_myplugin_data, adicionar um índice em (userid, status) e renomear old_value para previous_value em local_myplugin_logs. Carregue o esquema atual e atualize db/upgrade.php, db/install.xml e version.php.
- **Saída esperada:** o agente lê o esquema (`moodle://plugin/{component}/database`) e escreve o passo de upgrade, o `install.xml` atualizado e o `$plugin->version` incrementado.

### Empacotar um plugin para release

- **Parâmetros esperados:** o plugin (componente como `local_myplugin`, caminho relativo à raiz do Moodle como `local/myplugin` ou caminho absoluto) e, opcionalmente, um diretório de saída existente.
- **Exemplo:**
  > Empacote o local_myplugin em um ZIP em ~/releases.
- **Saída esperada:** o agente chama `release_plugin`. Ele gera um ZIP que exclui os arquivos gerados pelo próprio build82 e outros artefatos não distribuíveis (respeitando também o `.buildignore`) e informa o componente e a versão liberados, a origem (relativa à raiz do Moodle), o local do ZIP (relativo à raiz do Moodle quando está dentro dela, senão apenas o nome do arquivo) e o que foi excluído. O ZIP é gravado de forma atômica, então uma falha nunca deixa um arquivo parcial.

---

## Testes e Validação

### Executar uma verificação de saúde

- **Parâmetros esperados:** nenhum.
- **Exemplo:**
  > Execute uma verificação completa de saúde do build82 e me diga o que precisa ser corrigido.
- **Saída esperada:** o agente chama `doctor`. Você recebe as verificações de dependências do sistema (`php`, `ctags`, `git`), configuração, instalação do Moodle, atualidade dos índices globais, plugins em desenvolvimento, arquivos legados pendentes de migração, consistência entre plugins, uso de APIs obsoletas do core, verificações das próprias capabilities, uso das próprias strings de idioma e estatísticas de cache, mais um veredito geral.

### Encontrar uso de API obsoleta

- **Parâmetros esperados:** plugins em desenvolvimento marcados com `.indevelopment`.
- **Exemplo:**
  > Algum dos meus plugins em desenvolvimento chama funções obsoletas do Moodle? O que devo usar no lugar?
- **Saída esperada:** `doctor` (seção de APIs obsoletas) mais `search_api` com visibilidade obsoleta para as substituições.

### Verificar capabilities e strings de idioma

- **Parâmetros esperados:** plugins em desenvolvimento marcados com `.indevelopment`.
- **Exemplo:**
  > Verifique se meus plugins em desenvolvimento só checam capabilities que eles declaram e só usam strings de idioma que existem.
- **Saída esperada:** `doctor` (seções de consistência de capabilities e de strings de idioma). Você recebe as entradas ausentes por plugin.

### Verificar consistência entre plugins

- **Parâmetros esperados:** pelo menos dois plugins em desenvolvimento.
- **Exemplo:**
  > Algum dos plugins que estou desenvolvendo declara o mesmo nome de capability?
- **Saída esperada:** `doctor` (seção de consistência entre plugins). Ele compara apenas os nomes de capabilities declarados no `db/access.php` de cada plugin; nomes de tabelas e eventos não são verificados.

### Verificar se um plugin está pronto para o moodle.org

- **Parâmetros esperados:** o plugin (componente, caminho relativo ou caminho absoluto).
- **Exemplo:**
  > Empacote o local_myplugin, mas antes confira se ele atende aos requisitos do diretório de plugins do moodle.org. Não gere o ZIP se algo estiver faltando.
- **Saída esperada:** `release_plugin` com `strict` ativado. Ele valida os campos do `version.php` (component, requires, maturity), o arquivo de idioma, o provedor de privacidade, o `thirdpartylibs.xml` quando existe a pasta `thirdparty/` e a ausência de um diretório `.git`. Se alguma verificação falhar, lista os problemas e não gera o ZIP.

### Gerar testes PHPUnit

- **Parâmetros esperados:** a classe a testar (nome totalmente qualificado) e o plugin.
- **Exemplo:**
  > Gere uma classe de teste PHPUnit para \local_myplugin\util\data_processor. Carregue primeiro o contexto do plugin e depois cubra todos os métodos públicos usando advanced_testcase, com fixtures e pelo menos dois casos por método.
- **Saída esperada:** o agente carrega o contexto do plugin e o índice de classes (`moodle://plugin/{component}/functions`, `moodle://classes-index`) e escreve a classe de teste em `tests/`.

### Verificar a atualidade antes de trabalhar

- **Parâmetros esperados:** nenhum.
- **Exemplo:**
  > Antes de começarmos, garanta que o contexto do build82 está atualizado para os meus plugins em desenvolvimento e atualize o que estiver desatualizado.
- **Saída esperada:** `doctor` para detectar índices desatualizados (com mais de 7 dias), seguido de `update_indexes` e/ou `plugin_batch` para regenerar.

---

## Trabalhando Entre Sessões

Tarefas longas (uma revisão, uma depuração, um scaffold) atravessam várias conversas. Informe ao agente o estado atual e ele recarrega o contexto do plugin.

### Retomar o desenvolvimento

- **Parâmetros esperados:** o plugin e onde você parou.
- **Exemplo:**
  > Carregue o contexto do local_myplugin. Continuando de ontem: a estrutura foi criada com o scaffold. O próximo passo é a tarefa agendada em classes/task/sync_task.php.
- **Saída esperada:** o agente lê o contexto de IA do plugin (`moodle://plugin/{component}` ou `get_plugin_info`) e continua do ponto informado.

### Retomar uma revisão

- **Parâmetros esperados:** o plugin, o foco da revisão, o que já foi revisado e o que vem a seguir.
- **Exemplo:**
  > Carregue o local_myplugin. Estou continuando a revisão de segurança de ontem. Já revisados: lib.php, db/install.xml e index.php. Próximos: classes/external/ e classes/task/.
- **Saída esperada:** o agente carrega o contexto do plugin e usa `review_plugin` restrito aos arquivos restantes.

### Retomar uma sessão de depuração

- **Parâmetros esperados:** o plugin, o erro, o que você já verificou e o que ainda é desconhecido.
- **Exemplo:**
  > Carregue o local_myplugin. Continuando a investigação do erro "Table doesn't exist" para mdl_local_myplugin_data. Já verifiquei: o db/install.xml está correto e o plugin foi reinstalado. O erro só ocorre no contexto de curso. Continue daqui.
- **Saída esperada:** o agente carrega o contexto e segue com uma análise no estilo `debug_plugin`, sem repetir as verificações que você listou.

### Registrar o progresso nas notas do projeto

- **Parâmetros esperados:** o que registrar e o arquivo de notas usado pelo seu agente (por exemplo `CLAUDE.md`).
- **Exemplo:**
  > Atualize o CLAUDE.md com o estado atual: o local_myplugin foi revisado quanto à segurança em lib.php e index.php, e o erro "Table doesn't exist" foi causado por X, corrigido com Y.
- **Saída esperada:** o agente edita o seu arquivo de notas; nenhuma ferramenta do build82 é necessária. Na próxima sessão ele pode ler esse arquivo e recarregar o contexto do plugin.

Alguns clientes também salvam e retomam conversas inteiras (por exemplo `/chat save` e `/chat resume` no Antigravity CLI, ou `codex resume --last` no Codex).

---

## Veja Também

- [Referência dos prompts MCP](./reference/prompts.md) — argumentos e comportamento de `scaffold_plugin`, `review_plugin` e `debug_plugin`.
- [Referência das ferramentas](./reference/tools.md) — parâmetros de cada ferramenta.
