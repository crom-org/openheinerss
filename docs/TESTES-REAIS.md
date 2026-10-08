# Testes reais dos harnesses

## Provas da etapa 4a

Com timeout de 90s, o Codex passou nos dois turnos nativos: o primeiro (`guarde a palavra ABACAXI e responda só OK`) respondeu `OK` e `openheinerss run --retomar sess_831876d90aed 'qual palavra pedi para guardar?'` respondeu `ABACAXI`. O transcript dessa sessão ficou com 11 linhas.

A prova equivalente de `claude-conta2` foi iniciada com timeout de 90s, mas a conta respondeu `You've hit your session limit · resets 9am (America/Bahia)`. Não foi marcada como aprovação nem como falha de retomada: está bloqueada por cota externa e deve ser repetida após a renovação.

`openheinerss motores` foi executado sem `.openheinerss/motores.yaml`: listou os seis motores e informou `Nenhum papel` sem erro.

Data da execução: 08/10/2026 (America/Bahia). Cada caso usou um único prompt curto (`responda só OK`); o teste em lote foi sequencial. A conta principal do Claude ficou excluída no lote padrão.

## Versões observadas

| CLI | `--version` |
|---|---|
| codex | `codex-cli 0.159.2` |
| claude | `2.1.293 (Claude Code)` |
| opencode | `1.18.33` |
| aider | `aider 0.86.2` |
| agy | `1.3.1` |

## Matriz

Execução: `openheinerss harness test --todos --timeout 20s`.

| nome | base | modo | resultado | tempo | tokens | motivo |
|---|---|---|---|---:|---:|---|
| codex | codex | cli | OK | 3,641 s | 13.279 | texto e fim de sucesso confirmados |
| codex2 | codex | cli | OK | 4,317 s | 11.757 | texto e fim de sucesso confirmados |
| claude-conta2 | claude-code | cli | OK | 2,121 s | 18.595 | texto e fim de sucesso confirmados |
| claude-conta2 | claude-code | sdk | falha | 20,017 s | 0 | tempo esgotado |
| opencode | opencode | cli | OK | 7,612 s | 13.724 | texto e fim de sucesso confirmados |
| opencode-gratis | opencode | cli | OK | 8,274 s | 13.763 | texto e fim de sucesso confirmados |
| aider | aider | cli | falha | 20,002 s | 0 | tempo esgotado; CLI instalado, sem mensagem de cota/login |
| agy | agy | cli | OK | 7,480 s | 0 | texto e fim de sucesso confirmados; motor não informou uso |

O modo JSON também foi exercitado com `--json --pular codex2,agy --timeout 5s`; ele produziu a mesma matriz em JSON. O filtro `--incluir-principal` foi exercitado separadamente: `claude-code` CLI respondeu OK em 4,729 s com 21.027 tokens; o modo SDK atingiu timeout em 5 s. Isso foi um teste explícito de uma única frase, não uma tarefa pesada.

## Testes extras

`scripts/teste-real.sh` foi executado com timeout de 5 s por caso para manter os prompts curtos e evitar espera prolongada:

- retomada Codex: primeiro e segundo turnos terminaram por timeout;
- retomada Claude conta2: primeiro e segundo turnos terminaram por timeout;
- Claude conta2 SDK: timeout;
- `rodar` real Codex em repositório temporário: `FIM ... código 0`;
- `rodar` real OpenCode grátis em repositório temporário: `FIM ... código 0`.

Os repositórios temporários e logs foram removidos pelo próprio script. As instâncias foram copiadas para o temporário antes do teste do OpenCode, pois `rodar` resolve configurações relativas ao diretório corrente.

Em uma repetição isolada da retomada Claude conta2 com 20 s, o primeiro turno confirmou OK e o segundo terminou sem texto; portanto a retomada não foi declarada bem-sucedida.

## Correções encontradas

- Codex, Claude, OpenCode, Aider e AGY agora iniciam o CLI em grupo de processos próprio; timeout/Stop envia sinal ao grupo e não deixa filhos segurando pipes.
- O Claude CLI agora guarda o `session_id` real recebido no primeiro stream para a retomada seguinte.
- O worker SDK traduz `permissionMode=ask` do protocolo para `manual`, nome aceito pela versão instalada do Claude Code.
- Testes unitários cobrem a configuração de grupo do Codex e a tradução do modo de permissão SDK.

Os timeouts de Claude SDK e Aider foram registrados como falha por timeout, não como falta de CLI, cota ou login: ambos tinham `--version` disponível e não emitiram uma mensagem confiável dessas categorias.

## Rodada de 08/10 09:40 (depois da etapa 4a)

`openheinerss harness test --todos --timeout 90s`: 7 de 8 OK — codex, codex2, claude-conta2 (CLI e SDK), opencode, opencode-gratis e agy.
`aider` (configuração padrão do usuário) falhou por causa **externa**: o provedor recusou por créditos ("request requires more credits, or fewer max_tokens"); o openheinerss classificou como "sem cota" em 3,4 s.
Como resolver: usar a instância `aider-gratis` (`.openheinerss/harnesses/aider-gratis.yaml`, modelo `openrouter/nvidia/nemotron-3-ultra-550b-a55b:free`), que respondeu de verdade (611 tokens enviados, 47 recebidos, depois de uma nova tentativa automática por sobrecarga do provedor), ou pôr créditos/outro modelo no aider.

## Rodada de 08/10 09:50 — 8/8 OK (decisão do usuário: aider via `aider-gratis`)

`openheinerss harness test --todos --pular aider --timeout 150s` (lista lida do catálogo: embutidos + instâncias do usuário):

| nome | base | modo | resultado | tempo | tokens |
|---|---|---|---|---:|---:|
| agy | agy | cli | OK | 25,3 s | — |
| aider-gratis | aider | cli | OK | 22,6 s | — |
| claude-conta2 | claude-code | cli | OK | 27,0 s | 6 |
| claude-conta2 | claude-code | sdk | OK | 6,2 s | 6 |
| codex | codex | cli | OK | 1 min 57 s | 13.935 |
| codex2 | codex | cli | OK | 46,4 s | 12.425 |
| opencode | opencode | cli | OK | 14,1 s | 13.817 |
| opencode-gratis | opencode | cli | OK | 13,2 s | 13.649 |

Tempos altos com a máquina em carga ~20. Corrigido: a lista do `--todos` era fixa no código (instâncias embutidas) e o `--pular` pulava também as instâncias da mesma base.

## Rodada da etapa 7 (08/10 11:08) — 8/8 OK

Binário compilado do código da revisão final; máquina com carga ~5. Comando: `openheinerss harness test --todos --pular aider --timeout 150s` (lista lida do catálogo; só as contas de teste, nunca a principal do Claude). Total: 86,9 s, código de saída 0.

| nome | base | modo | resultado | tempo | tokens |
|---|---|---|---|---:|---:|
| agy | agy | cli | OK | 18,4 s | — |
| aider-gratis | aider | cli | OK | 15,2 s | — |
| claude-conta2 | claude-code | cli | OK | 3,0 s | 6 |
| claude-conta2 | claude-code | sdk | OK | 8,2 s | 6 |
| codex | codex | cli | OK | 22,3 s | 13.277 |
| codex2 | codex | cli | OK | 4,0 s | 12.451 |
| opencode | opencode | cli | OK | 10,6 s | 13.886 |
| opencode-gratis | opencode | cli | OK | 5,2 s | 13.553 |

**`rodar` real com o Codex num repositório temporário** (`git init`, um commit vazio): `openheinerss rodar real-codex codex --texto "Crie o arquivo REAL-OK.txt contendo apenas OK e faça um commit git com essa alteração. Não faça mais nada." --tentativas 1 --pasta-agentes <tmp>/agentes`. Resultado: `FIM real-codex código 0` em 48,6 s, worktree `agente/real-codex` com o commit `Add REAL-OK marker` e `REAL-OK.txt` contendo `OK`; `meta.json` com `"motor":"codex"`, `"tentativa":1`, `"codigo":0`; log terminado em `[completo]` e `FIM 11:08 código 0`. O repositório temporário foi apagado depois.

**O que a revisão mudou nestes testes.** Na primeira tentativa desta rodada o `claude-conta2` no modo SDK deu `sem CLI`: o caminho do `@anthropic-ai/claude-agent-sdk` estava escrito no código do adaptador (uma pasta desta máquina) e foi removido. O caminho agora é configuração da instância: `.openheinerss/harnesses/claude-conta2.yaml` declara `OPENHEINERSS_CLAUDE_SDK_PATH` no `env`, e a checagem de pré-requisitos passou a enxergar o `env` da instância. Com isso o SDK voltou a dar OK (8,2 s). Outra correção que apareceu aqui: o `rodar` fazia `fsync` a cada linha do log, o que com o disco ocupado levava segundos por escrita; foi removido.

**Ainda não provado com conta real.** A troca automática por `reserva` quando a cota acaba de verdade (no uso real de 08/10 ela foi feita à mão). Está provada com harness custom falso (roteiro em [VERIFICACAO.md](VERIFICACAO.md), passo 7, e testes do pacote `orchestrator`) e fica para a próxima vez que uma conta esgotar.
