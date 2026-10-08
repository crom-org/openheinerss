# Ponte: o openheinerss não esconde nada do harness

O openheinerss é só a ponte entre o cliente (CLI, SDKs, Central) e o harness. Regras que valem para todos os adaptadores:

- **`harness_args` / `harnessArgs` / `--harness-arg` (`--arg`)**: argumentos nativos extras vão ao processo **intactos e na mesma ordem**, antes do prompt. Não há lista de permitidos nem filtro.
- **Opções tipadas** (`effort`, `add_dirs`, `sandbox`, `agent`…) são atalhos; o que não tiver chave tipada vai por `harness_args`.
- **`/x` chega literal** quando o harness aceita comandos de barra no modo sem tela (ex.: `claude -p`). Onde não aceita, a ponte traduz o que tem equivalente (`/model`, `/effort`, `/new`, `/clear`…) e avisa com `agent.text` + `agent.complete`. Sem equivalente: erro claro `o harness X não aceita /cmd no modo sem tela e não há equivalente na linha de comando; <dica>`. Caminhos como `/home/x` não são comandos.
- **`agent.raw`**: toda linha do harness que não vira outro evento (stdout sem mapeamento e stderr) chega como `agent.raw` com `stream` e `line` originais. Clientes antigos ignoram o método novo; os eventos já existentes não mudaram.
- **Cota**: só é falta de cota quando o turno não teve resultado bem-sucedido (um `rate_limit_event` ou o texto do agente citando "cota" não derrubam um turno que terminou bem).
- **Reserva**: ao trocar de motor pela reserva, a instância usa o próprio modelo/esforço padrão; não herda o modelo do harness principal.

- **Catálogo de comandos**: `openheinerss comandos <harness>` / `harness.comandos` lista os `/x` de cada harness com o repasse desta tabela (`literal`, `traduzido`, `sem_equivalente`) e as anotações do usuário (`comandos.yaml`). Ver docs/02-protocol-spec.md.

Detalhe por harness (tabela "antes/agora" e o que não tem equivalente):


## Ponte: claude-code

Fonte: `claude --help` (salvo em /tmp/ponte-help/claude.txt) e `pkg/harness/claudecode`. Modo CLI = `claude -p --output-format stream-json`; modo SDK = worker Node com `@anthropic-ai/claude-agent-sdk`.

| Recurso do harness | Antes | Agora |
|---|---|---|
| Comandos `/...` (skills, personalizados, embutidos) | iam como prompt, sem tratamento | texto com `/` vai **literal** ao `claude -p`; só `/model X` e `/effort X` são traduzidos (muda as próximas chamadas, evento `text` de aviso + `complete`, sem chamar o processo). `/model` sem argumento vai literal |
| Flags nativas quaisquer | não passavam | `harness_args` (CLI `--arg`, protocolo `harnessArgs`) no argv, na ordem, depois das opções tipadas e antes do `--`/prompt. SDK: `--flag valor` vira `extraArgs` do SDK; posicionais são ignorados |
| `--model` | sim | igual (e `/model`) |
| `--effort` (low…max) | não | opção `effort` → `--effort` (SDK: `extraArgs.effort`) |
| `--append-system-prompt` | não | `SessionConfig.SystemPrompt` (SDK: `systemPrompt.append`) |
| `--add-dir` | não | `add_dirs` (lista) → um `--add-dir` por item (SDK: `additionalDirectories`) |
| `--mcp-config` | não | `mcp_config` (texto ou lista) (SDK: `extraArgs.mcp-config`) |
| `--allowed-tools` / `--disallowed-tools` | não | `allowed_tools` / `disallowed_tools` (juntados por vírgula) (SDK: `allowedTools`/`disallowedTools`) |
| `--permission-mode`, bypass | sim (`permissoes`/`rodar`) | igual |
| `--resume <id>` | sim (id do stream) | igual |
| `--continue` | não | opção `continue: true` quando não há id de retomada (SDK: `continue`) |
| `--max-turns` | — | **não existe** no `--help` instalado; use `harness_args` se a versão aceitar |
| Anexos/imagens | ignorados no CLI | continuam sem flag no `claude -p`; só o SDK recebe `images`. Use `harness_args` ou `--input-format stream-json` |
| Stream (`system`, `rate_limit_event`, tipos novos, blocos de conteúdo desconhecidos) | descartados | emitidos como `raw` (`agent.raw`), além dos eventos mapeados; `rate_limit_event` rejeitado só vira cota se o turno terminar sem resultado bom |
| stderr | só no texto de erro | cada linha vira `raw` (stream `stderr`); `stderrTail` nas mensagens de erro continua |
| Custo/uso | `usage` com `total_cost_usd` | igual |
| Eventos mapeados | text, thinking, tool_call, tool_result, usage, error, complete | iguais |
| Demais flags (`--bg`, `--worktree`, `--settings`, `--agents`, `--plugin-dir`, `--fork-session`, `--json-schema`, `--max-budget-usd`…) | inacessíveis | via `harness_args` |

## Ponte: codex

Fonte: `codex --help` / `codex exec --help` (/tmp/ponte-help) e `pkg/harness/codex`. Modo CLI = `codex exec --json` (e `exec resume <thread>`).

| Recurso do harness | Antes | Agora |
|---|---|---|
| Comandos `/...` | iam como prompt (o `codex exec` não os interpreta) | `/model X`, `/effort X` (ou `/reasoning X`): mudam as próximas chamadas, evento `text` + `complete`. `/new` e `/clear`: esquecem o id da thread. `/compact`: **NoEquivalent** (não há compactação no `exec`; dica de `-c` via harnessArgs). Qualquer outro `/x`: **NoEquivalent** com dica de usar `harness_args` ou o comando nativo. Erro devolvido por `SendPrompt` |
| Flags nativas quaisquer | não passavam | `harness_args` no argv, na ordem, antes do id da thread e do `--` |
| `-m` | sim | igual |
| Esforço (`-c model_reasoning_effort`) | sim | igual |
| `--sandbox` | sempre `--dangerously-bypass-approvals-and-sandbox` | opção `sandbox` (read-only, workspace-write, danger-full-access) → `--sandbox`; sem ela, o bypass de antes. Em `exec resume` (sem `-s`) vira `-c sandbox_mode="..."` |
| `--profile` | não | opção `profile` → `--profile` (não existe em `exec resume`: a retomada herda a thread) |
| `-c k=v` | só o esforço | opção `config` (lista) → um `-c` por item |
| Imagens `-i` | anexos ignorados | anexos base64 → arquivos temporários → `--image=<arquivo>` (forma com `=`, porque a opção aceita lista); apagados ao fim do processo |
| Retomar | `exec resume <thread>` | igual; `/new` zera |
| `thread.started`, `turn.started`, `item.*` não tratados | descartados | `raw` (`agent.raw`) |
| stderr | cada linha vira `error` | continua, e cada linha também vira `raw` (stream `stderr`) |
| Custo/uso | `usage` (tokens) | igual (o codex não informa custo em USD) |
| Eventos mapeados | text, tool_call, tool_result, usage, error, complete | iguais |
| `--add-dir`, `--output-schema`, `--ephemeral`, `-C`, `--oss`, `--enable`… | inacessíveis | via `harness_args` (atenção: `exec resume` aceita menos flags) |

## Ponte: opencode

Base: `opencode run --help` (/tmp/ponte-help/opencode-run.txt) e `pkg/harness/opencode/opencode.go`.
Opções tipadas vêm de `SessionConfig.Options`; tudo o que não tiver chave tipada vai por `harness_args`.

| Recurso do harness | Antes | Agora |
|---|---|---|
| `--format json` (stream) | fixo | fixo; linha JSON sem mapeamento (ex. `step_start`) → evento `raw` (stdout) |
| `-m/--model provedor/modelo` | `cfg.Model` | igual; `/model X` muda o modelo das próximas chamadas |
| `--variant` (esforço) | não | `Options.effort` (ou `variant`); `/effort X` |
| `--agent` | não | `Options.agent`; `/agent X` |
| `-s/--session` | `opencode_session_id`, aprendido do stream | igual; `/new` e `/clear` esquecem o id |
| `-c/--continue`, `--fork` | não | `Options.continue`, `Options.fork` (fork só com sessão/continue) |
| `-f/--file` (anexos) | não | `Options.files` (caminhos) + anexos do prompt gravados em arquivo temporário |
| `--command <cmd>` | não | `/x a b` → `--command x` + mensagem `a b` (sem args, sem mensagem) |
| `--share`, `--thinking` | não | `Options.share/thinking`; `/share`, `/thinking` ligam nas próximas chamadas |
| `--auto` (aprova permissões) | não | via `harness_args` |
| `--attach`, `--dir`, `--port`, `--title`, `--pure`, `--log-level`, `--print-logs`, `-u/-p` | não | via `harness_args` (intactos, na ordem, antes de `--` e da mensagem) |
| `-i/--interactive` | não | sem equivalente (precisa de tela) |
| MCP, custo/uso | uso (`step_finish`) → `usage` | igual; MCP é configuração do opencode (arquivo de config), não há flag |
| Eventos | text, tool_use, step_finish, error | + `raw` para o resto do stdout e para todo o stderr (o `[stderr]`/erro de antes continua) |
| Comandos só de tela (`/exit /quit /q /themes /editor /details /help /sessions /agents`) | texto cru ao modelo | `NoEquivalent` (erro síncrono em `SendPrompt`) |

Ordem do argv: `run --format json [--session|--continue] [--fork] [-m] [--variant] [--agent] [--share] [--thinking] [--file…] [--command] <harness_args…> [-- mensagem]`.

## Ponte: aider

Base: /tmp/ponte-help/aider.txt e `pkg/harness/aider/aider.go`.

| Recurso do harness | Antes | Agora |
|---|---|---|
| `--message` (um turno e sai) | `--message=texto` | igual, mas agora é o ÚLTIMO argumento |
| `/comandos` (`/ask`, `/architect`, `/add`, `/run`, `/commit`…) | texto cru | literais dentro de `--message` (o aider interpreta) |
| `--model` | `cfg.Model` | igual; `/model X` muda o modelo das próximas chamadas (literal não persistiria: o processo sai) |
| `--reasoning-effort` | não | `Options.reasoning_effort` ou `effort`; `/reasoning-effort X` e `/effort X` |
| `--read` / `--file` | não | `Options.read_files` / `Options.files`; anexos do prompt viram arquivos temporários em `--file` |
| `--restore-chat-history` (retomar) | não | `Options.continue` (ou `restore_chat_history`) |
| `/new`, `/clear`, `/reset` | — | literais (o aider limpa o histórico do próprio turno); não há id de sessão para esquecer |
| `--edit-format/--chat-mode`, `--architect`, `--weak-model`, `--editor-model`, `--thinking-tokens`, `--map-tokens`, `--auto-commits`, `--test-cmd`, `--lint-cmd`, `--load`, `--set-env`, `--api-key`… | não | via `harness_args` (antes de `--message`) |
| Permissões | `--yes-always` fixo | igual (modo sem tela não tem como perguntar); outro valor via `harness_args` |
| MCP | — | o aider não tem MCP |
| Custo/uso | não há evento | sem mapeamento: a linha de tokens/custo do aider chega como `text` |
| stderr | só no erro final (`stderrTail`) | + um evento `raw` por linha |

Sem `NoEquivalent`: o aider interpreta qualquer `/x` na mensagem, então nada é recusado.
Ordem: `--yes-always … --no-browser [--model] [--reasoning-effort] [--read…] [--file…] [--restore-chat-history] <harness_args…> --message=…`.

## Ponte: agy (Antigravity)

Base: /tmp/ponte-help/agy.txt e `pkg/harness/agy/agy.go`.

| Recurso do harness | Antes | Agora |
|---|---|---|
| `-p/--print` + `--output-format stream-json` | fixo (`-p=texto`) | igual; o prompt continua por último |
| `--model` | `cfg.Model` | igual; `/model X` |
| `--effort low…max` | não | `Options.effort`; `/effort X` |
| `--agent` | não | `Options.agent`; `/agent X` |
| `--mode accept-edits\|plan` | não | `Options.mode`; `/mode X` |
| `--continue`, `--conversation ID` | não | `Options.continue`, `Options.conversation`; `/new` e `/clear` esquecem os dois |
| `--project`, `--add-dir`, `--sandbox` | não | `Options.project`, `add_dirs`, `sandbox` |
| `--dangerously-skip-permissions` | fixo | fixo (modo sem tela) |
| `/comandos` e skills | texto cru | literais em `-p=` (`--disable-slash-commands` existe ⇒ o print mode expande) |
| `/exit`, `/quit` | — | `NoEquivalent` |
| `--input-format`, `--json-schema`, `--print-timeout`, `--log-file`, `--remote-control`, `--disable-slash-commands`, `--new-project` | não | via `harness_args` (antes de `-p=`) |
| Subcomandos (`mcp`, `models`, `plugin`…) | — | fora do escopo de um turno; MCP/plugins são gerenciados no próprio agy |
| Anexos | não | agy não tem flag de anexo: sem equivalente (cite o caminho no prompt) |
| Eventos | JSON conhecido + texto | + `raw` por linha de stderr (o `text` do stderr continua) |

Ordem: `[--model] --dangerously-skip-permissions --output-format stream-json [--effort] [--agent] [--mode] [--conversation|--continue] [--project] [--add-dir…] [--sandbox] <harness_args…> -p=prompt`.

## Ponte: custom e mock

**custom** (`pkg/harness/custom.go`): argv = `args` do spec (com `{{prompt}}` substituído) **+ `harness_args`** (intactos, na ordem). Se o spec usa `{{prompt}}`, os extras vêm depois dele (o spec é dono da posição do prompt; para colocá-los antes, escreva-os em `args`); com `prompt: stdin` eles ficam antes do prompt. `/x` vai literal (argumento ou stdin). Cada linha de stderr e cada JSON de tipo desconhecido saem como evento `raw` (o `text` do JSON desconhecido continua).

**mock** (`pkg/harness/mock`): grava os `harness_args` (`ReceivedArgs()`) e os prompts (`ReceivedPrompts()`). Se há args, o primeiro evento do turno é `raw` com `harness_args=<json>`. Prompt `/x ...` → `raw` e `text` com `comando /x recebido` e `complete` (sem a simulação de permissão).
