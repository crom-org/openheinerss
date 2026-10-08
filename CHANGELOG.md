# Changelog

Todas as mudanças relevantes do Openheinerss desde a v1.0.0. O formato segue o
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/). Nada abaixo foi publicado ainda
(sem tag, release ou pacote): a publicação depende do ok do dono do projeto.

## [Não lançado]

### Adicionado (MCP entregue a cada harness; classificador de risco opcional)

- Os servidores de `mcp.json` (global `~/.openheinerss/` e `~/.config/openheinerss/`, e do projeto, que vence) passam a ser **entregues a cada harness, por execução**, sem editar a config pessoal: claude-code `--mcp-config <temporário 0600>` (CLI e SDK), codex `-c mcp_servers.<nome>.*` (env/headers pelo ambiente com `env_vars`/`env_http_headers`, nunca no argv), opencode `OPENCODE_CONFIG_CONTENT`. aider (sem cliente MCP) e agy (só `agy mcp add`, persistente) ficam declarados como sem suporte no catálogo (`mcp` em `catalog.list`) e no PONTE.md. Escolha: `--sem-mcp`/`--mcp a,b` em `run` e `rodar`, `semMcp`/`mcp` em `session.create`, `OPENHEINERSS_MCP`. `mcp efetivos` lista o que será entregue (env oculto). `mcp.json` aceita `type` (`http`/`sse`) e `headers`.
- Classificador de risco **opcional e desligado por padrão** (a ponte continua só túnel): `serve/run --classificar-risco`, `classificarRisco` na sessão e nos SDKs. Ligado, `agent.tool_call` e `agent.permission_request` ganham `risco` (`baixo`|`medio`|`alto`) e `motivoRisco`, por regras embutidas (rm -rf, git push, deploy, curl|sh, escrita fora da worktree = alto; leitura = baixo) e `risco.yaml` por projeto/global. Nunca bloqueia; só informa.

### Adicionado (orquestrador que espera os filhos)

- `rodar` registra os agentes filhos (`OPENHEINERSS_PAI`/`OPENHEINERSS_PAI_LOGS` no ambiente do harness; `pai`, `branch` e `worktree` no `meta.json` do filho; `logs/<pai>.filhos/`). Quando o turno do pai termina com filhos vivos, espera o FIM deles (meta.json + PID, sem polling caro) e retoma a sessão do pai com o resumo de cada filho (FIM, código, branch, relatório). Turno que diz "aguardando…" sem filhos e com worktree suja também é retomado, com aviso para terminar em primeiro plano. Flags `--esperar-filhos`/`--wait-children` (padrão ligado, até 2 h) e `--rodadas-filhos`/`--child-rounds` (padrão 5). `agentes` mostra `pai=`/`filhos=`. Filho roda em sessão própria (`setsid`); pai interrompido para os filhos vivos. Nova linha nas regras padrão sobre agentes/comandos em segundo plano. Corrige orquestradores que davam FIM com os executores ainda rodando (orq-ponte-total, orq-negar-encerra).

### Corrigido (alinhamento de `comandos`)

- `openheinerss comandos <harness>` calcula a largura das colunas pelo maior nome (teto 40) e pelo número de runes, não de bytes: nomes longos como `/crom-tv-agentes-externos` e acentos não desalinham mais a tabela.
### Corrigido (aceitação das novidades — análise 26 da Central)

- `comandos.yaml` não perde anotações entre processos: trava `comandos.yaml.lock` (flock; Windows: arquivo exclusivo) em volta de ler+alterar+gravar, temporário único (`CreateTemp` na mesma pasta) e erro real se a gravação falhar. Teste com 2 processos × 25 anotações: 50/50 (antes 23/50).
- SDKs Python e PHP: erro RPC de `session.prompt` (ex.: `/compact` no codex) vira exceção em vez de esperar `agent.complete` para sempre; teto de segurança sem eventos (`prompt_timeout`/`$promptTimeout`, 3600 s; `timeout` por chamada).
- `geracao` no envelope de todos os eventos do serve (`agent.raw`, `agent.text`, `agent.tool_call`, `agent.complete`… e `orq.*`), WebSocket e stdio.
- `rodar` com regras padrão e prompt `/comando`: o comando vai primeiro e as regras por outro canal (claude: `--append-system-prompt`; codex: `-c developer_instructions`; aider: `--read`; demais: depois do comando).
- Anotações mascaram segredos (`***`, como o `--seco`) ao gravar e ao listar; `comandos.yaml` com 0600 e pasta nova com 0700.

### Adicionado (catálogo de comandos por harness)

- `openheinerss comandos <harness> [--json] [--cwd]` (alias `commands`), `comandos anotar <harness> </cmd> "<texto>"` e `comandos confirmar <harness> </cmd>`; no servidor, `harness.comandos`, `harness.comandos.anotar` e `harness.comandos.confirmar`; SDKs TS/Py/PHP `listCommands`, `annotateCommand`, `confirmCommand`. Cada harness base (claude-code, codex, opencode, aider, agy) tem o catálogo embutido com descrição em pt-BR e o repasse (`literal`, `traduzido`, `sem_equivalente`); instâncias herdam da `base:`. Comandos e skills instalados no harness (`.claude/commands`, `skills/`, `command/` do opencode, `prompts/` do codex) são descobertos em tempo de execução. Anotações e `confirmado` ficam em `comandos.yaml` (`~/.config/openheinerss/` e a pasta de `--config`/`OPENHEINERSS_CONFIG`), gravado preservando comentários. Pedido do usuário para a crom-central.

### Adicionado (negar = encerrar)

- `serve --negar-encerra` (alias `--deny-ends`) e campo opcional `encerrar` em `rodar.decidir` (vence a flag naquela decisão): uma negação termina a execução com `FIM HH:MM código 3`, `meta.json` com `codigo` 3 e `motivo` `"negado"`, e `orq.fim` com `motivo` `"negado"`, sem nova tentativa nem reserva. Sem a opção, nada muda. SDKs TS/Py/PHP: `decideRun(..., encerrar)`. Pedido da crom-central.

### Adicionado (pasta de configuração)

- Flag global `--config <pasta>` (alias `--configuracao`) e variável `OPENHEINERSS_CONFIG`: apontam a pasta de instâncias (`harnesses/`) e `motores.yaml` para todos os comandos (`limites`, `harness`, `rodar`, `run`, `motores`, `serve`), sem depender da pasta atual. Ordem: flag > variável > busca atual. Pedido da crom-central (`limites --json` mudava conforme o cwd).

### Corrigido (5ª verificação do `rodar`)
- Cota/sobrecarga só valem em canal de erro (evento de erro, stderr do motor sem trabalho útil, resultado com
  `is_error`); o texto livre do agente nunca é examinado. `agy` só falha por evento de erro/stderr.
- Chaves (`--arquivo-chaves`) vão no ambiente de cada execução, sem `os.Setenv` global nem trava compartilhada.
- Esperas de `.worktree.lock` e `.vagas.lock` respeitam o cancelamento.
- `--seco` mascara todo valor de ambiente (exceto `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `PWD`…), mostra `-p` do `agy`
  e coloca caminhos com espaço entre aspas.
- Missões (`missao-*`) rodam num clone `--shared` descartável (sem escrita nas refs do repositório) e o
  `RELATORIO-AGENTE.md` é copiado para `relatorios/<nome>.md` antes de apagar a pasta.
- Meta do codex mostra modelo/esforço reais; filhos do motor morrem junto com o runner (Pdeathsig no Linux);
  falha ao gravar `--eventos-log` avisa; instâncias custom da raiz carregam de dentro de uma worktree.

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
- SDKs TypeScript, Python e PHP **1.1.0**: `registerHarness`, `listHarnesses`, instâncias, `run`,
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
