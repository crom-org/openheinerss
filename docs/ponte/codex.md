# Ponte: codex

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
