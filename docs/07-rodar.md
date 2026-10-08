# Executar missões

`openheinerss rodar <nome> <instância|harness>` executa o prompt em
`.claude/agentes/prompts/<nome>.md`, acrescentando `prompts/_regras.md`.
Agentes comuns recebem uma worktree `agente/<nome>`. O padrão da pasta é
`.claude/agentes` e a branch base é `main` (se o repositório não tiver `main`,
vale a branch atual/HEAD; sem commits ou com `--branch-base` inexistente o
`rodar` falha com a causa no log e no `FIM`, nunca em silêncio).

**Raiz do repositório.** A pasta de agentes, os logs, os limites e o nome do
projeto são sempre os do REPOSITÓRIO (`git rev-parse --git-common-dir`), mesmo
quando o `rodar` (ou `agentes`) é chamado de dentro de uma worktree, inclusive
a de outro agente: nada de worktree aninhada, logs divididos ou
`projeto=nome-da-worktree`.

**Missões somente leitura.** Nomes `missao-*` rodam numa pasta descartável
(uma worktree solta do repositório, ou pasta vazia se isso falhar) apagada no
fim; nada que o motor criar (`.aider*`, `.gitignore`…) chega ao repositório.

**Limite de agentes.** `--max-agentes` conta as vagas e registra o `meta.json`
sob uma trava (`logs/.vagas.lock`): com `--max-agentes 2` nunca rodam mais de 2
ao mesmo tempo, mesmo quando uma vaga é liberada por `agentes parar`.

```bash
openheinerss rodar revisor claude-conta2 --modelo claude-sonnet --esforco high
openheinerss rodar missao-relatorio mock --retomar
openheinerss rodar rapido mock --texto "responda OK"
openheinerss rodar curta claude-conta2 --modo sdk --texto "crie SDK-OK.txt e faça commit"
```

`--texto` (`--text`) dá o prompt direto na linha de comando, sem arquivo; `--prompt` aponta outro arquivo. O nome do agente só aceita letras, números, `.`, `_` e `-` (ele vira nome de pasta e de branch).

`--seco` (`--dry-run`) não cria pasta, trava nem arquivo: imprime o comando
completo (binário, modelo efetivo e argumentos; valores de variáveis com
KEY/TOKEN/SECRET aparecem como `***`). Quando o `rodar` termina com código ≠ 0,
a linha final do terminal traz a causa curta: `FIM nome código 1: <causa>`; o
detalhe fica no log.

Por padrão, o runner acrescenta regras curtas ao prompt: trabalhar somente dentro da pasta/worktree do agente e não fazer buscas fora dela (`find /`, `find ~`, `locate` ou varreduras de disco). Use `--sem-regras-padrao` (`--no-default-rules`) para desligá-las. Também é possível definir `regras_padroes: caminho/para/regras.txt` em `.openheinerss/config.yaml`; `sem_regras_padroes: true` desliga as regras pela configuração.

**Código de saída.** O processo termina com o código do `FIM` do log: `0` concluído, `1` erro (tentativas esgotadas), `2` falta de cota sem reserva (ou todas as instâncias acima de `--cota-max`) e `130` parado por Ctrl-C/SIGTERM (o log e o `meta.json` são fechados antes de sair) ou por `agentes parar`.

Cada execução atualiza `logs/<nome>.log` e, atomicamente,
`logs/<nome>.meta.json`. `--retomar` (ou `RETOMAR=1`) preserva o log e adiciona
o texto de continuação. Uma instância custom pode declarar `reserva: [outra]`;
ao encontrar a regex de cota ou um erro, as reservas são tentadas até
`--tentativas` (padrão 4). Cada reserva usa o modelo e o esforço da própria
instância: `--modelo`/`--esforco` valem só para o motor principal (o modelo de
um harness não serve em outro).

Uma instância baseada em `claude-code` pode declarar `modo: sdk`; o `rodar`
também aceita `--modo sdk` para forçar o modo. Erro de provedor (sobrecarga,
indisponibilidade, HTTP 429/5xx ou o padrão `error_regex`/`erro_regex` da
instância) é falha mesmo quando o motor termina com `completed`; com `reserva`,
a próxima instância é tentada.

**Cota e sobrecarga só no texto.** O padrão de cota (regex da instância ou o
padrão) e o de erro de provedor valem para eventos de erro. Uma instância que
apenas imprime a frase no stdout e sai com 0, sem regex, também é tratada como
falha, mas só quando o turno não produziu resultado: nenhuma ferramenta usada,
texto final curto (até 160 caracteres) e a frase do provedor logo no início
(ex.: `You've hit your session limit · resets 3pm`, `Service temporarily
overloaded`). **Limitação:** um agente que escreve sobre "cota" num resumo, usa
ferramentas ou responde algo mais longo nunca é confundido (houve falsos
positivos reais); em troca, uma mensagem do provedor embutida num texto maior
não é reconhecida sem `quotaRegex`/`error_regex` na instância. No `claude-code`,
um `rate_limit_event` com `status: rejected` só vira falta de cota se o turno
terminar sem resultado bem-sucedido, e um `result` com sucesso nunca é cota.

**Erro transitório sem reserva.** Sem `reserva`, erro do provedor repete a
MESMA instância até `--tentativas`, com espera curta crescente (2 s, 4 s, 6 s…
até 10 s). Com `reserva`, troca de instância na hora. Cota sem reserva não é
repetida (código 2).

**Repasse ao harness.** `--harness-arg X` (alias `--arg`, repetível) manda `X` intacto e na ordem ao processo do motor, sem separar por vírgula nem filtrar; `--seco` mostra esses args. Linhas que o motor escreve e que não viram evento, e o stderr, aparecem no log como `[raw stdout] ...`/`[raw stderr] ...`. Um prompt que começa com `/` vai literalmente ao motor (ver `session.prompt` em `02-protocol-spec.md`).

Um agente interrompido por Ctrl-C deixa a worktree e o log para o `--retomar`. Limites opcionais: `--carga-maxima`, `--max-agentes`, `--pasta-agentes` e
`--branch-base`. O protocolo JSON-RPC oferece os mesmos recursos pelo método
`run`, com os campos `nome`, `motor`, `prompt`, `retomar` e os limites. Use
`--cota-max 95` (ou `OPENHEINERSS_COTA_MAX`) para pular uma instância que já
atingiu esse percentual e tentar sua `reserva`.

### Contas AGY como instâncias

Contas não ficam no código. Os exemplos `examples/harnesses/agy-conta1.yaml` e
`agy-conta2.yaml` usam o wrapper `agy-conta.sh`, a variável `AGY_CONTA` e
encadeiam `agy-conta1` para `agy-conta2` com `reserva`. Copie-os para
`.openheinerss/harnesses/` e ajuste o caminho do wrapper ao seu checkout; os
logins permanecem nas pastas isoladas do ambiente local.

## Consultar cotas

`openheinerss limites` lê somente os eventos `rate_limits` recentes em cada
`CODEX_HOME` e os arquivos `~/.config/crom-painel/statusline-<instância>.json`
do Claude. `--json` entrega o mesmo resultado para a Central, incluindo
percentual, horário de reinício e idade do dado. Instâncias são declaradas em
`.openheinerss/harnesses` com `base: codex`/`base: claude-code` e seus envs;
nenhuma conta é embutida no programa.
