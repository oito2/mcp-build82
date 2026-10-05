🌐 [English](../../../en/guides/workflows/examples.md) | **Português** | 🏠 [Índice](../../index.md)

---

# Exemplos de Uso

Casos de uso reais com prompts prontos, sequência de passos e resultado esperado. Use estes exemplos como ponto de partida para seus próprios fluxos de desenvolvimento.

---

## 1. Fluxo Completo: Do Zero ao Plugin Funcional

Este exemplo mostra a sequência completa de um ciclo de desenvolvimento, do scaffold ao primeiro commit.

**Passo 1 — Criar a estrutura:**

```
scaffold_plugin
  type="local"
  name="audit_log"
  description="Registra um histórico de auditoria das ações dos usuários"
  features="database tables, scheduled tasks, capabilities, event observers"
```

**Passo 2 — Gerar contexto de desenvolvimento:**

```
Gere o contexto de IA para o plugin local_audit_log que acabei de criar.
```

**Passo 3 — Ativar monitoramento:**

```
Inicie o monitoramento do plugin local_audit_log para alterações de arquivo.
```

**Passo 4 — Implementar lógica:**

```
Carregue o contexto do plugin local_audit_log. Preciso implementar o observer
de evento para core\event\user_loggedin. Ele deve registrar na tabela
local_audit_log_entries os campos: userid, eventname, ip e timecreated.
```

**Passo 5 — Revisar antes do commit:**

```
/review_plugin plugin="local/audit_log" focus="security"
```

**O que o servidor faz em cada etapa:**
- Passo 1: injeta contexto Moodle + padrões da versão detectada — IA gera arquivos corretos
- Passo 2: `generate_plugin_context` — cria 12 arquivos `PLUGIN_*.md` sob `local/audit_log/.build82/`
- Passo 3: `watch_plugins action="start"` — atualiza contexto automaticamente ao salvar `db/*.php`/`version.php`
- Passo 4: `get_plugin_info` — IA lê `PLUGIN_DB_TABLES.md` e gera o observer com nomes reais de tabela e campos
- Passo 5: prompt `review_plugin` — IA analisa com foco em capability checks, output escaping e SQL injection

---

## 2. Criando um Plugin Local do Zero

**Cenário:** Você precisa de um plugin local que exibe um relatório de participação por curso para coordenadores.

**Prompt de scaffold:**

```
scaffold_plugin
  type="local"
  name="relatorios"
  description="Relatórios de participação e frequência por curso para coordenadores"
  features="database tables, web services, capabilities, scheduled tasks"
```

**Após criar os arquivos, carregue o contexto e continue:**

```
Gere o contexto do plugin local_relatorios. Em seguida, crie a tabela
local_relatorios_cache no db/install.xml com os campos: id, courseid,
userid, participation_count (int), last_access (int) e timegenerated (int).
```

```
Agora crie a task agendada em classes/task/build_cache_task.php.
Ela deve reconstruir os registros expirados (mais de 24h) da tabela
local_relatorios_cache usando os dados do core do Moodle.
```

**Resultado esperado:** plugin com estrutura completa, tabela XMLDB correta com atributos `NOTNULL` e `SEQUENCE`, task agendada seguindo a interface `\core\task\scheduled_task` e capability `local/relatorios:viewreport`.

---

## 3. Analisando um Plugin Existente

**Cenário:** Você assumiu a manutenção de um plugin legado e precisa entender rapidamente o que ele faz.

**Passo 1 — Carregar contexto e pedir resumo:**

```
Carregue o contexto do plugin local_legado e me dê uma análise completa:
arquitetura, tabelas do banco, eventos registrados, web services,
capabilities e fluxo de execução principal.
```

**Passo 2 — Aprofundar em uma área específica:**

```
Use explain_plugin para me explicar em detalhes a seção de banco de dados
do plugin local_legado. Há tabelas sem índices ou campos mal tipados?
```

**Passo 3 — Verificar callbacks legados:**

```
O plugin local_legado usa callbacks do lib.php que já foram substituídos
pela Hook API no Moodle 4.3+? Liste cada um com os passos de migração.
```

**Passo 4 — Buscar dependências do core:**

```
Quais funções do core o plugin local_legado está usando que estão
marcadas como @deprecated? Sugira os substitutos corretos.
```

---

## 4. Debugging de Erros

**Cenário A — Erro de banco de dados:**

```
O plugin local_myplugin está gerando este erro ao abrir a página principal:

"Table 'moodle.mdl_local_myplugin_sessions' doesn't exist"

Carregue o contexto do plugin e me ajude a identificar a causa raiz.
O erro ocorre só para usuários não-administradores.
```

**Cenário B — Erro de classe não encontrada:**

```
Recebi este erro no log do Moodle:

"Cannot find class 'local_myplugin\output\renderer'"

O erro aparece quando acesso a página index.php do plugin. Analise a
estrutura do plugin local_myplugin e me diga o que está faltando ou
com namespace incorreto.
```

**Cenário C — Task agendada silenciosa (não executa):**

```
A task agendada \local_myplugin\task\sync_task não aparece no log
de execução do cron. Carregue o contexto do plugin e verifique
a definição em db/tasks.php em busca de erros de configuração.
```

**Usando o prompt debug_plugin:**

```
/debug_plugin
  plugin="local_myplugin"
  error="Table 'moodle.mdl_local_myplugin_sessions' doesn't exist"
  context="Ocorre ao abrir a página principal para usuários não-administradores"
```

---

## 5. Revisão de Código Antes do Commit

**Revisão de segurança:**

```
/review_plugin plugin="local/myplugin" focus="security"
```

Verifica: `require_capability()` faltando, output sem `s()` ou `format_text()`, parâmetros sem `required_param()`, queries com concatenação de strings em vez de placeholders.

**Revisão de padrões de código:**

```
/review_plugin plugin="local/myplugin" focus="standards"
```

Verifica: convenções de namespace, visibilidade de métodos, completude do PHPDoc, nomenclatura seguindo Frankenstyle.

**Revisão de banco de dados:**

```
/review_plugin plugin="local/myplugin" focus="database"
```

Verifica: índices faltando em colunas usadas em `WHERE`, padrão N+1 (loop com `get_record` dentro), falta de `$DB->get_in_or_equal()` para cláusulas `IN`.

**Revisão de performance:**

```
/review_plugin plugin="local/myplugin" focus="performance"
```

Verifica: queries sem cache para dados raramente alterados, falta de paginação em listagens grandes, uso desnecessário de `get_records` quando apenas um registro é necessário.

**Revisão de uso de API:**

```
/review_plugin plugin="local/myplugin" focus="apis"
```

Verifica: event handlers legados em vez do sistema de eventos, tasks agendadas que não estendem `\core\task\scheduled_task`, web services que não usam `external_function_parameters`/`external_value`, callbacks que deveriam ser migrados para a Hook API.

---

## 6. Busca e Consulta de API

**Encontrando funções para uma tarefa específica:**

```
Quais funções da API do core devo usar para:
1. Verificar se um usuário está matriculado em um curso
2. Enviar uma mensagem de notificação para um usuário
3. Obter a nota de um usuário em uma atividade específica

Use search_api e prefira funções públicas, não depreciadas.
```

**Verificando se uma função existe na versão instalada:**

```
A função enrol_get_course_users() existe na minha versão do Moodle?
Qual é a assinatura completa e em qual arquivo está definida?
```

**Encontrando alternativas para funções depreciadas:**

```
O plugin local_myplugin usa add_to_log(). Esta função está depreciada.
Busque na API do core a alternativa recomendada e mostre como migrar.
```

**Explorando eventos disponíveis:**

```
Quais eventos do core posso observar para rastrear quando um usuário
completa uma atividade? Liste os eventos com seus namespaces completos
e os dados que cada um fornece.
```

---

## 7. Migração para Hook API (Moodle 4.3+)

**Cenário:** Seu plugin usa callbacks legados do `lib.php` que foram substituídos pela Hook API.

**Passo 1 — Identificar callbacks legados:**

```
Carregue o contexto do plugin local_myplugin e liste todos os callbacks
do lib.php que já foram substituídos pela Hook API no Moodle 4.3+.
Para cada um, informe o nome do callback, o hook equivalente e
se a migração é obrigatória ou opcional.
```

**Passo 2 — Gerar a migração:**

```
Migre o callback local_myplugin_before_footer() do lib.php para a Hook API.
Gere:
1. A entrada em db/hooks.php
2. A classe callback em classes/hook_callbacks.php
3. O método que substitui a lógica atual
Mantenha a lógica exatamente igual, apenas adaptando a estrutura.
```

**Passo 3 — Verificar a migração:**

```
Regenere o contexto do plugin local_myplugin e confirme que o callback
local_myplugin_before_footer() não está mais listado como legado
no índice de callbacks.
```

---

## 8. Empacotando um Plugin para Distribuição

**Cenário:** Você terminou de desenvolver um plugin e quer gerar um ZIP limpo para compartilhar ou instalar em outra instância Moodle.

**Empacotar o plugin:**

```
Gere uma versão do plugin local_caedauth.
```

ou

```
Empacote o plugin local_caedauth para distribuição.
```

O assistente chamará `release_plugin` com `component="local_caedauth"` (um caminho como `local/caedauth` ou um caminho absoluto também funciona), lerá a versão do `version.php` e gerará o arquivo `local_caedauth_2026041000.zip` no diretório de trabalho atual. O ZIP é gravado de forma atômica: uma falha nunca deixa um arquivo parcial e mantém qualquer ZIP anterior com o mesmo nome.

**O que é excluído do ZIP** (os arquivos permanecem no projeto):

- `.build82/` — todo arquivo de contexto gerado, tanto no local atual quanto no layout legado plano, para plugins ainda não migrados
- `CLAUDE.md`, `GEMINI.md`, `AGENTS.md` e seus arquivos de ignore correspondentes (`.claudeignore`, `.geminiignore`, `.aiexclude`) — arquivos de contexto de assistentes de IA
- `.indevelopment` — marcador de desenvolvimento
- `node_modules` — se presente

**Saída esperada:**

```
✅ Released local_caedauth (version 2026041000).

ZIP folder: caedauth/
Output: local_caedauth_2026041000.zip
Source: local/caedauth

Excluded from the archive:
- .indevelopment
- .build82
- CLAUDE.md
```

> `Source` é relativo à raiz do Moodle. `Output` é relativo à raiz do Moodle quando o ZIP é gravado dentro dela; caso contrário, apenas o nome do arquivo é exibido (o ZIP fica no diretório de saída ou no diretório de trabalho). O component na primeira linha é o declarado no `version.php`. Um `component` vazio é rejeitado com `❌ component is required`.

**Validando antes do envio:** peça um release estrito ("Package local_caedauth with strict validation") e a tool é chamada com `strict=true`. Ela então verifica os requisitos do diretório do moodle.org (`$plugin->requires` e `$plugin->maturity` no `version.php`, `lang/en/{component}.php`, `classes/privacy/provider.php`, `thirdpartylibs.xml` quando existe `thirdparty/`, nenhum diretório `.git`) e não cria o ZIP se alguma verificação falhar. Use `output_dir` para escolher onde o ZIP é gravado (o diretório precisa já existir).

**Excluindo arquivos extras:** crie um arquivo opcional `.buildignore` na raiz do plugin — um nome de arquivo por linha, comentários com `#` permitidos — para excluir arquivos/diretórios adicionais do ZIP além da lista fixa acima.

---

## 💡 Dicas Gerais

**Seja específico sobre o plugin:**
Em vez de "analise meu plugin", prefira "carregue o contexto do plugin `local_myplugin` e...". Isso garante que a IA use a tool correta antes de responder.

**Use os prompts para tarefas complexas:**
Para scaffold, revisão e debugging, os prompts `scaffold_plugin`, `review_plugin` e `debug_plugin` injetam contexto adicional automaticamente. Em clientes que expõem prompts MCP como slash commands (ex: Gemini Code Assist, modo Agent), também estão disponíveis dessa forma.

**Regenere o contexto após mudanças significativas:**
Após adicionar novas tabelas, classes ou callbacks, peça: _"Regenere o contexto do plugin local_myplugin."_ Isso mantém o `PLUGIN_AI_CONTEXT.md` sincronizado com o estado atual do código.

---

## 🔖 Continuidade entre Sessões

Os clientes de IA não guardam memória de conversa entre sessões por padrão. Cada vez que você abre uma nova sessão, o assistente começa do zero — sem saber em quais plugins você estava trabalhando, quais decisões foram tomadas ou qual o estado atual do desenvolvimento.

O `CLAUDE.md`, `GEMINI.md` e `AGENTS.md` são a forma mais robusta de manter continuidade — funciona em qualquer sessão, sem precisar lembrar de nada manualmente.

### Estratégia recomendada: arquivo de contexto vivo

Peça ao assistente para atualizar o arquivo de contexto ao final de cada sessão importante:

```
Atualize o CLAUDE.md com o estado atual do desenvolvimento:
quais plugins estão em andamento, o que foi implementado hoje
e quais são os próximos passos.
```

O assistente editará o arquivo diretamente no diretório Moodle. Na próxima sessão, ele lerá esse estado automaticamente.

### Salvando e retomando sessões por cliente

**Claude Code** — o `CLAUDE.md` é lido automaticamente no início de cada sessão, e sessões anteriores podem ser retomadas com `claude --continue` (a mais recente) ou `claude --resume`. Atualize o `CLAUDE.md` ao final de sessões longas.

**Antigravity CLI** — suporta salvar e retomar sessões:

```bash
# Dentro do Antigravity CLI, salvar a sessão atual
/chat save moodle-dev

# Na próxima vez, retomar de onde parou
/chat resume moodle-dev
```

**OpenAI Codex** — suporta retomar a sessão mais recente:

```bash
# Retomar a conversa mais recente do diretório atual
codex resume --last
```

### Template de bloco de estado para o arquivo de contexto

Adicione uma seção como esta ao seu `CLAUDE.md` / `GEMINI.md` / `AGENTS.md` e peça ao assistente para atualizá-la regularmente:

```markdown
## Estado Atual do Desenvolvimento

Última atualização: 2025-01-15

### Em andamento
- local_relatorios — implementando task de cache (build_cache_task.php)
  - Tabela local_relatorios_cache criada ✅
  - Task agendada criada ✅
  - Lógica de preenchimento: pendente ⏳

### Próximos passos
1. Implementar a query de participação em build_cache_task.php
2. Criar a página de relatório (index.php)
3. Adicionar capability local/relatorios:viewreport

### Decisões tomadas
- Cache com TTL de 24h (registros com timegenerated > now - 86400 são reconstruídos)
- Relatório acessível apenas para managers e editingteachers
```

---

## ➡️ Próximos Passos

- [Referência de Tools](../../reference/tools.md) — parâmetros completos de todas as tools
- [Referência de Prompts](../../reference/prompts.md) — detalhes de scaffold_plugin, review_plugin e debug_plugin
- [Problemas Comuns](../../troubleshooting/common-issues.md) — quando algo não funciona como esperado
- [Voltar ao Índice](../../index.md)
