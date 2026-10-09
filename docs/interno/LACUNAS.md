# Lacunas: promessas × código (situação final, etapa 7)

Pedido do usuário: "Eu quero o Open Harness completo". A etapa 0 do `PLANO-COMPLETO.md` comparou tudo o que README, `docs/`, o antigo `documentacao/` e os READMEs dos SDKs prometiam com o que existia no código. Este arquivo guarda o resultado **depois** das etapas 1 a 7: cada lacuna aponta onde foi fechada ou, quando não foi implementada, a decisão e a justificativa. Não há item "falta" em aberto: o que não existe aparece como **não implementado (decisão)** e a documentação diz isso com todas as letras.

Legenda: **existe** (código + teste ou execução real) · **corrigido** (era lacuna; fechada por código) · **doc corrigida** (a promessa saiu ou mudou na documentação) · **não implementado (decisão)** (fora do escopo desta entrega, documentado como tal).

Verificação: cada linha foi conferida lendo o código e/ou executando o binário compilado na etapa 7; o roteiro para repetir isso está em [VERIFICACAO.md](VERIFICACAO.md).

## 1. CLI, flags e variáveis de ambiente

| Promessa | Situação | Onde está / decisão |
| :--- | :--- | :--- |
| `doctor`, `init`, `serve --stdio/--port/--host`, `mcp list/add`, `version` | existe | `cmd/openheinerss/main.go`; `version` informa versão, commit e data por `ldflags` (build local = `dev`) |
| `run` com `--harness/--mode/--model/--provider/--esforco/--papel/--motor/--retomar` | existe | `newRunCmd`; o fim do turno e os erros do agente aparecem; Ctrl-C encerra o motor |
| `run --permission-mode` | doc corrigida | a flag nunca existiu; o modo de permissão vem de `config.yaml` ou do RPC (`ask`, `always_allow`, `plan`) |
| `harness list/add/test` | existe | `harness test --todos` lê o catálogo real; `--pular`, `--json`, `--timeout`, `--incluir-principal`, `--retomar` |
| `harness test --todos` | corrigido | etapa 1; matriz em [TESTES-REAIS.md](TESTES-REAIS.md); sai com código 1 se algum teste não deu OK |
| `motores` | corrigido | `motores.yaml` virou opcional (sem ele: perfis e "Nenhum papel") |
| `limites [--json]` | existe | `pkg/limites`; lê Codex (`CODEX_HOME`) e Claude (statusline do crom-painel) |
| `rodar` (`--retomar`, `RETOMAR=1`, `--carga-maxima`, `--quando-carga-abaixo`, `--max-agentes`, `--tentativas`, `--cota-max`, `--texto`, `--pasta-agentes`, `--branch-base`) | existe | `pkg/orchestrator`; código de saída = código do `FIM` |
| `agentes` (`listar`, `ver`, `parar`) | existe | etapa 6; estado por PID e log |
| `docs` (manual gerado) | existe | `docs/09-cli.md`; `TestManualCLIAtualizado` falha se divergir |
| Apelidos em inglês de comandos e flags | existe | `launch`/`dispatch`, `agents`, `limits`, `engines`, `--all`, `--skip`, `--resume`... |
| `OPENHEINERSS_PORTA`, `OPENHEINERSS_COTA_MAX`, `CARGA_MAXIMA`, `MAX_AGENTES`, `TENTATIVAS`, `AGENTES`, `BRANCH_BASE`, `RETOMAR` | existe | `configuredPort`, `orchestrator.Options.defaults` |
| `OPENHEINERSS_ORIGENS`, `OPENHEINERSS_CLAUDE_SDK_PATH` | existe (novas, etapa 7) | origens extras do WebSocket; caminho opcional do SDK do Claude |
| `OPENHEINERSS_HARNESS`, `OPENHEINERSS_LOG_LEVEL` | doc corrigida | nunca existiram; foram retiradas da documentação |
| Transporte IPC / Unix socket | não implementado (decisão) | STDIO cobre o uso local e WebSocket o de rede; dito em `docs/00` e `docs/05` |
| `openheinerss setup <harness>` | não implementado (decisão) | o `doctor` e o `suggestedFix` indicam a correção; `docs/01` diz que não há comando `setup` |

## 2. Protocolo RPC e eventos

| Promessa | Situação | Onde está / decisão |
| :--- | :--- | :--- |
| `session.create/prompt/permission_respond/abort/list`, `catalog.list`, `doctor.check` | existe | `pkg/server/router.go`; `docs/02` |
| `session.resume` | corrigido | etapa 4a; reabre o transcript e entrega o ID nativo ao Codex/Claude; IDs com `/` ou `..` são recusados |
| `harness.register`, `harness.listar` | existe | registra instância sem arquivo |
| `rodar.iniciar/listar/parar/decidir`, `limites.obter`, `eventos.assinar` e `orq.inicio/progresso/erro/precisa_decisao/fim` | existe | etapa 2; testes de ponta a ponta no WebSocket e no STDIO; exemplo em `scripts/ws-cliente.mjs` |
| `agent.thinking/text/tool_call/tool_result/permission_request/complete/error/usage` | existe | `pkg/protocol/events.go` |
| `agent.complete` com duração e totais de tokens | corrigido | `duration_ms`, `input_tokens`, `output_tokens`, `total_tokens` |
| `agent.error` com `suggestedFix` | corrigido | preenchido nos casos conhecidos (CLI ausente, sem login, sem cota) por `harness.ClassifyFailure` |
| Nomes antigos (`session.start`, `session.permission`, `doctor.run`, `severity`, `prompt`/`attachments`) | doc corrigida | `docs/02` é a única especificação; a pasta `documentacao/` virou só um aviso |
| `agent.permission_request` em qualquer harness | doc corrigida | só `claude-code` e `mock` emitem; Codex roda com bypass de aprovações; `risk` é informativo |
| Classificador universal de criticidade (low/medium/high) | não implementado (decisão) | exigiria interceptar as ferramentas de cada CLI, que não expõem isso; `risk` vem do motor |
| Modos de permissão `prompt`/`auto_allow`/`deny` | doc corrigida | valores reais: `ask`, `always_allow`, `plan` |
| Erros 4001/4002/4010/4011/4020/4030 | existe | `pkg/protocol/jsonrpc.go` |

## 3. Cache, modelos locais, transcripts e checkpoints

| Promessa | Situação | Onde está / decisão |
| :--- | :--- | :--- |
| Prompt caching "ativado por padrão" na Anthropic | corrigido + doc corrigida | o Openheinerss não implementa cache; só **não** desliga o do Claude Code quando o provedor é a Anthropic direta, e define `DISABLE_PROMPT_CACHING=1` para os demais (`directAnthropic`) |
| KV-cache de GPU (Ollama/vLLM) | não implementado (decisão) | é do servidor local; o Openheinerss só repassa `--model ollama/...` aos CLIs |
| Modelos locais (`opencode`/`aider` + `ollama/...`) | existe | repasse do modelo ao CLI |
| Transcript em `.openheinerss/sessions/<id>.jsonl` | corrigido | etapa 4a; gravado pelo `session.Manager` em todos os harnesses |
| Checkpoints | corrigido | cópia de arquivos antes do primeiro prompt (sem `git checkout`); na etapa 7 passou a pular `.git`, dependências (`node_modules`, `vendor`...), arquivos acima de 10 MB e a parar em 256 MB (`parcial`) |
| Rollback automático antes de cada ferramenta | não implementado (decisão) | `RestoreCheckpoint` existe no pacote, mas nenhum comando/RPC o chama; dito em `docs/05` |
| Modo API HTTP do `opencode` ("dual mode") | não implementado (decisão) | o adaptador chama `opencode run --format json`; dito em `docs/03` |
| `codex` via Assistants API | doc corrigida | o adaptador usa `codex exec --json` (e `exec resume`) |
| Hub MCP que hospeda servidores e injeta ferramentas | não implementado (decisão) | `pkg/mcp` só mantém `mcp.json`; dito no README e em `docs/01` |
| `~/.openheinerss/profiles` e `shims/` | doc corrigida | `profiles/` existe; `shims/` é só o local opcional do SDK do Claude (`~/.openheinerss/shims/node_modules`) |

## 4. SDKs

| Promessa | Situação | Onde está / decisão |
| :--- | :--- | :--- |
| TS: `Openheinerss`, `on(...)`, `prompt()`, hook React no próprio pacote | existe | `sdk/typescript/src`; `wsEndpoint` + `transport` |
| TS, Python, PHP: `registerHarness`, `listHarnesses`, `run`, `listRuns`, `stopRun`, `decideRun`, `getLimits`, `subscribeEvents` e porta 4820 | corrigido | etapa 3 (0.2.0); teste de integração de cada SDK contra o servidor real; `run` ganhou `texto` (prompt em texto) na etapa 7 |
| Python: `Agent.stream()` com eventos em dicionário | existe | `sdk/python/openheinerss/agent.py`; o `stream()` autoriza as permissões sozinho (dito no README do SDK) |
| PHP: `Agent::session()`, `prompt()` | existe | `sdk/php/src/Agent.php` |
| PHP: `$agent->stream()` | não implementado (decisão) | `prompt($texto, $callback)` já entrega os eventos; `docs/04` descreve o que existe |
| Python `openheinerss.aio`, WebSocket em Python/PHP, pacote `@openheinerss/react` | não implementado (decisão) | STDIO cobre os dois SDKs; o hook React vem no pacote principal; `docs/04` e `docs/05` dizem isso |

## 5. Harnesses custom, orquestração e roadmap

| Promessa | Situação | Onde está / decisão |
| :--- | :--- | :--- |
| 6 harnesses embutidos | existe | `mock`, `claude-code`, `opencode`, `codex`, `agy`, `aider`; testes reais 8/8 em [TESTES-REAIS.md](TESTES-REAIS.md) |
| Instâncias por arquivo (`.openheinerss/harnesses`, `~/.config/openheinerss/harnesses`), herança `base:`, `reserva`, regexes | existe | `pkg/harness/custom.go`; `~/` expandido em `command` e `env`; nada de conta embutido no código (`grep` das instâncias do projeto no código dá zero) |
| Troca automática por `reserva` ao acabar a cota | existe | provada com harness falso (roteiro de verificação) e com o mock nos testes; a prova com conta real depende de uma conta esgotar de verdade |
| Parada imediata por cota sem reserva (`FIM código 2`, "resets 9am") | corrigido | etapa 6/7 |
| Roadmap `docs/05` sem itens `[x]` falsos | doc corrigida | reescrito na etapa 7 com "Entregue" e "Parcial ou não implementado" |
| Duplicação `docs/` × `documentacao/` | doc corrigida | `docs/` é a única fonte; `documentacao/README.md` só aponta para ela |

## 6. Problemas achados na revisão da etapa 7 (além da auditoria)

Corrigidos com teste (detalhes no [CHANGELOG](../CHANGELOG.md)): `mcp add` travando para sempre; caminho do SDK do Claude embutido no código; processo zumbi em harness custom e no worker SDK; saída com erro de harness custom contada como sucesso; `~` sem expansão em `env` sem `base`; WebSocket aceitava qualquer origem; `serve --stdio` ignorava SIGTERM; `rodar` não fechava log/meta com Ctrl-C e o processo saía sempre com código 0; `agentes parar` podia sinalizar o servidor e escrevia o `FIM` duas vezes; nome de agente com `..`; worktree criada antes de validar o prompt; checkpoint copiando dependências e arquivos enormes; ID de sessão com `/` em `session.resume`; sessões sem encerramento ao desligar o servidor; `--cota-max` presa em janelas de cota já renovadas; `harness test mock` travando em permissão; `run` podia travar no fim do turno.
