# 📡 Especificação do Protocolo (Openheinerss Protocol)

O protocolo de comunicação entre o cliente e o binário Openheinerss é baseado no padrão **JSON-RPC 2.0 / Linhas JSON (NDJSON)**. Ele funciona de forma bidirecional (full-duplex).

---

## 1. Comandos do Cliente -> Openheinerss (Requests)

### `session.create`
Inicia uma nova sessão de agente com o harness escolhido.
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session.create",
  "params": {
    "harness": "claude-code", // ou "opencode", "codex", "agy"
    "cwd": "/home/user/projeto",
    "provider": "openrouter",
    "model": "anthropic/claude-3.7-sonnet",
    "options": {
      "effort": "high",
      "harnessArgs": ["--add-dir", "../outro"],
      "permissionMode": "ask",
      "systemPrompt": "Você é um assistente sênior..."
    }
  }
}
```

`options.harnessArgs` (lista de strings) são argumentos nativos extras do harness: vão **intactos e na mesma ordem** ao processo, antes do prompt. Não há lista de permitidos; a ponte não filtra nada. O mesmo campo existe em `rodar.iniciar`/`run` (`harnessArgs`) e na CLI como `--harness-arg`/`--arg` (repetível, sem separar por vírgula).

### `session.prompt`
Envia uma instrução ou mensagem do usuário para a sessão ativa.

**Regra das `/`.** Texto que começa com `/` (ex.: `/compact`, `/model x`) vai **literalmente** ao harness quando ele aceita no modo sem tela. Onde não aceita, a ponte traduz para o equivalente (`/model X` muda o modelo das próximas chamadas, `/clear` ou `/new` esquece o id de retomada, `/effort X` muda o esforço, `/compact` usa o nativo se houver) e emite um `agent.text` avisando, seguido de `agent.complete`. Sem equivalente, a resposta é um **erro JSON-RPC** com mensagem clara ("o harness X não aceita /cmd no modo sem tela e não há equivalente na linha de comando; <dica>"); nada é engolido. A CLI não intercepta nenhuma `/`, então não há prefixo de escape.
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session.prompt",
  "params": {
    "sessionId": "sess_abc123",
    "text": "Refatore o arquivo main.go para separar as rotas",
    "images": [
      {
        "mediaType": "image/png",
        "data": "iVBORw0KGgoAAAANSUhEUgAAAA..."
      }
    ]
  }
}
```

### `session.permission_respond`
Responde a um pedido de autorização de execução de ferramenta.
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "session.permission_respond",
  "params": {
    "sessionId": "sess_abc123",
    "requestId": "perm_987",
    "decision": "allow" // ou "deny"
  }
}
```

### `session.abort`
Interrompe imediatamente o processamento atual do agente.
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "session.abort",
  "params": {
    "sessionId": "sess_abc123"
  }
}
```

### `session.resume`
Reabre uma sessão gravada em `.openheinerss/sessions/<id>.jsonl` (no `cwd` informado) e entrega ao motor o ID nativo da conversa, para continuar sem reprocessar. Devolve o mesmo resultado de `session.create`. Erros: `SESSION_NOT_FOUND` (sem transcript ou ID com `/`/`..`) e `INVALID_PARAMS` (transcript sem configuração).
```json
{"jsonrpc":"2.0","id":3,"method":"session.resume","params":{"sessionId":"sess_abc123","cwd":"/home/user/projeto"}}
```

### `harness.register`
Registra um harness ou instância em tempo de execução, sem gravar arquivo. Os campos são os de um arquivo de harness custom (`name`, `base`, `command`, `args`, `env`, `model`, `prompt`, `finishRegex`, `quotaRegex`, `reserva`; veja [06-harness-custom.md](06-harness-custom.md)). Resposta: `{"name":"...","status":"registered"}`.
```json
{"jsonrpc":"2.0","id":4,"method":"harness.register","params":{"name":"meu-claude","base":"claude-code","env":{"CLAUDE_CONFIG_DIR":"~/.claude-conta2"}}}
```

---

## 2. Eventos do Openheinerss -> Cliente (Notifications / Streams)

Todos os eventos gerados pelos diferentes harnesses são normalizados para estes tipos:

### `agent.thinking`
Streaming do raciocínio interno do modelo (Extended Thinking / CoT).
```json
{
  "jsonrpc": "2.0",
  "method": "agent.thinking",
  "params": {
    "sessionId": "sess_abc123",
    "delta": "Analisando a estrutura do arquivo main.go..."
  }
}
```

### `agent.text`
Streaming da resposta em texto para o usuário.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.text",
  "params": {
    "sessionId": "sess_abc123",
    "delta": "Vou criar o arquivo `routes.go` para modularizar as rotas."
  }
}
```

### `agent.tool_call`
Notificação de chamada de ferramenta em andamento.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.tool_call",
  "params": {
    "sessionId": "sess_abc123",
    "callId": "call_123",
    "tool": "FileEdit",
    "input": {
      "path": "main.go",
      "action": "replace"
    }
  }
}
```

### `agent.tool_result`
Resultado retornado após a execução da ferramenta.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.tool_result",
  "params": {
    "sessionId": "sess_abc123",
    "callId": "call_123",
    "status": "success",
    "output": "1 arquivo alterado: +25 -10 linhas."
  }
}
```

### `agent.permission_request`
Quando uma ferramenta requer aprovação do usuário.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.permission_request",
  "params": {
    "sessionId": "sess_abc123",
    "requestId": "perm_987",
    "tool": "Bash",
    "command": "rm -rf build/",
    "risk": "high"
  }
}
```

### `agent.raw`
Linha original do harness que não virou outro evento (stdout sem mapeamento) e as linhas de stderr, exatamente como foram escritas. Chega no WebSocket e no stdio como as demais notificações; o `rodar` também a grava no log (`[raw stdout] ...`).
```json
{
  "jsonrpc": "2.0",
  "method": "agent.raw",
  "params": {
    "sessionId": "sess_abc123",
    "harness": "codex",
    "stream": "stderr",
    "line": "texto exatamente como veio"
  }
}
```

---

## 3. Comandos de Sistema e Catálogo

### `catalog.list`
Lista os harnesses disponíveis, modos suportados (`sdk`, `cli`) e modelos compatíveis.
```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "catalog.list"
}
```

### `doctor.check`
Executa o diagnóstico de dependências do ambiente.
```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "doctor.check",
  "params": {
    "harness": "claude-code" // opcional
  }
}
```

---

## 4. Padrão de Erros JSON-RPC Enriquecidos

Quando ocorre um erro de execução ou dependência ausente, a resposta de erro segue o padrão:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": 4010,
    "message": "Pré-requisito ausente: Node.js 18+ não encontrado no PATH",
    "data": {
      "harness": "claude-code",
      "mode": "sdk",
      "missing": ["node"],
      "suggestedFix": "Instale o Node.js v20+ ou execute no modo 'cli' com o binário claude instalado."
    }
  }
}
```

### Códigos de Erro Padronizados
| Código | Nome | Descrição |
| :--- | :--- | :--- |
| `4001` | `SESSION_NOT_FOUND` | ID de sessão inexistente ou expirado |
| `4002` | `HARNESS_NOT_FOUND` | Harness solicitado não existe ou não está registrado |
| `4010` | `HARNESS_DEPENDENCY_MISSING` | Falta um binário ou runtime no SO (Node, Docker, CLI) |
| `4011` | `AUTH_TOKEN_MISSING` | Chave de API ou token de autenticação não configurado |
| `4020` | `PERMISSION_REJECTED` | A ferramenta foi negada pelo usuário ou política de segurança |
| `4030` | `PROCESS_CRASHED` | O subprocesso do agente finalizou inesperadamente |


---

## 5. Orquestração (`rodar`, limites e eventos `orq.*`)

Cada processo `serve` cria uma `geracao` aleatória no início. Ela aparece no envelope de todas as respostas JSON-RPC, no envelope de **todos** os eventos do serve (`agent.*` e `orq.*`, campo `geracao` ao lado de `method`) e também nos parâmetros dos eventos `orq.*`; clientes devem usá-la junto aos IDs, pois `rodar-1` e `dec-2` podem ser reciclados depois de um reinício.

Os mesmos métodos valem no STDIO (NDJSON) e no WebSocket (`ws://127.0.0.1:4820/ws`). **Segurança do WebSocket:** o servidor executa agentes na máquina, então só aceita conexões sem cabeçalho `Origin` (SDKs e scripts) ou de origens locais (`localhost`, `127.0.0.1`, `[::1]`, Tauri). Para liberar a página de um painel remoto, defina `OPENHEINERSS_ORIGENS` com as origens separadas por vírgula (ou `*` para liberar todas, por sua conta e risco); qualquer outra recebe `403`. O `crom-central` (ou qualquer cliente) lança agentes pelo servidor e acompanha tudo por eventos. Os eventos `agent.*` continuam como na seção 2; os `orq.*` descrevem a **missão** (um agente do `rodar`), não a conversa.

### `eventos.assinar`
Registra a conexão para receber os eventos `orq.*`. Sem assinar, nenhum `orq.*` chega. Uma nova chamada na mesma conexão troca o filtro. Todos os campos são opcionais.

```json
{"jsonrpc":"2.0","id":1,"method":"eventos.assinar","params":{"projeto":"crom-tv","agente":"etapa-1","cwd":"/home/j/projetos/crom-tv"}}
```
```json
{"jsonrpc":"2.0","id":1,"result":{"assinado":true,"projeto":"crom-tv","agente":"etapa-1","observando":"/home/j/projetos/crom-tv/.claude/agentes"}}
```

- `projeto` e `agente` filtram os eventos (vazio = todos). O nome do projeto é o nome da pasta da raiz git (ou o campo `projeto` de `rodar.iniciar`).
- `cwd` / `pasta` dizem qual pasta de agentes observar (padrão: raiz git do servidor + `.claude/agentes`, ou `$AGENTES`). Veja "Agentes lançados pelo CLI" abaixo.

### `rodar.iniciar`
Mesmas opções do `openheinerss rodar` (nomes em português, como em `run`), mais `projeto`. O prompt vem de `texto` (texto direto), de `prompt` (caminho de um arquivo) ou, na falta dos dois, de `<pasta>/prompts/<nome>.md`; o nome do agente só aceita letras, números, `.`, `_` e `-`. Não bloqueia: devolve o `id` e os eventos informam o andamento.

```json
{"jsonrpc":"2.0","id":2,"method":"rodar.iniciar","params":{"nome":"etapa-1","motor":"codex2","modelo":"","esforco":"high","prompt":"","retomar":false,"pasta":"","branchBase":"main","cargaMax":0,"maxAgentes":4,"tentativas":4,"cotaMax":0,"harnessArgs":["--x","a,b"],"cwd":"/home/j/projetos/crom-tv","projeto":"crom-tv"}}
```
```json
{"jsonrpc":"2.0","id":2,"geracao":"geracao-a1b2c3","result":{"geracao":"geracao-a1b2c3","id":"rodar-1","agente":"etapa-1","projeto":"crom-tv"}}
```
Se o servidor foi iniciado com `serve --max-agentes N`, `maxAgentes` vira no máximo `N` (0 ou ausente também vira `N`).

Erros (`-32602`): nome ou motor vazio; agente com o mesmo nome já rodando neste servidor.

### `rodar.listar`
Lê `logs/*.meta.json` da pasta de agentes (`cwd`, `pasta` e `projeto` opcionais) e junta as execuções do servidor e as decisões pendentes. `estado`: `aguardando` (esperando vaga/worktree), `rodando`, `concluido`, `falhou` ou `interrompido` (processo morreu sem registrar o fim).

```json
{"jsonrpc":"2.0","id":3,"method":"rodar.listar","params":{"cwd":"/home/j/projetos/crom-tv"}}
```
```json
{"jsonrpc":"2.0","id":3,"result":{"agentes":[{"id":"rodar-1","agente":"etapa-1","projeto":"crom-tv","estado":"rodando","motor":"codex2","modelo":"padrão","tentativa":1,"inicio":"2026-10-08T07:30:00-03:00","pid":4242,"log":"/home/j/projetos/crom-tv/.claude/agentes/logs/etapa-1.log"}],"decisoes":[{"id":"dec-2","agente":"etapa-1","projeto":"crom-tv","pergunta":"Permitir a ferramenta Bash: git status (risco medium)?","opcoes":["permitir","negar"]}]}}
```

### `rodar.parar`
Para uma execução **lançada por este servidor**, por `id` ou `agente`. O fim chega como `orq.fim` com `codigo` 130 (o meta e o log também recebem o FIM).

```json
{"jsonrpc":"2.0","id":4,"method":"rodar.parar","params":{"id":"rodar-1"}}
```
```json
{"jsonrpc":"2.0","id":4,"result":{"parando":true}}
```

### `rodar.decidir`
Responde um `orq.precisa_decisao`. `resposta`: `"permitir"` ou `"negar"` (aceita também `sim`/`não`, `allow`/`deny`); `mensagem` é opcional. O agente fica parado até a resposta (ou até `rodar.parar`). `run` (id da execução) e `geracao` são opcionais; se enviados e não baterem com os da decisão/servidor, a resposta é recusada (`-32602`) e a decisão continua pendente. `rodar.listar` mostra as decisões ainda pendentes para quem conectar depois. Rodando pelo CLI (sem servidor) as permissões são aprovadas automaticamente, como antes.

**Negar = encerrar.** `encerrar` (bool, opcional) numa negação termina a execução na hora: o agente é parado, o log recebe `FIM HH:MM código 3`, o `meta.json` fica com `codigo` 3 e `motivo` `"negado"` e sai `orq.fim` com `codigo` 3 e `motivo` `"negado"`, sem nova tentativa nem reserva. Sem o campo vale o padrão do servidor: `serve --negar-encerra` (alias `--deny-ends`) liga; sem a flag, a negação continua como falha comum (`orq.erro` e nova tentativa, se houver). `encerrar` vence a flag naquela decisão (`false` desliga com a flag ligada); com `permitir` é ignorado.

```json
{"jsonrpc":"2.0","id":7,"method":"rodar.decidir","params":{"run":"rodar-1","id":"dec-3","resposta":"negar","encerrar":true}}
```

```json
{"jsonrpc":"2.0","id":5,"method":"rodar.decidir","params":{"geracao":"geracao-a1b2c3","run":"rodar-1","id":"dec-2","resposta":"permitir"}}
```
```json
{"jsonrpc":"2.0","id":5,"geracao":"geracao-a1b2c3","result":{"id":"dec-2","resposta":"permitir"}}
```

### `harness.comandos`, `harness.comandos.anotar`, `harness.comandos.confirmar`
Comandos nativos (`/compact`, `/model`…) de um harness ou instância. Não é preciso escolher harness para a Central: cada um tem a sua lista. A instância herda o catálogo da sua `base:` (e as anotações de cada nível da herança; a do nível mais específico vence).

- `harness.comandos` `{harness, cwd?}` → `{harness, base, cadeia, desconhecido, arquivo, comandos:[{nome, descricao, repasse, detalhe?, origem, anotacao?, confirmado}]}`.
  - `repasse`: `literal` (vai como está), `traduzido` (a ponte troca por flag/opção) ou `sem_equivalente` (só existe na tela; mandar dá erro claro). `desconhecido` é o repasse de um `/x` fora da lista.
  - `origem`: `embutido` (catálogo da auditoria docs/PONTE.md), `descoberto` (achado nos arquivos do harness: `.claude/commands`, `skills/*/SKILL.md` em `cwd` e em `CLAUDE_CONFIG_DIR`; `command(s)/` do opencode; `prompts/` do codex) ou `usuario` (só no `comandos.yaml`).
  - `cwd` (opcional): pasta do projeto onde procurar comandos do harness.
- `harness.comandos.anotar` `{harness, comando, anotacao}` e `harness.comandos.confirmar` `{harness, comando}` gravam no `comandos.yaml` e devolvem o comando já mesclado. A tela de confirmação do primeiro uso é do cliente; o openheinerss só guarda `confirmado`.
- Arquivo: `~/.config/openheinerss/comandos.yaml` (respeita `XDG_CONFIG_HOME`); com `--config`/`OPENHEINERSS_CONFIG`, também `<pasta>/comandos.yaml`, que vence o global e é onde se grava. A gravação preserva comentários, usa trava entre processos (`comandos.yaml.lock`: flock no Linux/macOS, arquivo exclusivo no Windows) cobrindo ler+alterar+gravar, temporário único na mesma pasta e devolve erro se não gravar — dois `serve` na mesma `--config` não perdem anotações. O arquivo fica com permissão 0600 (pasta criada com 0700). Segredos em `anotacao` (`NOME=valor`/`NOME: valor` com nome de credencial — token, secret, senha, password, api_key, authorization…, `Bearer …` e chaves conhecidas `sk-…`, `ghp_…`, `github_pat_…`, `glpat-…`, `xox?-…`, `AKIA…`) viram `***` antes de gravar e ao listar.

```yaml
claude-code:
  "/compact":
    anotacao: "serve para compactar o Claude Code; o central não usa (memória é organizada de outro jeito)"
    confirmado: true
```
```json
{"jsonrpc":"2.0","id":7,"method":"harness.comandos","params":{"harness":"conta2"}}
```
```json
{"jsonrpc":"2.0","id":7,"result":{"harness":"conta2","base":"claude-code","cadeia":["claude-code","conta2"],"desconhecido":"literal","arquivo":"/home/u/.config/openheinerss/comandos.yaml","comandos":[{"nome":"/compact","descricao":"Compacta a conversa do Claude Code (resume o histórico para liberar contexto)","repasse":"literal","detalhe":"vai literal ao claude -p; se o claude não aceitar o comando sem tela, ele mesmo responde","origem":"embutido","anotacao":"serve para compactar o Claude Code; o central não usa (memória é organizada de outro jeito)","confirmado":true}]}}
```

### `limites.obter` e `harness.listar`
`limites.obter` devolve o mesmo JSON do `openheinerss limites --json` (o método antigo `limites` continua valendo). `harness.listar` devolve os harnesses e instâncias carregados, no formato de `catalog.list`.

```json
{"jsonrpc":"2.0","id":6,"method":"limites.obter"}
```
```json
{"jsonrpc":"2.0","id":6,"result":{"agora":"2026-10-08T07:30:00-03:00","instancias":[{"nome":"codex","base":"codex","janelas":[{"nome":"5h","percentual":12.5,"reiniciaEm":"2026-10-08T11:00:00-03:00"}]}]}}
```

### Eventos `orq.*`
Notificações sem `id`. Os eventos de execuções lançadas por `rodar.iniciar` trazem `id`; os de agentes vindos do CLI não.

| Evento | Quando | Campos |
| :--- | :--- | :--- |
| `orq.inicio` | começa cada tentativa | `geracao`, `agente`, `projeto`, `motor`, `modelo`, `tentativa`, `worktree` |
| `orq.progresso` | texto ou ferramenta nova; **no máximo 1 a cada 2 s por agente** (o excedente sai no fim da janela, só o mais recente) | `geracao`, `agente`, `projeto`, `resumo` (até ~160 caracteres) |
| `orq.precisa_decisao` | `agent.permission_request` do agente | `geracao`, `run`, `id`, `agente`, `projeto`, `pergunta`, `opcoes` |
| `orq.erro` | falha de uma tentativa, cota ou erro antes de começar | `geracao`, `agente`, `projeto`, `mensagem`, `cota` (bool) |
| `orq.fim` | missão terminou (também após erro ou parada) | `geracao`, `agente`, `projeto`, `codigo`, `tentativas`, `duracao` (segundos), `relatorio` (caminho do `RELATORIO-AGENTE.md`, se existir), `motivo` (`"negado"` quando uma negação encerrou; ausente nos demais) |

Ordem garantida por agente: `orq.inicio` → (`orq.progresso` | `orq.precisa_decisao` | `orq.erro`)* → `orq.fim`; nenhum progresso depois do fim. Com reservas ou novas tentativas há um `orq.inicio` por tentativa e um só `orq.fim`.

```json
{"jsonrpc":"2.0","method":"orq.inicio","params":{"id":"rodar-1","agente":"etapa-1","projeto":"crom-tv","motor":"codex2","modelo":"padrão","tentativa":1,"worktree":"/home/j/projetos/crom-tv/.claude/agentes/etapa-1"}}
{"jsonrpc":"2.0","method":"orq.progresso","params":{"id":"rodar-1","agente":"etapa-1","projeto":"crom-tv","resumo":"ferramenta Bash {\"command\":\"go test ./...\"}"}}
{"jsonrpc":"2.0","method":"orq.precisa_decisao","params":{"id":"dec-2","agente":"etapa-1","projeto":"crom-tv","pergunta":"Permitir a ferramenta Bash: git status (risco medium)?","opcoes":["permitir","negar"]}}
{"jsonrpc":"2.0","method":"orq.erro","params":{"id":"rodar-1","agente":"etapa-1","projeto":"crom-tv","mensagem":"execução interrompida por falta de cota","cota":true}}
{"jsonrpc":"2.0","method":"orq.fim","params":{"id":"rodar-1","agente":"etapa-1","projeto":"crom-tv","codigo":0,"tentativas":2,"duracao":812.4,"relatorio":"/home/j/projetos/crom-tv/.claude/agentes/etapa-1/RELATORIO-AGENTE.md"}}
```

### Agentes lançados pelo CLI
Um `openheinerss rodar` iniciado fora do servidor também aparece. Depois de `eventos.assinar`, o servidor lê `logs/*.meta.json` e `*.log` da pasta de agentes a cada 1 s (leitura periódica, sem dependência extra) e emite `orq.inicio`, `orq.progresso` (última linha nova do log) e `orq.fim` (do `codigo` do meta). Regras:
- na primeira leitura, só agentes que **estão rodando** (PID vivo) geram `orq.inicio`; execuções já terminadas são histórico;
- se o PID morrer sem registrar o fim, saem `orq.erro` e `orq.fim` com `codigo` 1;
- agentes lançados pelo próprio servidor não são lidos duas vezes.
