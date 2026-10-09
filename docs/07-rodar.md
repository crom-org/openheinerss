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

Por padrão, o runner acrescenta regras curtas ao prompt: trabalhar somente dentro da pasta/worktree do agente e não fazer buscas fora dela (`find /`, `find ~`, `locate` ou varreduras de disco). Use `--sem-regras-padrao` (`--no-default-rules`) para desligá-las.

Quando o prompt é um `/comando` (ex.: `--texto "/review agora"`), ele continua sendo o começo do que o harness recebe e as regras vão por outro canal: **claude-code** (e instâncias com essa base) → `--append-system-prompt`; **codex** → `-c developer_instructions="..."`; **aider** → `--read <pasta>/logs/<nome>.regras.md`; **opencode, agy, custom e mock** → no mesmo prompt, depois do comando (separadas por uma linha em branco). Prompt comum continua com as regras antes do texto. Também é possível definir `regras_padroes: caminho/para/regras.txt` em `.openheinerss/config.yaml`; `sem_regras_padroes: true` desliga as regras pela configuração.

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

### Agentes filhos (orquestrador que espera)

Um `rodar` passa ao harness `OPENHEINERSS_PAI=<nome>` e `OPENHEINERSS_PAI_LOGS=<pasta de logs>`. Um
`openheinerss rodar` lançado de dentro dele (agente filho, em primeiro ou segundo plano) grava `"pai"` no
próprio `meta.json` (junto com `branch` e `worktree`), se registra em `logs/<pai>.filhos/<filho>` e roda
numa sessão própria (`setsid`), para não morrer junto com o grupo do harness do pai.

Quando o turno do pai termina bem e há filhos vivos, o `rodar` espera cada um dar FIM (lê o `meta.json`
e testa o PID a cada 2 s; nada de consumo de CPU) e **retoma a sessão do pai** (`--resume` nativo do
claude/codex; sem sessão nativa, reenvia o prompt) com a mensagem `--- RETOMADA AUTOMÁTICA (rodada N de M) ---`,
que lista FIM, código, motivo, branch, worktree, relatório e log de cada filho. Se o turno diz que vai
esperar ("aguardando os executores", "em segundo plano"…) sem filho nenhum e a worktree tem mudanças sem
commit, o pai é retomado com o aviso de que nada vai acordá-lo e deve terminar em primeiro plano.
Sem filhos e sem esse caso, nada muda. Se o pai é interrompido, os filhos vivos são parados.

- `--esperar-filhos sim|nao|<duração>` (alias `--wait-children`; env `OPENHEINERSS_ESPERAR_FILHOS`): padrão
  ligado, espera até 2 h; vencido o prazo, retoma assim mesmo e marca os filhos como `AINDA RODANDO`.
- `--rodadas-filhos N` (alias `--child-rounds`; env `OPENHEINERSS_RODADAS_FILHOS`): máximo de retomadas (padrão 5).
- `--filhos-obrigatorios` (alias `--require-children`; `filhosObrigatorios` em `rodar.iniciar`): se algum filho
  terminou com código ≠ 0, morreu sem FIM ou ainda roda quando o pai acaba, o pai termina com **código 4**
  (`FIM HH:MM código 4`, saída do processo 4), `motivo` `"filho falhou"` e `filhos_falhos` no `meta.json`, e a
  linha `filho falhou (--filhos-obrigatorios): a (código 1); b (ainda rodando, PID n)` no log. Sem a flag, o
  código do pai é o do próprio turno.
- Filhos órfãos: o pai nunca dá FIM em silêncio com filho vivo (espera vencida, limite de rodadas ou
  `--esperar-filhos nao`). O log e o stderr ganham `AVISO: o pai terminou com N agente(s) filho(s) ainda
  rodando (órfãos): …`, o `meta.json` ganha `filhos_orfaos` (e `motivo` `"filhos órfãos"` se não houver outro),
  e o servidor emite `orq.filhos_orfaos` antes do `orq.fim`.
- `openheinerss agentes` mostra `pai=`, `filhos=`, `órfãos=` (filhos vivos de um pai que já terminou) e marca
  o filho com `ÓRFÃO(pai terminou)`; no `--json`, `pai`, `filhos`, `orfaos` e `orfao`.

Códigos de FIM do `rodar`: 0 ok, 1 erro, 2 sem cota, 3 negado (`--negar-encerra`), 4 filho falhou
(`--filhos-obrigatorios`), 5 parado pelo detector (retomável com `--retomar`), 130 interrompido.

As regras padrão do prompt incluem: "Se lançar agentes filhos ou comandos em segundo plano, o openheinerss te
acorda quando eles terminarem; não encerre dizendo que vai esperar sem ter lançado nada."

### Meta.json, órfãos e checkpoints

O `meta.json` de cada agente ganhou campos opcionais (um meta antigo continua legível): `ultimo_evento_em` (RFC3339, atualizado a cada evento do motor, no máximo uma escrita a cada 5 s), `head` (sha do HEAD da worktree no fim de cada turno/tentativa), `inicio_pid` (horário de início do processo, campo 22 de `/proc/<pid>/stat`; vazio fora do Linux, para não confundir um PID reutilizado) e `checkpoints` (`n`, `ref`, `sha`, `em`, `motivo`).

**Órfão.** Meta sem `fim` cujo processo morreu (ou cujo PID agora é de outro processo, por `inicio_pid`) aparece como `órfão` em `agentes listar`; a listagem grava `fim`, `codigo` -1 e `motivo` `"órfão"` e a vaga volta ao limite. `agentes parar` de um órfão não manda sinal: só fecha o meta. Um filho cujo pai morreu sem FIM aparece como `PAI MORTO` (`pai_morto` no `--json`) e não é alterado.

**Checkpoint git-sombra.** No começo e no fim de cada turno/tentativa o `rodar` grava, na worktree do agente, um commit da árvore inteira (arquivos novos incluídos, `.gitignore` respeitado) em `refs/openheinerss/<nome>/<n>`, usando um índice temporário: o índice, o HEAD e a branch do agente não mudam, e só grava se a árvore mudou desde o último. Sem git, nada é feito (o `pkg/checkpoint`, por cópia de arquivos, segue como alternativa do servidor e dos SDKs).

- `openheinerss agentes checkpoints <nome>` lista `n`, quando, motivo e o `git diff --shortstat` contra o anterior.
- `openheinerss agentes desfazer <nome> [n]` restaura a worktree do agente (nunca o repositório principal) ao checkpoint `n`; sem `n`, ao anterior ao último. Antes, guarda o estado atual como checkpoint `antes-de-desfazer` (use o `n` dele para refazer). Se o checkpoint tem outro HEAD, a branch volta ao pai registrado com `git reset --soft` (os commits de depois seguem alcançáveis pelo checkpoint `antes-de-desfazer`); arquivos não rastreados somem com `git clean -fd` (sem `-x`: ignorados ficam). Recusa se o agente está rodando, a menos que `--forcar`.
- `ApagarCheckpoints(dir, nome)` (Go) remove as refs do agente; use-o ao apagar a worktree.

### Arquivo de estado e retomada

No fim de cada turno/tentativa o `rodar` grava `logs/<nome>.estado.md` (sem custo de modelo): objetivo, prompt original (cortado em 4000 caracteres), HEAD, `git diff --stat` contra a base da worktree, os últimos 6 trechos de texto do log e o `RELATORIO-AGENTE.md` da worktree como "resumo do agente". Segredos são mascarados. O cabeçalho guarda `em`, `turno`, `head`, `head_resumo` e `turno_resumo`.

Ao retomar (`--retomar`), trocar de motor ou recomeçar em sessão nova pelo limite de contexto, esse estado vai no lugar do texto fixo "CONTINUAÇÃO", para qualquer harness. O git manda; o resumo é só contexto. O resumo é descartado se o HEAD mudou depois dele (commits mais novos que o relatório) ou se está mais de 2 turnos atrasado (`OPENHEINERSS_RESUMO_MAX_TURNOS`); aí vai o histórico: diff, últimos eventos e prompt original. Sem git e sem log, vale o texto fixo antigo.

### Limite de contexto por projeto e por harness

O contexto grande é onde a conta mais gasta. A chave `contexto:` do `config.yaml` define um limite de tokens
e o que fazer ao passar dele; vale no projeto (`<repo>/.openheinerss/config.yaml`) e no global
(`--config`/`OPENHEINERSS_CONFIG`, ou `~/.config/openheinerss/config.yaml` e `~/.openheinerss/config.yaml`,
este vence), e o projeto vence o global:

```yaml
contexto:
  padrao: {limite_tokens: 150000, acao: aviso}   # acao: aviso | nova-sessao
  harnesses:
    claude-conta2: {acao: nova-sessao}            # nome de instância ou de harness base
    codex: {limite_tokens: 200000}
```

Cada campo é resolvido separadamente, nesta ordem: projeto (instância, base, `padrao`), depois global
(instância, base, `padrao`). Sem nada, fica desligado. `--limite-contexto N` (`0` desliga) e
`--acao-contexto aviso|nova-sessao` vencem a configuração. `openheinerss config contexto [--harness X]` mostra
o valor efetivo e de onde vem cada campo. Arquivo inválido é erro e a execução nem começa.

O tamanho do contexto é o `input` do último evento de uso (o `total` se só ele vier). Ao chegar no limite:
`aviso` escreve `[contexto] N tokens >= limite L (origem: projeto|global|flag)` no log, uma vez por sessão, e
uma linha `orq.contexto` no log de eventos (`--eventos-log`). `nova-sessao` avisa, encerra o turno e recomeça o
MESMO motor em sessão nova (sem a sessão nativa), com o prompt original mais o texto de continuação. Não conta
como tentativa nem como falha de cota; no máximo 3 reinícios por execução (depois só avisa), registrados em
`reinicios_contexto` no `meta.json`.

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

### Agente parado

Enquanto o motor roda, o `rodar` confere a cada 60 s quatro sinais baratos (nenhum custa token): **S1** log sem escrita (mtime), **S2** worktree sem mudança (HEAD, hash do `git status --porcelain` e o maior mtime dos arquivos alterados), **S3** mesmo último trecho do log, **S4** CPU do motor e dos filhos (`/proc/<pid>/stat`, só Linux; abaixo de 5% de um núcleo entre duas leituras conta como "~0"). As linhas `[parado]` que o próprio detector escreve não contam como atividade.

- S1+S2+S3 por `aviso` (padrão 10 min): linha `[parado] …` no log e, com `--eventos-log`, `orq.parado <nome> nivel=aviso …`.
- O mesmo por `parar` (padrão 20 min) com `acao: parar` e CPU ~0: SIGTERM no grupo do motor, `meta.json` com `motivo` `"parado"` e fim com **código 5** (erro retomável: rode de novo com `--retomar`). Com a CPU ativa (build ou teste longo) só avisa, nunca para.
- Só S3 com o log ainda ativo: aviso de **laço** ("mude de abordagem"); não interrompe.

Configuração (o projeto vence o global, campo a campo; depois vêm as variáveis de ambiente `OPENHEINERSS_PARADO_AVISO_MIN`, `OPENHEINERSS_PARADO_PARAR_MIN`, `OPENHEINERSS_PARADO_ACAO` e os padrões):

```yaml
parado:
  aviso_min: 10      # 0 desliga o detector
  parar_min: 20      # 0 nunca interrompe
  acao: aviso        # aviso (padrão) ou parar
```

As flags `--parado-aviso` e `--parado-parar` (duração, ex. `15m`; `0` desliga) vencem tudo; dar `--parado-parar` maior que zero liga `acao: parar`. O `rodar` grava a última checagem em `logs/<nome>.parado.json`: `agentes listar` mostra `parado` só com S1 (15 min, `OPENHEINERSS_LOG_PARADO_MIN`) mais S2 e S3 parados, e `lento` quando só o log está sem escrita (ou o detector não está olhando).
