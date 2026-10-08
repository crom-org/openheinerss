# Ponte: custom e mock

**custom** (`pkg/harness/custom.go`): argv = `args` do spec (com `{{prompt}}` substituído) **+ `harness_args`** (intactos, na ordem). Se o spec usa `{{prompt}}`, os extras vêm depois dele (o spec é dono da posição do prompt; para colocá-los antes, escreva-os em `args`); com `prompt: stdin` eles ficam antes do prompt. `/x` vai literal (argumento ou stdin). Cada linha de stderr e cada JSON de tipo desconhecido saem como evento `raw` (o `text` do JSON desconhecido continua).

**mock** (`pkg/harness/mock`): grava os `harness_args` (`ReceivedArgs()`) e os prompts (`ReceivedPrompts()`). Se há args, o primeiro evento do turno é `raw` com `harness_args=<json>`. Prompt `/x ...` → `raw` e `text` com `comando /x recebido` e `complete` (sem a simulação de permissão).
