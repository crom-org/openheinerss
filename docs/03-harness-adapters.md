# Adaptadores de harness

O contrato interno está em `pkg/harness/harness.go`. Cada adaptador inicia um processo (ou o worker SDK do Claude), envia prompts e normaliza NDJSON para eventos `agent.*`.

O adaptador é só a ponte: argumentos nativos (`harness_args`), comandos `/x` e as linhas sem mapeamento (`agent.raw`) passam sem filtro. Veja [PONTE.md](PONTE.md).

## Harnesses embutidos

| Nome | Execução real | Permissão |
|---|---|---|
| `mock` | simulação determinística offline | emite pedido fixo para teste |
| `claude-code` | CLI `claude` ou worker SDK | emite `agent.permission_request` |
| `opencode` | CLI `opencode run --format json` | não emite permissão própria |
| `codex` | `codex exec --json` (com retomada nativa quando aplicável) | executa com bypass de aprovações |
| `agy` | CLI `agy` | não emite permissão própria |
| `aider` | CLI `aider` | não emite permissão própria |

Os eventos comuns são `agent.thinking`, `agent.text`, `agent.tool_call`, `agent.tool_result`, `agent.complete`, `agent.error` e, quando o motor informa, `agent.usage`. Apenas `claude-code` e `mock` emitem hoje `agent.permission_request`; `risk` é um valor recebido/fixo, não uma classificação universal do Openheinerss.

## Modos e modelos

O CLI aceita `mock`, `cli` e `sdk` conforme o harness. Não existe um cliente HTTP API separado do OpenCode: o adaptador chama o CLI `opencode run --format json`. O Codex também não usa Assistants API/Threads/Runs; chama o CLI `codex exec --json`.

Modelos locais podem ser passados ao CLI quando o motor instalado os suporta, por exemplo `--model ollama/qwen2.5-coder:32b`. O cache de GPU é responsabilidade do servidor local; não é gerenciado pelo Openheinerss.

## Cache, histórico e checkpoints

O Openheinerss não implementa prompt caching: ele só decide se o cache do próprio Claude Code fica ligado. Com a Anthropic direta o cache é deixado como está; com outro provedor (`provider` diferente de `anthropic`/vazio ou `ANTHROPIC_BASE_URL` fora da Anthropic) o adaptador define `DISABLE_PROMPT_CACHING=1`, porque provedores de terceiros costumam recusá-lo.

As sessões gravam o transcript em `.openheinerss/sessions/<id>.jsonl` e criam um checkpoint por cópia de arquivos antes do primeiro prompt (`.openheinerss/checkpoints/`, sem tocar no git; pula dependências e arquivos grandes). `session.resume` e `run --retomar <id>` reabrem a sessão usando o ID nativo do motor (Codex e Claude Code). Não há rollback automático antes de cada ferramenta.

No modo SDK do Claude Code o worker Node procura `@anthropic-ai/claude-agent-sdk` no `node_modules` do projeto, em `$OPENHEINERSS_CLAUDE_SDK_PATH` e em `~/.openheinerss/shims/node_modules`; sem ele, a sessão falha na hora com a correção sugerida (use o modo CLI).

## Harness custom

Use `.openheinerss/harnesses/*.yaml` ou `harness add` para registrar um comando sem recompilar. O comando pode herdar `base`, definir `command`, `args`, `env`, `model`, `prompt`, `finishRegex` e `quotaRegex`, e emitir NDJSON com `text`, `tool`, `error`, `usage` e `end`/`complete`. Consulte [06-harness-custom.md](06-harness-custom.md).
