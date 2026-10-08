# Ponte: aider

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
