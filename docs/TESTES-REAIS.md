# Testes reais dos harnesses

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
