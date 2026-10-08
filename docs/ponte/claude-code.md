# Ponte: claude-code

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
| Stream (`system`, `rate_limit_event`, tipos novos, blocos de conteúdo desconhecidos) | descartados | emitidos como `raw` (`agent.raw`), além dos eventos mapeados |
| stderr | só no texto de erro | cada linha vira `raw` (stream `stderr`); `stderrTail` nas mensagens de erro continua |
| Custo/uso | `usage` com `total_cost_usd` | igual |
| Eventos mapeados | text, thinking, tool_call, tool_result, usage, error, complete | iguais |
| Demais flags (`--bg`, `--worktree`, `--settings`, `--agents`, `--plugin-dir`, `--fork-session`, `--json-schema`, `--max-budget-usd`…) | inacessíveis | via `harness_args` |
