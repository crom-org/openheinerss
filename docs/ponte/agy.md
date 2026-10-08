# Ponte: agy (Antigravity)

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
