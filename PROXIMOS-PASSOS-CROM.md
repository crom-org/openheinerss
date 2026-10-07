# Próximos passos: usar o openheinerss nos projetos da Crom

Preparado em 07/10/2026 depois de uma análise rápida do código. Leia antes de mexer.

## Objetivo

O openheinerss vira a **camada que roda os agentes** (no lugar do `rodar.sh` dos projetos), e a
**Central de Tarefas** (repositório privado `crom-painel`, http://localhost:4792) continua sendo a
tela, ouvindo o WebSocket do openheinerss. Também usar o openheinerss para desenvolver o próprio
openheinerss.

## Problemas encontrados (corrigir primeiro)

1. **Adaptador do Codex errado** (`pkg/harness/codex/codex.go`, ~linha 104): chama
   `exec.CommandContext(c.ctx, "codex", "run")`. Esse subcomando não existe. O certo é:
   `codex exec -m <modelo> -c model_reasoning_effort=<esforço> --dangerously-bypass-approvals-and-sandbox "<prompt>"`
   (é como a Crom usa hoje, com o login do ChatGPT, sem custo por token). O modo API aponta para
   `https://api.openai.com/v1` (Assistants/Threads), que é pago e não é o que usamos: deixe-o
   opcional, e o padrão deve ser o CLI. Para streaming, veja `codex exec --json`.
2. **Porta 4799 em conflito**: o WebSocket do openheinerss usa 4799, que agora é a porta do painel
   do crom-animacoes (crom-painel). Mude o padrão (sugestão: **4820**) e aceite `--porta`/env.
   Lugares: `cmd/openheinerss/main.go`, `sdk/typescript/src/index.ts`, `sdk/typescript/src/react.ts`,
   `README.md`, `docs/04-sdk-any-language.md`, `docs/05-roadmap.md`, `documentacao/01,02,08,09`.
3. **Pouco teste** (10 arquivos `_test.go`, projeto de 1 dia). Antes de usar em código de produção,
   cubra pelo menos: iniciar/parar processo, streaming de eventos, retomar sessão, e cada adaptador
   com o motor `mock`.

## O que o rodar.sh faz hoje e o openheinerss ainda não

Referência: `~/Documentos/GitHub/crom-tv/.claude/agentes/rodar.sh` (e o genérico em `crom-painel/rodar/`).

- Trocar de conta/motor sozinho quando a cota acaba (agy tem 32 contas; "SEM COTA" → próxima).
- Retomar com "CONTINUAÇÃO" (`RETOMAR=1`) lendo `git status`/`git log`/`RELATORIO-AGENTE.md`.
- Uma **worktree git por agente** (`git worktree add .claude/agentes/<nome> -b agente/<nome> origin/main`).
- Respeitar a carga da máquina (não lançar com loadavg alto) e um máximo de agentes simultâneos.
- Gravar `logs/<nome>.log` ao vivo e `logs/<nome>.meta.json` (`{motor, modelo, esforco, inicio}`)
  — a Central lê esses arquivos.
- Contas do Claude por pasta: `CLAUDE_CONFIG_DIR=~/.claude-conta2` (conta 2, Sonnet 5.5 fixo).
  O adaptador `claudecode` já aceita isso (`claudecode.go` ~linha 130).

## Integração com a Central (crom-painel)

- Hoje a Central lê arquivos (`logs/*.log`, `*.meta.json`, `tarefas.json`). Caminho sugerido:
  a Central passa a se inscrever no WebSocket do openheinerss e mostra os eventos (texto, ferramenta,
  permissão pedida, fim), mantendo os arquivos como reserva.
- Limites: a Central já lê os do Codex (`~/.codex/sessions`, eventos `rate_limits`) e os do Claude
  pelo statusline (`crom-painel statusline` → `~/.config/crom-painel/statusline-<conta>.json`).
  O openheinerss pode expor isso num método `limites` em vez de cada um reler os arquivos.

## Ordem sugerida

1. Corrigir o adaptador do Codex e a porta; testes com `mock` e um teste real curto com `codex exec`.
2. Implementar no openheinerss: worktree por agente, meta.json, troca de conta na falta de cota, carga máxima.
3. Trocar o `rodar.sh` do crom-tv para chamar o openheinerss **só para o Codex**; rodar 1–2 tarefas reais pequenas e comparar.
4. Ligar a Central no WebSocket; depois migrar os outros motores e aposentar o `rodar.sh`.

## Regras da casa

- Textos e commits em português do Brasil simples.
- Nunca imprimir tokens/senhas/.env. Nunca `pkill -f` (mata o próprio shell): ache o PID exato.
- Não mexer em produção do crom-tv a partir daqui.
