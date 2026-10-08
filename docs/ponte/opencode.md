# Ponte: opencode

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
