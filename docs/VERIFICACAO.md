# Roteiro de verificação do zero

Para uma pessoa ou um agente conferir o Openheinerss sem confiar em ninguém. Cada passo traz o comando exato, o resultado esperado e o tempo aproximado (máquina sem carga alta; o `mock` simula latência e pode levar até ~10 s). Os passos 1 a 12 **não gastam token nem cota**: usam o harness `mock` e scripts falsos. O passo 14 (opcional) usa motores reais.

Pré-requisitos: Go 1.22+, git, e para os passos de cliente: Node 22+ (o `WebSocket` embutido), Python 3 e, opcionalmente, PHP 8. Todos os comandos rodam num shell bash.

## 0. Preparar variáveis e um repositório de teste (10 s)

```bash
REPO=/caminho/do/clone/openheinerss          # o clone que você quer verificar
DEMO=$(mktemp -d)                            # repositório descartável para as missões
cd "$DEMO" && git init -q -b main && git -c user.name=t -c user.email=t@t commit -q --allow-empty -m base
mkdir -p .claude/agentes/prompts .openheinerss/harnesses
echo "Siga as regras do projeto." > .claude/agentes/prompts/_regras.md
```

Esperado: nenhuma saída. `git log --oneline` mostra um commit `base`.

## 1. Instalar (compilar o clone) (10–60 s)

```bash
cd "$REPO" && go build ./... && go vet ./... && ./install.sh --local   # instala em ~/.local/bin
# ou, sem tocar no seu PATH:
OPENHEINERSS_INSTALL_DIR="$DEMO/bin" ./install.sh --local
OH="$DEMO/bin/openheinerss"                 # (use OH=openheinerss se instalou em ~/.local/bin)
"$OH" version
```

Esperado: `Instalado em .../openheinerss (build local).` e `openheinerss dev (commit desconhecido, data desconhecida)`. Sem `--local`, o script baixa o release publicado (v1.0.0, anterior às novidades) e confere o SHA-256.

## 2. `doctor` (1 s)

```bash
cd "$DEMO" && "$OH" doctor
```

Esperado: título `🩺 Openheinerss Doctor`, uma linha `✅ ... Disponível` para Git, Go, Node.js, Claude Code CLI, OpenCode CLI e Docker (`❌` para o que não estiver instalado, com a correção sugerida) e um `Resumo`.

## 3. Harnesses embutidos (2 s)

```bash
"$OH" harness list
"$OH" harness test mock --prompt "responda OK"; echo "exit=$?"
```

Esperado: `harness list` mostra, em ordem alfabética, `agy`, `aider`, `claude-code`, `codex`, `mock` e `opencode` (origem `embutido`). `harness test mock` imprime `thinking`, `text`, `permission` (o teste nega sozinho), `text` e `complete {harness-test permission_denied ...}`, e `exit=0`.

## 4. Instância nova por arquivo (3 s)

```bash
cat > .openheinerss/harnesses/meu-mock.yaml <<'EOF'
name: meu-mock
base: mock
model: modelo-de-teste
effort: low
EOF
"$OH" harness list | grep meu-mock
"$OH" harness test meu-mock | tail -1
```

Esperado: `meu-mock                 custom     meu-mock` e uma última linha `complete {harness-test permission_denied ...}`. Nenhuma recompilação: a instância é só o arquivo. (Instâncias reais de conta/provedor seguem o mesmo formato: `base: claude-code`, `env: {CLAUDE_CONFIG_DIR: ~/.claude-contaN}`.)

Registro de servidores MCP (só edita `.openheinerss/mcp.json`; o Openheinerss não hospeda servidores MCP):

```bash
"$OH" mcp add fs npx -y servidor-fs
"$OH" mcp list
```

Esperado: `✅ Servidor MCP 'fs' registrado com sucesso em .openheinerss/mcp.json!` e, na lista, `• fs              [stdio] npx`, ambos em menos de 1 s.

## 5. `rodar`: worktree, log, FIM e meta.json (2–10 s)

```bash
echo "Diga olá." > .claude/agentes/prompts/ola.md
"$OH" rodar ola meu-mock; echo "exit=$?"
git worktree list
tail -3 .claude/agentes/logs/ola.log
cat .claude/agentes/logs/ola.meta.json
```

Esperado:
- `FIM ola código 0`, `log: .../.claude/agentes/logs/ola.log` e `exit=0`;
- `git worktree list` mostra `.claude/agentes/ola` na branch `agente/ola`;
- o log termina em `[completo]` e `FIM hh:mm código 0`, e tem a linha `### tentativa 1 (hh:mm) motor meu-mock`;
- o `meta.json` é uma linha com `"motor":"meu-mock"`, `"modelo":"modelo-de-teste"`, `"esforco":"low"`, `"tentativa":1`, `"pid"`, `"inicio"`, `"fim"` e `"codigo":0`.

Prompt direto, sem arquivo: `"$OH" rodar rapido mock --texto "responda OK"` termina com `FIM rapido código 0`. Um nome inválido é recusado: `"$OH" rodar ../x mock --texto oi` falha com `nome de agente inválido`, e `"$OH" rodar sem-prompt mock` falha com `abrir prompt` **sem** criar worktree.

## 6. Retomar (2–10 s)

```bash
"$OH" rodar ola meu-mock --retomar
grep -c '### tentativa' .claude/agentes/logs/ola.log
grep -c 'CONTINUAÇÃO' .claude/agentes/logs/ola.log
```

Esperado: `FIM ola código 0`; o log **foi preservado** (`2` linhas `### tentativa`) e o prompt da segunda execução contém o texto `--- CONTINUAÇÃO ---` (`1` ou mais ocorrências). Sem `--retomar` o log é zerado.

## 7. Reserva por cota, com harness custom falso (3 s)

```bash
cat > .openheinerss/harnesses/sem-cota.yaml <<'EOF'
name: sem-cota
command: sh
args: ["-c", "cat >/dev/null; echo 'You have hit your session limit · resets 9am'"]
quotaRegex: "session limit"
reserva: [ok-falso]
EOF
cat > .openheinerss/harnesses/ok-falso.yaml <<'EOF'
name: ok-falso
command: sh
args: ["-c", "cat >/dev/null; echo '{\"type\":\"text\",\"text\":\"OK\"}'; echo '{\"type\":\"end\"}'"]
EOF
cat > .openheinerss/harnesses/sem-cota-solo.yaml <<'EOF'
name: sem-cota-solo
command: sh
args: ["-c", "cat >/dev/null; echo 'You have hit your session limit · resets 9am'"]
quotaRegex: "session limit"
EOF
echo "tarefa" > .claude/agentes/prompts/reserva.md && cp .claude/agentes/prompts/reserva.md .claude/agentes/prompts/solo.md
"$OH" rodar reserva sem-cota --tentativas 3; echo "exit=$?"
grep -E '^### tentativa|^FIM' .claude/agentes/logs/reserva.log
"$OH" rodar solo sem-cota-solo; echo "exit=$?"
tail -3 .claude/agentes/logs/solo.log
```

Esperado:
- `reserva`: `exit=0`; o log mostra `### tentativa 1 ... motor sem-cota`, depois `### tentativa 2 ... motor ok-falso` e `FIM ... código 0`; o `meta.json` fica com `"motor":"ok-falso"` e `"tentativa":2`. Foi a troca automática de instância por falta de cota.
- `solo` (sem `reserva`): para na primeira tentativa, `exit=2`; o log termina com `Falta de cota: resets 9am` e `FIM ... código 2`.

## 8. Limites de carga, de agentes e de cota (10 s)

```bash
echo t > .claude/agentes/prompts/carga.md
timeout 5 "$OH" rodar carga mock --carga-maxima 0.0001 --tentativas 1; echo "exit=$?"; ls .claude/agentes/logs | grep -c '^carga'
"$OH" limites | head -5
"$OH" rodar carga mock --quando-carga-abaixo 1000 --tentativas 1 | head -1
```

Esperado: o primeiro comando fica esperando a carga cair e o `timeout` o encerra (`exit=124`; nenhum arquivo `carga*` criado, pois ele nem começou); `limites` imprime `Limites em <data>` e as instâncias Codex/Claude (percentual e horário de reinício, se houver leitura; `--json` dá o mesmo em JSON); o último termina com `FIM carga código 0` (carga abaixo de 1000). `--cota-max N` pula uma instância cujo uso de cota já passou de N%: se você tem o Codex instalado, `"$OH" rodar carga codex --cota-max 1 --tentativas 1` sai com `exit=2` e o log diz `pulando codex: cota de codex em X% (limite 1.0%)`, sem chamar o motor. `--max-agentes` é conferido no passo 9.

## 9. `agentes`: listar, ver e parar (15 s)

```bash
cat > .openheinerss/harnesses/lento.yaml <<'EOF'
name: lento
command: sh
args: ["-c", "cat >/dev/null; echo '{\"type\":\"text\",\"text\":\"trabalhando\"}'; sleep 120; echo '{\"type\":\"end\"}'"]
EOF
echo t > .claude/agentes/prompts/lento.md
"$OH" rodar lento lento --tentativas 1 > lento.out 2>&1 &
sleep 4
"$OH" agentes
"$OH" agentes ver lento | tail -2
# um segundo agente só começa quando houver vaga:
"$OH" rodar outro mock --texto oi --max-agentes 1 --tentativas 1 > outro.out 2>&1 &
sleep 3; cat outro.out            # vazio: ele está esperando a vaga
"$OH" agentes parar lento; sleep 2
wait %1; echo "lento saiu com $?"; cat lento.out
pgrep -fc 'sleep 120'
wait; cat outro.out
```

Esperado:
- `agentes` lista `lento ... rodando ... última=trabalhando` (e os agentes dos passos anteriores como `terminou código N`); `agentes --json` dá o mesmo em JSON;
- `agentes ver lento` mostra o fim do log;
- `outro.out` fica vazio enquanto `lento` ocupa a única vaga; depois do `parar`, `outro` roda e imprime `FIM outro código 0`;
- `agentes parar lento` imprime `Agente lento parado (código 130)`; `lento saiu com 130` e `lento.out` mostra `FIM lento código 130`; o log tem uma só linha `FIM ... código 130`; o processo `sleep 120` do motor **não sobra** (`pgrep` imprime `0`).

## 10. Servidor WebSocket com eventos `orq.*` (10 s)

```bash
cp .claude/agentes/prompts/reserva.md .claude/agentes/prompts/ws1.md
cp .claude/agentes/prompts/reserva.md .claude/agentes/prompts/ws2.md
"$OH" serve --port 4891 & SERVE=$!
sleep 1
node "$REPO/scripts/ws-cliente.mjs" ws1 sem-cota --cwd "$DEMO" --url ws://127.0.0.1:4891/ws; echo "exit=$?"
node "$REPO/scripts/ws-cliente.mjs" ws2 mock --cwd "$DEMO" --url ws://127.0.0.1:4891/ws; echo "exit=$?"
```

Esperado no `ws1` (reserva por cota, pelo servidor): `RESPOSTA 1 {... "assinado":true ...}`, `RESPOSTA 2 {"id":"rodar-1","agente":"ws1",...}` e a sequência `orq.inicio` (motor `sem-cota`, tentativa 1) → `orq.erro` (`"cota":true`) → `orq.inicio` (motor `ok-falso`, tentativa 2) → `orq.progresso` (`"resumo":"OK"`) → `orq.fim` (`"codigo":0,"tentativas":2`), e `exit=0`.
Esperado no `ws2` (mock): `orq.inicio` → `orq.progresso` → `orq.precisa_decisao` (pergunta "Permitir a ferramenta Bash: git status ...") → o cliente responde `RESPOSTA 3 {"id":"dec-N","resposta":"permitir"}` → `orq.fim` com `codigo` 0. Use `--decidir negar` para ver a negativa virar falha.

Proteção contra páginas web de terceiros (o servidor executa agentes na sua máquina):

```bash
H='-H Connection:Upgrade -H Upgrade:websocket -H Sec-WebSocket-Version:13 -H Sec-WebSocket-Key:x3JJHMbDL1EzLkh9GBhXDw=='
curl -s -o /dev/null -w "%{http_code}\n" --max-time 3 $H -H "Origin: https://site-mal.example" http://127.0.0.1:4891/ws
curl -s -o /dev/null -w "%{http_code}\n" --max-time 3 $H -H "Origin: http://localhost:3000" http://127.0.0.1:4891/ws
kill -TERM "$SERVE"; wait "$SERVE"; echo "serve saiu com $?"
```

Esperado: `403`, depois `101`, e `serve saiu com 130` em menos de 1 s (o servidor sai com SIGTERM, ou com Ctrl-C quando está em primeiro plano; um shell não interativo entrega `&` com SIGINT ignorado, por isso o roteiro usa `kill -TERM`). Para liberar outra origem: `OPENHEINERSS_ORIGENS=https://painel.exemplo "$OH" serve ...`.

STDIO (mesmo protocolo, sem rede):

```bash
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"harness.listar"}' | "$OH" serve --stdio | head -c 300
```

Esperado: uma linha JSON-RPC com `"result":{"harnesses":[...]}` contendo `mock` e as instâncias. O processo termina quando a entrada fecha.

## 11. SDKs TypeScript, Python e PHP (30–90 s, a maior parte é `npm ci`)

Cada teste sobe `serve --stdio`, registra um harness, assina eventos, consulta limites, lança um `rodar` com o mock e o para. Rode a partir de `$DEMO` (o `rodar` usa a pasta atual como repositório):

```bash
export OPENHEINERSS_BIN="$OH" PYTHONDONTWRITEBYTECODE=1
(cd "$REPO/sdk/typescript" && npm ci --no-audit --no-fund && npm run build)
node --test "$REPO/sdk/typescript/test/integration.test.mjs" 2>&1 | grep -E '^# (pass|fail)'
PYTHONPATH="$REPO/sdk/python" python3 -m unittest discover -s "$REPO/sdk/python" -p test_integration.py 2>&1 | tail -3
php "$REPO/sdk/php/test_integration.php"
```

Esperado: `# pass 1` e `# fail 0`; `Ran 1 test` / `OK`; `PHP SDK integração OK`. Os três terminam sozinhos (o `close()` do TypeScript encerra o servidor).

## 12. Testes e documentação do repositório (3–6 min)

```bash
cd "$REPO"
go build ./... && go vet ./... && go test -count=1 ./...
flock /tmp/crom-pesado.lock go test -race -count=1 ./...
go test -cover ./... | grep -v 'no test files'
go run ./cmd/openheinerss docs --check; git status --short docs/09-cli.md
```

Esperado: tudo `ok`; nenhuma corrida (`-race`); a cobertura por pacote é impressa; o manual `docs/09-cli.md` não muda (`git status` sem linha para ele). Os números da última rodada estão em `RELATORIO-AGENTE.md` da etapa 7.

## 13. Limpeza

```bash
cd "$REPO" && git worktree prune; rm -rf "$DEMO"
```

Esperado: nenhuma saída. (As worktrees do `$DEMO` somem junto com ele; o repositório do clone não é tocado pelos passos acima.)

## 14. Verificações adicionais da Central

### Nomes iguais em paralelo

Inicie uma execução lenta e, antes de ela terminar, tente lançar o mesmo nome em outro terminal:

```sh
openheinerss rodar duplicado mock --texto "tarefa lenta" &
openheinerss rodar duplicado mock --texto "segunda tentativa"
```

Esperado: a segunda execução falha com `já está rodando` e código diferente de zero. `openheinerss agentes parar duplicado` deve parar o processo vivo registrado no `meta.json`.

### Falha no meio

Use um harness falso que morra sem emitir `end`/`complete` durante o prompt.

Esperado: a execução registra `FIM ... código` diferente de zero, e `agentes listar` não deixa o agente como `rodando`.

### Cota real do Claude Code

Use uma amostra NDJSON com `result` contendo `is_error: true` e `result: "You've hit your session limit · resets 9am"` (ou `You have hit your session limit`).

Esperado: o evento vira falha de processo/cota, o log não registra sucesso código 0 e uma instância em `reserva` é tentada quando configurada.

## 15. Opcional: motores reais (gasta uma frase de cota por teste)

Só com contas de teste, nunca a conta principal do Claude, e com prompts curtíssimos. As instâncias vivem em `.openheinerss/harnesses/` do projeto que as usa (aqui, o próprio clone):

```bash
cd "$REPO" && "$OH" harness test --todos --pular aider --timeout 150s
```

Esperado: tabela `nome | base | modo | resultado | tempo | tokens | motivo` com `OK` em cada linha (motor sem login ou sem cota aparece como `sem login`/`sem cota`, com o motivo, nunca como travamento). O código de saída é `1` se alguma linha não for `OK`. O último resultado medido está em [TESTES-REAIS.md](TESTES-REAIS.md). Para um `rodar` real curto, use um repositório temporário e `--texto "Crie REAL-OK.txt com a palavra OK e faça commit"` com a instância desejada; `scripts/teste-real.sh` faz isso para Codex e OpenCode.
