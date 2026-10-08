# Auditoria de lacunas — openheinerss (etapa 0 do PLANO-COMPLETO.md)

Pedido: "Eu quero o Open Harness completo" — comparar o que README.md, `docs/`, `documentacao/` e os READMEs de `sdk/` prometem com o que existe no código.

Método: leitura de toda a documentação (2.659 linhas), greps em `cmd/`, `pkg/`, `sdk/`, e execução do binário compilado (`go build -o /tmp/oh-lacunas ./cmd/openheinerss`), com `--help` da raiz e de todos os subcomandos. Nenhum arquivo de código foi alterado. Nenhum segredo impresso.

## Atualização da etapa 4b — resolvido pela documentação

Marcados como resolvidos documentalmente em 08/10/2026:

- [x] `documentacao/` deixou de duplicar o conteúdo: os capítulos antigos foram removidos e o README aponta para `docs/`.
- [x] A especificação oficial é `docs/02-protocol-spec.md`; os nomes incorretos (`session.start`, `session.permission`, `doctor.run`, `session.resume`, `severity`) não são mais publicados.
- [x] O manual único [`docs/09-cli.md`](09-cli.md) é gerado de `--help` por `openheinerss docs` e o teste `TestManualCLIAtualizado` falha quando diverge.
- [x] README e docs agora dizem explicitamente que cache Anthropic, transcript automático, checkpoints/rollback, IPC, host MCP, OpenCode HTTP API e classificador universal de permissões não estão implementados.
- [x] README e docs registram a implementação real: Codex via `codex exec --json`, permissões emitidas por `claude-code`/`mock`, cotas locais, `rodar` e instâncias custom.

Os itens de implementação que continuam parciais ou ausentes permanecem registrados como tal em [`docs/05-roadmap.md`](05-roadmap.md); não são promessas do produto.

Comandos reais do binário (`/tmp/oh-lacunas --help`, `cmd/openheinerss/main.go:53-62`):
`completion`, `doctor`, `harness {add,list,test}`, `init`, `limites`, `mcp {add,list}`, `motores`, `rodar`, `run`, `serve`, `version`.

---

## 1. Tabela de promessas

Legenda do **estado**: `existe` · `parcial` · `falta` · `doc desatualizada` (promessa existe em parte ou a doc descreve algo que mudou).

### 1.1 CLI, flags e variáveis de ambiente

| Promessa | Onde é prometida | Estado | Evidência no código | Sugestão |
| :--- | :--- | :--- | :--- | :--- |
| `openheinerss doctor` | README.md:95; docs/05:23; documentacao/09:14,25 | existe | cmd/openheinerss/main.go:54,130-144; saída real OK (Git, Go, Node, Claude, OpenCode, Docker) | — |
| Saída do doctor com **Aider** e **Ollama** | documentacao/09:36-37 | doc desatualizada | pkg/doctor/doctor.go:21-60 só tem Git, Go, Node, Claude Code CLI, OpenCode CLI, Docker | Atualizar exemplo ou adicionar checagens de `aider`/`ollama` |
| `openheinerss init` cria `.openheinerss/` com `config.yaml`, `mcp.json`, `sessions/`, `checkpoints/` | README.md:101; docs/05:13; documentacao/06:11; documentacao/02:87-100 | existe | cmd/openheinerss/main.go:55,146; pkg/config/config.go:26-72 | — |
| `openheinerss run` com `--harness/--mode/--model/--provider` | README.md:108-117; documentacao/09:71-76 | parcial | flags reais: cmd/openheinerss/main.go:293-300 (existe `--esforco`, `--papel`, `--motor`, alias `--modelo`) | — |
| `run --permission-mode` (prompt/auto_allow/deny) | documentacao/09:76 | **falta** | não há essa flag (grep em cmd/openheinerss/main.go:293-300); valor de `permission_mode` só vem do `config.yaml`/RPC | Remover da doc ou implementar a flag |
| `run` com modo padrão "sdk/api" | documentacao/09:73 | doc desatualizada | default da flag `--mode` é `"mock"` (cmd/openheinerss/main.go:296) | Corrigir doc |
| `openheinerss serve --stdio` / `--port` / `--host` | README.md:150-153; docs/05:11-12; documentacao/09:91-94 | existe | cmd/openheinerss/main.go:109-113 | — |
| WebSocket em `:4820` (raiz e `/ws`) | README.md:17; documentacao/02:49-52 | existe | pkg/server/websocket.go:111-112; cmd/openheinerss/main.go:119-127 | — |
| Transporte **IPC / Unix Sockets** | docs/00:36; docs/01:23,62 | **falta** | nenhum `net.Listen("unix")` em `cmd/` e `pkg/` | Remover da doc ou implementar |
| `openheinerss setup <harness>` (correção guiada) | docs/01:112; docs/05:25 | **falta** | não existe comando `setup` no registro (cmd/openheinerss/main.go:53-62) | Remover da doc ou criar o comando |
| `openheinerss mcp list` / `mcp add` | README.md:220-223; docs/06:23; documentacao/06:45-65 | existe | cmd/openheinerss/main.go:412-455; pkg/server/router.go:167-197 (RPC `mcp.list`/`mcp.add`) | — |
| `openheinerss harness list/add/test` | README.md:252-254; docs/06:23 | existe | cmd/openheinerss/main.go:474-573; saída real de `harness list` mostra 6 embutidos + 3 custom | — |
| `harness test --todos` (matriz completa) | PLANO-COMPLETO.md:16 (etapa 1) | **falta** | `harness test` aceita só `<nome>` e `--prompt` (cmd/openheinerss/main.go:514-542) | Adicionar flag `--todos` (etapa 1) |
| `openheinerss motores` lista harnesses e papéis | README.md:142 | parcial | cmd/openheinerss/main.go:380-404 — lista os 6, mas **sai com erro** se `.openheinerss/motores.yaml` não existir (testado: `Error: abrir configuração de motores ... no such file or directory`) | Tornar `motores.yaml` opcional |
| `openheinerss limites [--json]` | README.md:142; docs/07:26-33 | existe | cmd/openheinerss/main.go:344-376; saída real JSON com `agora`/`instancias`/janelas 5h e semanal | — |
| `openheinerss rodar <nome> <instância>` com `--retomar`/`RETOMAR=1`, `--carga-maxima`, `--cota-max`, `--tentativas`, `--pasta-agentes`, `--branch-base` | README.md:162-167; docs/07:3-24 | existe | cmd/openheinerss/main.go:305-340, 315 (`RETOMAR=1`); pkg/orchestrator/runner.go:423 (worktree), 173/176 (envs) | — |
| `openheinerss version` | documentacao/09:19 | existe | cmd/openheinerss/main.go:463; saída `v0.1.0-alpha` | Trocar por versão real na etapa 5 |
| Env `OPENHEINERSS_PORTA` | documentacao/09:116 | existe | cmd/openheinerss/main.go:119 | — |
| Env `OPENHEINERSS_HARNESS` | documentacao/09:117 | **falta** | nenhuma ocorrência em `cmd/`/`pkg/` | Remover da doc ou implementar |
| Env `OPENHEINERSS_LOG_LEVEL` | documentacao/09:118 | **falta** | nenhuma ocorrência; projeto não tem nem logger por nível | Remover da doc |
| Env `OPENHEINERSS_COTA_MAX` / `CARGA_MAXIMA` | docs/07:23 | existe | pkg/orchestrator/runner.go:173-176 | — |
| `CLAUDE_CONFIG_DIR` isolado por provedor | README (implícito); docs/03:82; documentacao/09:119 | existe | pkg/harness/claudecode/claudecode.go:137-140 (`~/.openheinerss/profiles/claude-<provider>`) | — |

### 1.2 Protocolo RPC e eventos

| Promessa | Onde é prometida | Estado | Evidência no código | Sugestão |
| :--- | :--- | :--- | :--- | :--- |
| Método `session.create` | docs/02:9 | existe | pkg/protocol/messages.go:5; pkg/server/router.go:32 | — |
| Método `session.start` | documentacao/03:15; documentacao/README.md:19; **README.md:236** | **falta** (doc errada) | pkg/protocol/messages.go:5 tem só `session.create`; router não tem `session.start` | Corrigir documentacao/03 e a tabela do README |
| `session.prompt` (campo `text` + `images`) | docs/02:30-48 | existe | pkg/protocol/messages.go:70-74 (`Text`, `json:"images"`) | — |
| `session.prompt` com campo `prompt` e `attachments` | documentacao/03:51-71 | doc desatualizada | campo real é `text` (messages.go:72) e o anexo vira `images` | Corrigir doc |
| `session.permission_respond` (`decision`) | docs/02:50-63 | existe | pkg/protocol/messages.go:7,83-88; router.go:54 | — |
| `session.permission` (`allow: true`) | documentacao/03:86-112 | **falta** (doc errada) | método real é `session.permission_respond` com `decision` | Corrigir doc |
| `session.abort` | docs/02:65; documentacao/03:116 | existe | messages.go:8; router.go:65 | — |
| **`session.resume`** | README.md:236; docs/03:122; docs/05:58; documentacao/03:133; documentacao/05:71; documentacao/README:19 | **falta** | `grep -rn "resume" pkg/server pkg/session pkg/protocol` → zero; router.go:31-199 não tem o caso | Implementar ou tirar de todos os docs (é a promessa "retomar sessão sem reprocessar") |
| `session.list` | docs/05:58 | existe | messages.go:9,118-121; router.go:76 | Documentar em documentacao/03 |
| `catalog.list` | docs/02:163; documentacao/03:150 | existe | messages.go:11; router.go:80 | — |
| Campos `display_name`/`supported_modes` no catálogo | documentacao/03:172-173 | doc desatualizada | JSON real é camelCase `displayName`/`supportedModes` (messages.go:157-159) | Corrigir doc |
| `doctor.check` | docs/02:173 | existe | messages.go:10; router.go:161 | — |
| `doctor.run` | documentacao/03:209; documentacao/README:19 | **falta** (doc errada) | método real é `doctor.check` | Corrigir doc |
| `agent.thinking`, `agent.text`, `agent.tool_call`, `agent.tool_result`, `agent.permission_request`, `agent.complete`, `agent.error` | README.md:79-87; docs/02:80-157; documentacao/03:228-331 | existe | pkg/protocol/events.go:5-12; tradução em pkg/session/manager.go:272-295 | — |
| `agent.usage` (tokens) | docs/06:19 | existe | events.go:12,66-72; emitido por claudecode.go:363, codex.go:286, opencode.go:335, custom.go:410 | — |
| `agent.complete` com `duration_ms`, `input_tokens`, `output_tokens` | README.md:85; documentacao/03:303-315 | doc desatualizada | `CompleteParams` só tem `sessionId`+`reason` (events.go:53-56); tokens saem em `agent.usage` | Corrigir README/doc (ou enriquecer o payload) |
| `agent.error` com **`suggestedFix`** | README.md:86; documentacao/03:318-330; documentacao/10:133 | **falta** | `ErrorParams` não tem o campo (events.go:59-63); `suggestedFix` só existe em erro JSON-RPC (pkg/protocol/jsonrpc.go:62), em `doctor` (messages.go:135) e nos pré-requisitos (pkg/harness/harness.go:45) | Adicionar `suggestedFix` ao payload de `agent.error` |
| `agent.permission_request` com campo `risk` (low/medium/high) | docs/02:143-157 | existe | events.go:44-50; valor vem do Claude SDK (claudecode.go:560,568) e é fixo `"medium"` no mock (mock.go:146) | — |
| `agent.permission_request` com campo `severity` + `reason` | documentacao/03:286-301 | doc desatualizada | campo real é `risk` (events.go:49) | Corrigir doc |
| Erros 4001/4002/4010/4011/4020/4030 | docs/02:209-217 | existe | pkg/protocol/jsonrpc.go:15-20 | — |
| `harness.register` / `registerHarness({...})` | README.md:257; docs/06:23 | parcial | método existe (messages.go:14; router.go:84); SDK TS (sdk/typescript/src/index.ts:35) e Python (sdk/python/openheinerss/agent.py:93) expõem; **PHP não** (sdk/php/src/Agent.php sem `registerHarness`) | Adicionar no PHP ou ajustar README |
| Métodos de orquestração `rodar.*`, `limites.obter`, `harness.listar`, `eventos.assinar` e eventos `orq.*` | README.md:261; docs/02:222-315 | existe | pkg/protocol/orquestracao.go:4-22; pkg/server/router.go:110-159 | — |

### 1.3 Cache, modelos locais, transcripts e checkpoints

| Promessa | Onde é prometida | Estado | Evidência no código | Sugestão |
| :--- | :--- | :--- | :--- | :--- |
| **Prompt caching nativo "ativado por padrão"** para conexões diretas da Anthropic (−90% de custo) | README.md:36; docs/03:113-115; documentacao/05:36-43 | **falta / contradiz o código** | Não há nenhuma lógica de `cache_control`/marcadores ephemeral; e o adaptador injeta **sempre** `DISABLE_PROMPT_CACHING=1` (pkg/harness/claudecode/claudecode.go:127), para todo provedor | Decidir: implementar caching de verdade OU reescrever README/documentacao/05 |
| Bypass automático `DISABLE_PROMPT_CACHING=1` **só para provedores de terceiros** | README.md:37; docs/03:116; documentacao/05:46-54 | doc desatualizada (comportamento real: sempre) | claudecode.go:127 é incondicional dentro de `Start` | Condicionar à lista de terceiros ou corrigir a doc |
| **KV-cache em GPU** (Ollama/vLLM) gerenciado pelo Openheinerss | README.md:38; docs/03:118-119; documentacao/05:58-62 | parcial | Não há código de KV-cache (é responsabilidade do servidor local); o que existe é o repasse de `--model ollama/...` (pkg/harness/opencode/opencode.go:148-149; pkg/harness/aider/aider.go:122) | Reenunciar como "compatível com", não "gerenciado" |
| Modelos locais offline: `opencode + ollama/...`, `aider + ollama/...`, `mock` | README.md:48-62; docs/03:126-131; documentacao/05:82-117 | existe | opencode.go:148-149; aider.go:122; mock (pkg/harness/mock/mock.go); comandos testados existem | — |
| **Transcript caching**: histórico gravado em `.openheinerss/sessions/<id>.jsonl` nos 6 harnesses | README.md:39; docs/01:90; docs/03:121-122; docs/05:56-58; documentacao/05:66-71 | **falta** (código morto) | `pkg/storage/transcript.go:38-69` grava e `:72-97` lê, mas **nenhum chamador**: `grep -rn "RecordEvent\|LoadSession" --include=*.go` fora de `pkg/storage/` → zero. `.openheinerss/sessions/` do projeto está vazio | Ligar `storage.RecordEvent` no `pkg/session/manager.go` (broadcast) |
| **Checkpoints & rollback** (`pkg/checkpoint`) antes de edições arriscadas, sem gastar tokens | README.md:40; docs/01:65; docs/05:59-60; documentacao/05:75-78; documentacao/07:56-61 | **falta** (código morto + frágil) | Nenhum chamador de `CreateCheckpoint`/`RestoreCheckpoint` fora de `pkg/checkpoint/`; além disso `CreateCheckpoint` só grava **metadata + hash do git** (checkpoint.go:36-69) — não fotografa arquivos — e `RestoreCheckpoint` faz `git checkout -- .` (checkpoint.go:81-83), descartando mudanças não commitadas | Reescrever (snapshot de conteúdo) e ligar ao fluxo de permissão |
| Modo API do `opencode` (HTTP direto OpenAI/Ollama/DeepSeek, "dual mode") | README.md:21; docs/03:92-93; documentacao/04:52 | parcial/**falta** | Enum `ModeAPI` existe (pkg/harness/harness.go:16) mas `opencode.go` só executa `opencode run --format json` (SendPrompt, opencode.go:144-149) em qualquer modo; não há cliente HTTP | Implementar ou tirar "dual mode" da doc |
| `codex` = **Assistants API com Threads e Runs** | README.md:22; docs/03:95-97; documentacao/04:56-62 | doc desatualizada | Implementação real é o CLI: `codex exec --json` (pkg/harness/codex/codex.go:32,94,227-229), com retomada por `codex exec resume` (thread id) | Reescrever a doc (CLI, não Assistants API) |
| `agy` e `aider` participam do contrato de permissão | README.md:84 (lista os 6) | parcial | Só `claude-code` e `mock` emitem `EventPermission` (grep: claudecode.go, mock.go; agy.go/aider.go/codex.go/opencode.go = 0); codex roda com `--dangerously-bypass-approvals-and-sandbox` (codex.go:227-229) | Corrigir README ou implementar |
| Hub MCP **central** que instancia servidores e injeta ferramentas em qualquer harness | README.md:25; docs/01:116-121; documentacao/06:70-76 | parcial | `pkg/mcp/hub.go` só lê/grava `.openheinerss/mcp.json` (`LoadConfig`, `ListServers`, `RegisterServer`; 111 linhas) e é usado só pelos comandos `mcp list/add` e RPC; **nenhuma injeção de ferramentas** no harness | Limitar a promessa a "configuração central" ou implementar o host |
| Permissões com modos `prompt` / `auto_allow` / `deny` | documentacao/07:13-17; README.md:240 | doc desatualizada | Valores reais: `ask`/`always_allow`/`plan` (pkg/protocol/messages.go:43; pkg/config/config.go:48) e mapeamento CLI `bypassPermissions`/`manual` (claudecode.go:312-331) — não há `deny` nem `auto_allow` | Alinhar nomes (doc ou código) |
| Classificação de criticidade low/medium/high por regra (só leitura/edição/destrutivo) | documentacao/07:21-27 | **falta** | Não existe classificador: risk vem do Claude SDK (claudecode.go:560) e é literal `"medium"` no mock (mock.go:146) | Criar `pkg/permission` ou corrigir doc |
| Handshake síncrono de permissão (cliente aprova → harness segue) | documentacao/07:31-52; docs/01:75 | existe | pkg/session/manager.go:194-219 (`RespondPermission`); events.go:44-50; fluxo mock testado (`harness test mock` → `permission {perm_mock_1 Bash git status medium}`) | — |
| Diretórios `~/.openheinerss/profiles`, `shims/` | docs/01:95-104 | parcial | `profiles/` existe (pkg/config/config.go:81-82); `shims/` não — o worker Node é embutido como string (`claudecode.go:153`) | Ajustar doc |

### 1.4 SDKs

| Promessa | Onde é prometida | Estado | Evidência no código | Sugestão |
| :--- | :--- | :--- | :--- | :--- |
| TS: `new Openheinerss({options:{...}})`, `on("thinking"/"text"/"permission")`, `prompt()` | README.md:176-183; sdk/typescript/README.md | existe | sdk/typescript/src/index.ts:15,70,182-200 | — |
| TS: `registerHarness` | README.md:257 | existe | index.ts:35-37 | — |
| TS: `endpoint: "ws://localhost:4820"` | documentacao/08:17; docs/04:53 | doc desatualizada | campo real é `wsEndpoint` + `transport: "stdio"\|"websocket"` (index.ts:8-13) | Corrigir exemplos |
| TS: `agent.on("complete", meta => meta.duration_ms/output_tokens)` | documentacao/08:41-43 | doc desatualizada | emite `p.reason` (index.ts:203-204) e o tipo não tem tokens | Corrigir exemplo |
| TS/React: hook `useOpenheinerss` com `permissionRequest`/`respondPermission` | documentacao/08:49-87 | existe | sdk/typescript/src/react.ts:16,119-129 (retorna `permissionRequest`, `respondPermission`, `abort`) | — |
| TS: pacote separado `@openheinerss/react` | docs/04:42-47; docs/05:67 | doc desatualizada | o hook é exportado do próprio `@openheinerss/sdk` (index.ts:6 `export * from "./react.js"`); não existe pacote `@openheinerss/react` | Unificar a doc |
| TS: transporte WebSocket em navegador | documentacao/08:9 ("navegadores modernos") | parcial | index.ts:142-161 usa `WebSocket` global, mas `sendRPC` depende de `msg.id` correlacionado — sem `rodar`/`limites`/`eventos.assinar` (grep em `sdk/` → zero) | Completar na etapa 3 do plano |
| PHP: `Agent::session()`, `prompt()` | README.md:195-199; sdk/php/README.md; documentacao/08:96-113 | existe | sdk/php/src/Agent.php:19,52 | — |
| PHP: **`$agent->stream(callable)`** | docs/04:98-100; documentacao/08:123-131 | **falta** | sdk/php/src/Agent.php tem só `prompt($text, ?callable $onEvent)`, `respondPermission`, `session` (grep `stream` → zero) | Criar `stream()` ou corrigir docs |
| PHP: `$result->text()` | docs/04:95 | doc desatualizada | `prompt()` devolve `string` (Agent.php:52) | Corrigir doc |
| PHP: `registerHarness` | README.md:257 ("SDKs") | **falta** | sem método em sdk/php/src/Agent.php | Adicionar ou restringir a promessa |
| PHP: transporte WebSocket | documentacao/08 (implícito) | **falta** | só `proc_open("openheinerss serve --stdio")` (sdk/php/src/Transport/StdioTransport.php:18) | Doc ou implementar |
| Python: `Agent`, `stream()`, `prompt()`, eventos em dict | README.md:205-211; sdk/python/README.md; documentacao/08:141-157 | existe | sdk/python/openheinerss/agent.py:53,86 | — |
| Python: `event.type` (atributo) | docs/04:112-117 | doc desatualizada | `stream()` devolve dicts `{"type", "data"}` (agent.py:77) | Corrigir exemplo de docs/04 |
| Python: **`openheinerss.aio.AsyncAgent`** (asyncio) | documentacao/08:159-171; docs/05:70-71 ([x]); documentacao/02:34 | **falta** | `openheinerss/` só tem `agent.py`, `transport.py`, `__init__.py`; `__init__.py` exporta `Agent`, `StdioTransport` | Criar módulo `aio` ou desmarcar o roadmap |
| Python: WebSocket | documentacao/08:144 | **falta** | transport.py:7-9 só spawna `serve --stdio` | Doc |
| Python: auto-resposta de permissão no `stream()` | (não prometido; observação) | risco | agent.py:79-81 autoriza **automaticamente** toda permissão no stream | Documentar comportamento |
| SDKs expõem `rodar`, `limites`, eventos `orq.*`, porta 4820 | PLANO-COMPLETO.md:18 (etapa 3) | **falta** | grep `rodar|limites|eventos.assinar` em `sdk/` → zero | Etapa 3 do plano |

### 1.5 Harnesses custom, orquestração e roadmap

| Promessa | Onde é prometida | Estado | Evidência no código | Sugestão |
| :--- | :--- | :--- | :--- | :--- |
| 6 harnesses embutidos (`mock`, `claude-code`, `opencode`, `codex`, `agy`, `aider`) | README.md:18-24; documentacao/04:9-17 | existe | `harness list` real: 6 embutidos (mais `claude-conta2`, `codex2`, `opencode-gratis` custom) | — |
| Instâncias custom em `.openheinerss/harnesses/` e `~/.config/openheinerss/harnesses` | README.md:131-140,249; docs/06:3 | existe | pkg/harness/custom.go:175-179; arquivos reais `.openheinerss/harnesses/{codex2,claude-conta2,opencode-gratis}.yaml` | — |
| Herança `base:`, eventos NDJSON `text/tool/error/usage/end` | README.md:249; docs/06:10-21 | existe | pkg/harness/custom.go:77,410,457 (inclui `usage` e `complete`) | — |
| `motores.yaml` (`papel: motor/modelo [esforco=]`) | README.md:122-145 | existe | pkg/motor/motor.go:61-111 | — |
| Roadmap Fase 4 marcada `[x]`: "Endpoints `session.list` e `session.resume`" | docs/05:58 | parcial | `session.list` existe; `session.resume` não | Desmarcar |
| Roadmap Fase 4 `[x]`: "Persistência NDJSON" e "Checkpoints & Git rollback" | docs/05:56-60 | **falta** | código existe mas sem chamadores (ver 1.3) | Desmarcar |
| Roadmap Fase 5 `[x]`: Python síncrono **e assíncrono** | docs/05:70-71 | parcial | sem `aio` | Desmarcar async |
| Roadmap Fase 1-3 `[x]` (serve, init, run, mock, doctor, dual mode, catálogo) | docs/05:7-49 | existe | conforme 1.1/1.2 (dual mode do opencode é o asterisco) | — |
| Itens não marcados (Integração CCO, extensão VSCode) | docs/05:72-74 | corretamente pendentes | — | — |

---

## 2. Os 10 problemas mais importantes (resumo)

1. **Prompt caching prometido, mas o código desliga o cache sempre** — README.md:36, docs/03:113 e documentacao/05:36 prometem caching nativo Anthropic "ativado por padrão"; `pkg/harness/claudecode/claudecode.go:127` injeta `DISABLE_PROMPT_CACHING=1` incondicionalmente e não existe nenhuma lógica de marcadores de cache. *Ação: reescrever a doc (mais barato) ou condicionar a injeção ao provedor ser a Anthropic.*
2. **Transcript/session persistence não funciona** — README.md:39, docs/03:121 e documentacao/05:66 prometem `.openheinerss/sessions/*.jsonl` gravado por todos os harnesses; `pkg/storage/transcript.go` é **código morto** (zero chamadores; `grep RecordEvent/LoadSession` fora do pacote → nada). Pasta `sessions/` do projeto vazia. *Ação: ligar no `pkg/session/manager.go`.*
3. **`session.resume` não existe** — prometido em README.md:236, docs/03:122, docs/05:58, documentacao/03:133, documentacao/05:71 e documentacao/README:19; o método não está em `pkg/protocol/messages.go:4-17` nem no `switch` de `pkg/server/router.go:31-199`. É a base da promessa "retomar sessão sem reprocessar". *(Existe retomada parcial nativa nos motores: `resumeID` em claudecode.go:120, opencode.go:113, codex.go:64 — mas sem método RPC.)*
4. **Checkpoints/rollback não são usados e a implementação não fotografa arquivos** — README.md:40 e documentacao/07:56 prometem snapshots antes de edições; `pkg/checkpoint/checkpoint.go` nunca é chamado e `CreateCheckpoint` grava só metadados + hash git (checkpoint.go:60-66), enquanto `RestoreCheckpoint` roda `git checkout -- .` (checkpoint.go:81), que apaga mudanças não commitadas de qualquer jeito. *Ação: reescrever com cópia real de arquivos e plugar no fluxo de permissão — ou remover a promessa.*
5. **Hub MCP é só editor de `mcp.json`, não host** — README.md:25, docs/01:119-121 e documentacao/06:72 prometem instanciação de servidores e injeção das ferramentas em qualquer harness; `pkg/mcp/hub.go` só carrega/registra a configuração e é consumido pelos comandos `mcp list/add` (cmd/openheinerss/main.go:417,452; pkg/server/router.go:173,193). *Ação: limitar a doc a "configuração centralizada" na etapa 4.*
6. **Permissão/criticidade não é universal nem classificada** — README.md:79-87 lista `agent.permission_request` para "qualquer harness" e documentacao/07:21 promete classificação low/medium/high; só `claude-code` (claudecode.go:560,568) e `mock` (mock.go:146, valor fixo `"medium"`) emitem o evento; `codex` roda com `--dangerously-bypass-approvals-and-sandbox` (codex.go:227-229). *Ação: corrigir README e, se quiser, criar um classificador.*
7. **`agent.error` sem `suggestedFix` e `agent.complete` sem métricas** — README.md:85-86 e documentacao/03:303-330 prometem os dois; `ErrorParams` (events.go:59-63) não tem `suggestedFix` e `CompleteParams` (events.go:53-56) só tem `reason` (tokens saem em `agent.usage`). *Ação: ajustar doc ou payload.*
8. **`documentacao/03` (especificação RPC) está inteiramente com os nomes errados** — `session.start` (:15), `session.permission` (:86), `doctor.run` (:209), campo `prompt`/`session_id`/`severity`/`tool_name` (:61,295,260), `display_name` snake_case (:172) e o README.md:236 repete esses nomes. O código usa `session.create`, `session.permission_respond`, `doctor.check`, `text`/`sessionId`/`risk`/`tool`, camelCase (pkg/protocol/messages.go:5-16,72; events.go:49). `docs/02` está correta. *Ação: reescrever documentacao/03 a partir de `docs/02` — quem seguir a "documentação oficial" escreve um cliente que não funciona.*
9. **Manual do CLI (documentacao/09) incompleto e com flags/envs inexistentes** — não cita `harness`, `motores`, `limites`, `rodar` (todos existem, main.go:53-62), promete `run --permission-mode` (:76) e as envs `OPENHEINERSS_HARNESS`/`OPENHEINERSS_LOG_LEVEL` (:117-118) que não existem; `docs/01:112` promete `openheinerss setup` (inexistente); e `openheinerss motores` **falha com erro** quando não há `.openheinerss/motores.yaml` (testado). *Ação: atualizar o manual na etapa 4 e tornar `motores.yaml` opcional.*
10. **Capacidades "dual mode"/API exageradas na doc** — `opencode` modo API (README.md:21, documentacao/04:52) não tem implementação HTTP (só `opencode run --format json`, opencode.go:144-149); `codex` não usa Assistants API/Threads-Runs (é `codex exec --json`, codex.go:32,94); IPC/Unix socket (docs/00:36, docs/01:62) não existe; SDK PHP sem `stream()`/`registerHarness`/WebSocket e Python sem `aio`/WebSocket. *Ação: reescrever as frases "dual mode/API/IPC" ou implementar.*

**Bônus (consistência):** os SDKs não têm nenhum método de orquestração (`rodar`, `limites`, `eventos.assinar`) — depende da etapa 3 do plano; e `harness test --todos` + `scripts/teste-real.sh` (etapa 1 do plano) ainda não existem.

---

## 3. Duplicação entre `docs/` e `documentacao/`

Existem **duas documentações paralelas** descrevendo o mesmo sistema, e elas **conflitam**:

| `docs/` | `documentacao/` | Situação |
| :--- | :--- | :--- |
| 00-overview.md (69 l.) | 01-visao-geral-e-manifesto.md (76 l.) | duplicação de visão geral/manifesto |
| 01-architecture.md (121 l.) | 02-arquitetura-e-design.md (102 l.) | duplicação de arquitetura e pastas |
| 02-protocol-spec.md (315 l.) | 03-especificacao-do-protocolo-rpc.md (331 l.) | **conflito grave**: `docs/02` usa os nomes corretos (`session.create`, `session.permission_respond`, `doctor.check`, `risk`); `documentacao/03` usa os errados (`session.start`, `session.permission`, `doctor.run`, `severity`) e ainda diverge de `docs/02` em `tool_name/arguments` vs `tool/input` e `content/is_error` vs `status/output`. O README:236 cita a versão errada. |
| 03-harness-adapters.md (151 l.) | 04-guia-completo-de-harnesses.md (81 l.) | duplicação (6 harnesses, dual mode, cache) |
| 04-sdk-any-language.md (154 l.) | 08-sdks-oficiais.md (171 l.) | duplicação; ainda conflitam: `docs/04:98` mostra `$agent->stream()` que não existe e `docs/04:112` usa `event.type` (atributo) enquanto `documentacao/08:150` usa dict |
| 05-roadmap.md (75 l.) | 10-guia-de-desenvolvimento-e-extensao.md (133 l.) | sobreposição parcial (roadmap vs passo a passo de adaptador); roadmap tem itens `[x]` que não existem |
| — (sem equivalente) | 05-cache…, 06-hub-mcp…, 07-checkpoints…, 09-manual-do-cli | conteúdo coberto por seções do **README** (cache: README:30-41; MCP: README:215-224; permissões/checkpoints: README:240; CLI: README:90-167) → **tripla duplicação** |
| 06-harness-custom.md, 07-rodar.md (novos, 2026-10-08) | — (nada) | `documentacao/` **não fala** de harness custom, `rodar`, `motores`, `limites`, `limites.obter`, eventos `orq.*` → a "documentação oficial" (README:228) está desatualizada em relação ao que foi implementado nas etapas 1–2 |

**Recomendação para a etapa 7 do plano:** manter `docs/` como fonte da verdade (está mais alinhada com o código) e transformar `documentacao/` em sinônimo/redirecionamento — hoje elas se contradizem em nomes de método, o que quebra qualquer cliente escrito pela documentação "oficial".

---

## 4. Decisões tomadas nesta auditoria (sem perguntas)

## 5. Fechamento da etapa 4a (08/10/2026)

Resolvidos no código: transcripts ligados ao manager em `.openheinerss/sessions/<id>.jsonl`; `session.resume` no RPC e `run --retomar <id>` com retomada nativa; cache condicional à Anthropic; checkpoints por cópia de arquivos, sem `git checkout -- .`; `suggestedFix` em erros conhecidos e duração/totais em `agent.complete`; Aider não interativo com classificação rápida; e `motores` tolerante à ausência de `motores.yaml`.

O modo SDK do Claude valida `@anthropic-ai/claude-agent-sdk` antes de iniciar e retorna dependência ausente com correção sugerida em vez de timeout. A prova real de `claude-conta2` ficou bloqueada pelo limite de sessão da conta e deve ser repetida após a renovação; o modo CLI continua disponível.

- Relatório gravado em `.claude/agentes/relatorios/auditoria-lacunas.md` (caminho determinado pelas regras da missão); o `docs/LACUNAS.md` previsto em PLANO-COMPLETO.md:15 **não foi criado** para não alterar o repositório de código/docs durante uma missão de análise.
- "Existe" só quando há comando/método/evento no código **e** foi verificado (por leitura ou execução do binário); "parcial" quando a implementação atende em parte; "falta" quando não há rastro no código; "doc desatualizada" quando o código mudou e a doc não acompanhou.
- Itens meramente promocionais ("binário de ~15MB", "startup sub-ms", "zero dependências") foram ignorados por não serem verificáveis de forma determinística nesta etapa.
- Nenhum segredo/token impresso; `limites --json` foi executado e apenas a estrutura (sem credenciais) foi citada — o comando lê só arquivos locais de statusline (docs/07:28-33).
