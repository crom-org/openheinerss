# Changelog

Todas as mudanças relevantes do Openheinerss desde a v1.0.0. O formato segue o
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/). Nada abaixo foi publicado ainda
(sem tag, release ou pacote): a publicação depende do ok do dono do projeto.

## [Não lançado]

### Adicionado
- **`rodar`** (apelidos `launch`, `dispatch`): executa uma missão com worktree `agente/<nome>`, log em
  `logs/<nome>.log`, `logs/<nome>.meta.json` (gravação atômica), linha final `FIM hh:mm código N`,
  `--retomar`/`RETOMAR=1`, reservas, até 4 tentativas e texto de continuação. O código de saída do
  processo é o código do `FIM` (0 ok, 1 erro, 2 sem cota, 130 parado).
- **Harness custom e instâncias por arquivo** em `.openheinerss/harnesses/*.yaml` (e
  `~/.config/openheinerss/harnesses`): `base` (herda um harness do código ou outra instância),
  `command`/`args`/`env`, `model`, `effort`, `prompt` (`stdin` ou `argument`), `finishRegex`,
  `quotaRegex` e `reserva`. Contas e provedores deixaram de existir no código; `harness add`,
  `harness list` e `harness test` completam o fluxo.
- **`harness test --todos`** (`--all`): matriz de testes reais lida do catálogo (embutidos + instâncias),
  com `--pular`, `--json`, `--timeout` e `--incluir-principal`; `harness test <nome> --retomar` testa
  a retomada em dois turnos; `scripts/teste-real.sh`.
- **`limites`** (`limits`) e `rodar --cota-max`: lê as cotas locais de Codex e Claude (percentual, horário de
  reinício, idade do dado) e pula instâncias acima do percentual.
- **Limites de execução**: `--max-agentes`, `--carga-maxima`, `--quando-carga-abaixo`, `--tentativas`;
  falta de cota sem `reserva` encerra na hora com código 2 e guarda o aviso do motor (`resets 9am`).
- **`agentes`** (`agents`) com `listar`, `ver` e `parar`: estado, duração e última linha lidos de logs e
  `meta.json`; `parar` mexe só no agente indicado.
- **`motores`** (`engines`) e `run --papel/--motor/--esforco` com `.openheinerss/motores.yaml` opcional.
- **Servidor**: métodos `rodar.iniciar`, `rodar.listar`, `rodar.parar`, `rodar.decidir`, `limites.obter`,
  `harness.listar`, `eventos.assinar` e eventos `orq.inicio`, `orq.progresso`, `orq.erro`,
  `orq.precisa_decisao` e `orq.fim` no WebSocket e no STDIO; agentes lançados fora do servidor também
  geram eventos (leitura periódica dos logs).
- **`session.resume`** e `run --retomar <id>` com retomada nativa (Codex, Claude); transcript em
  `.openheinerss/sessions/<id>.jsonl`; checkpoints por cópia de arquivos (sem `git checkout`).
- `suggestedFix` em `agent.error`, duração e totais em `agent.complete`.
- Aliases em inglês para todos os comandos e flags (`doctor`, `init`, `serve`, `run`, `harness`, `mcp`...).
- `openheinerss version` com versão, commit e data; `docs` gera o manual do CLI (`docs/09-cli.md`),
  com teste que falha se ele divergir; instalação por `install.sh` (checksum SHA-256, fallback `go build`)
  e GoReleaser multiplataforma.
- SDKs TypeScript, Python e PHP **0.2.0**: `registerHarness`, `listHarnesses`, instâncias, `run`,
  `listRuns`, `stopRun`, `getLimits` e `subscribeEvents` para os eventos `orq.*`.
- `rodar --texto` (`--text`) e o campo `texto` de `rodar.iniciar`/`run`: prompt em texto, sem arquivo (os SDKs passaram a usar `texto` nos exemplos; `prompt` continua sendo o caminho de um arquivo).
- `install.sh --local`: compila o clone em vez de baixar o release.
- `docs/VERIFICACAO.md` (roteiro de verificação do zero), `docs/COMO-DESENVOLVEMOS.md`, `docs/LACUNAS.md`
  e `docs/TESTES-REAIS.md`; `scripts/ws-cliente.mjs`, cliente WebSocket de exemplo.

### Alterado
- Porta padrão do WebSocket: **4820** (era 4799). `OPENHEINERSS_PORTA` muda a porta.
- Codex usa `codex exec --json` (não a Assistants API); Claude Code em CLI com `stream-json` ao vivo;
  OpenCode com `--format json`; Aider não interativo; AGY revisado. Todos os CLIs rodam em grupo de
  processos próprio, e timeout/Stop não deixa filhos presos.
- `docs/` é a única documentação técnica; o conteúdo antigo de `documentacao/` foi removido.
- A lista do `harness test --todos` vem do catálogo, não do código. `harness test --todos` termina com
  código 1 se algum teste não deu OK. `harness test` único falha (código 1) quando o motor reporta erro.
- O cache de prompt só é desligado para provedores que não são a Anthropic direta.

### Corrigido
- Detecção de cota só por evento de erro (nunca pelo texto que o agente escreve) e `overage` desligado
  não é falta de cota; avisos do stderr do Codex/CLIs não derrubam a tarefa.
- Harness custom: `~/` agora vale em `command` e em `env` mesmo sem `base`; o processo é colhido
  (`Wait`, sem zumbi); saída com erro vira `process_error` com o fim do stderr; cota no stderr é detectada;
  um evento de fim nunca é descartado quando o consumidor está lento.
- Modo SDK do Claude: o worker Node é colhido, e o caminho do SDK não está mais embutido no código; ele vem de
  `OPENHEINERSS_CLAUDE_SDK_PATH` (que pode ser declarado no `env` do arquivo da instância e é usado também na
  checagem de pré-requisitos) ou de `~/.openheinerss/shims/node_modules`.
- Parar um agente ou o servidor não deixa filhos órfãos: o cancelamento mata o grupo de processos inteiro do
  motor. `agentes parar` usa SIGTERM (um shell não interativo entrega `&` com SIGINT ignorado, e o agente não parava).
- `rodar` não faz mais `fsync` a cada linha do log (levava segundos por escrita e atrasava o fim do processo
  com o disco ocupado).
- `rodar`: nome de agente com `/`, `..` ou espaço é recusado; o prompt é lido antes de criar a worktree;
  Ctrl-C/SIGTERM fecham log e `meta.json` (`FIM ... código 130`) e param o motor; `--cota-max` ignora
  janelas de cota já renovadas e, com todos os motores acima do limite, termina com código 2.
- `agentes parar` não sinaliza mais o servidor quando o agente foi lançado por ele (use `rodar.parar`) e
  não escreve o `FIM` duas vezes.
- Servidor: WebSocket só aceita origens locais (ou as de `OPENHEINERSS_ORIGENS`), impedindo que uma
  página web abra agentes na máquina; ao desligar, para as sessões e espera os agentes fecharem o log;
  sessões abortadas não deixam goroutine presa; `session.resume` usa o contexto da sessão.
- `mcp add` (e o RPC `mcp.add`) travava para sempre (trava adquirida duas vezes); agora grava de forma atômica, cria `.openheinerss/` se faltar e recusa um `mcp.json` corrompido em vez de dar pânico.
- `serve` tem um limite de 10 s para desligar; IDs de sessão com `/` ou `..` são recusados no transcript.
- Checkpoint inicial não copia mais `node_modules`, `vendor`, `dist`, `build`... nem arquivos acima de 10 MB
  (limite total de 256 MB, marcado como `parcial`); falha no checkpoint vira aviso, não impede a sessão.
- `run`: o fim do turno não se perde mais (canal sem buffer), erros do agente aparecem, Ctrl-C encerra o
  motor. `harness test mock` não trava mais esperando permissão. `harness list` sai em ordem estável.

### Removido
- Código morto (`builtinSpec`) e a lista fixa de instâncias no `--todos`.

## [1.0.0] — 2026-10-06
- Primeira versão: servidor JSON-RPC 2.0 (STDIO e WebSocket), harnesses `mock`, `claude-code`, `opencode`,
  `codex`, `agy` e `aider`, hub MCP, `doctor`, `init`, `run`, `harness list`, e SDKs TypeScript, Python e PHP.
