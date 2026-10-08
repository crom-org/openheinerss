# SDKs e protocolo bruto

O transporte comum é JSON-RPC 2.0 em NDJSON via `openheinerss serve --stdio`. O servidor WebSocket (`ws://127.0.0.1:4820/ws`; `OPENHEINERSS_PORTA` muda a porta) também existe; os três SDKs o usam. Python e PHP usam STDIO por padrão e WebSocket com `Agent(transport="websocket", host=, port=, origin=)` / `url=` (PHP: `Agent::session(['transport' => 'websocket', 'port' => N])`), sem dependências; origem fora de `OPENHEINERSS_ORIGENS` é recusada (HTTP 403 → `TransportError`/`RuntimeException`).

Os três SDKs (versão 1.1.0) expõem a mesma orquestração: `registerHarness`, `listHarnesses`, `run` (`rodar.iniciar`), `listRuns`, `stopRun`, `decideRun`, `getLimits` e `subscribeEvents`. Todos os callbacks `orq.*` recebem `geracao` (e todo evento do serve, inclusive `agent.*`, traz `geracao` no envelope); erro RPC em `session.prompt` (ex.: `/compact` sem equivalente no codex) vira exceção nos três SDKs, e Python/PHP têm teto de segurança sem eventos no prompt (`prompt_timeout`/`$promptTimeout`, padrão 3600 s; `timeout` por chamada); a geração da última resposta também fica disponível no cliente. Comandos nativos do harness (`harness.comandos*`): `listCommands(harness, cwd?)`,
`annotateCommand(harness, comando, anotacao)` e `confirmCommand(harness, comando)` (Python também
`list_commands`, `annotate_command`, `confirm_command`). Cada comando traz `nome`, `descricao`,
`repasse` (`literal`/`traduzido`/`sem_equivalente`), `origem`, `anotacao` e `confirmado`. A tela de
confirmação do primeiro uso fica no cliente; o SDK só lê e grava o estado.

`decideRun(id, resposta, mensagem?, run?)`: `run` (id da execução, vem em `orq.precisa_decisao`) é opcional e, se enviado, o servidor recusa a resposta quando ele não bate com o da decisão. Em `run`, `texto` é o prompt em texto e `prompt` é o caminho de um arquivo de prompt.

## TypeScript

O pacote em `sdk/typescript/` exporta `Openheinerss` e o hook React pelo próprio pacote `@openheinerss/sdk` (não há pacote separado `@openheinerss/react`). A configuração usa `options`, `wsEndpoint` e `transport`.

```ts
import { Openheinerss } from "@openheinerss/sdk";

const agent = new Openheinerss({ options: { harness: "mock" } });
agent.on("text", delta => process.stdout.write(delta));
await agent.prompt("responda OK");
```

## Python

`sdk/python/` oferece `Agent.stream()` e `Agent.prompt()`. Os eventos do stream são dicionários com chaves `type` e `data`; não são objetos com atributo `event.type`. Os métodos de orquestração têm nomes em snake_case (`list_runs`) e em camelCase (`listRuns`).

```python
from openheinerss import Agent

agent = Agent(harness="mock")
for event in agent.stream("responda OK"):
    if event["type"] == "agent.text":
        print(event["data"]["delta"], end="")
```

O transporte mantém um leitor em thread: depois de `subscribe_events()` os callbacks de `orq.*` são chamados continuamente, inclusive enquanto o programa faz outra coisa, sem precisar chamar `list_runs()` para liberar eventos. `close()` encerra somente o processo filho criado pelo SDK.

Os callbacks Python são executados por uma fila e uma thread de despacho separadas do leitor. Portanto, é seguro chamar `decide_run()` dentro do callback; o leitor continua recebendo a resposta RPC. Se o servidor fechar o STDIO, todas as requisições pendentes terminam com um erro `TransportError` indicando EOF/conexão encerrada.

## PHP

`sdk/php/` oferece `Agent::session()`, `prompt()` (devolve uma string e aceita callback opcional de eventos), `respondPermission()` e `listen()`, além dos métodos de orquestração. Como PHP não tem uma thread portátil para esse caso, `listen($segundos)` mantém um loop de leitura com `stream_select`; use-o no processo que precisa observar eventos continuamente. Os métodos que aguardam respostas também bombeiam eventos durante a espera.

O transporte PHP inicia o processo com argumentos separados (sem shell), assim binários em caminhos com espaços funcionam. O transporte TypeScript usa `spawn` com argv pelo mesmo motivo e rejeita as RPCs pendentes quando o processo ou WebSocket chega ao EOF. No TypeScript, callbacks podem chamar métodos assíncronos como `decideRun()` sem bloquear o processamento da resposta.

## RPC mínimo em qualquer linguagem

```json
{"jsonrpc":"2.0","id":1,"method":"session.create","params":{"harness":"mock","mode":"mock","cwd":"."}}
{"jsonrpc":"2.0","id":2,"method":"session.prompt","params":{"sessionId":"sess_id","text":"responda OK"}}
```

Os nomes oficiais e campos estão em [02-protocol-spec.md](02-protocol-spec.md). Para um cliente WebSocket completo e curto veja `scripts/ws-cliente.mjs`; o passo a passo de verificação está em [VERIFICACAO.md](VERIFICACAO.md).

# SDKs 1.1.0

Os SDKs TypeScript, Python e PHP usam JSON-RPC sobre STDIO por padrão e conversam com
`openheinerss serve --stdio`. A porta WebSocket padrão é **4820**; defina
`OPENHEINERSS_PORTA` (ou `port`/`wsEndpoint` no TypeScript) quando usar o transporte WebSocket.

Todos expõem sessões, `registerHarness`/`harness.listar`, `rodar.iniciar`, `rodar.listar`,
`rodar.parar`, `rodar.decidir`, `limites.obter` e `eventos.assinar`. A assinatura aceita
`projeto`, `agente`, `cwd` e `pasta`; os callbacks recebem `orq.inicio`, `orq.progresso`,
`orq.fim`, `orq.erro` e `orq.precisa_decisao`.

`decideRun(id, resposta, mensagem, run, encerrar)` (Python: `decide_run(..., encerrar=True)`) aceita
`encerrar` opcional: numa negação, termina a execução com `orq.fim` código 3 e `motivo` `"negado"`,
sem nova tentativa. Omitido, vale o padrão do servidor (`serve --negar-encerra`).

Exemplo conceitual (os exemplos completos estão nos READMEs de cada SDK):

```text
registerHarness({name: "meu-harness", base: "mock"})
subscribeEvents({projeto: "demo"}, callback)
run({nome: "demo", motor: "mock", texto: "responda OK"})
```

O changelog 1.1.0 registra a correção da porta que era 4799 na versão 0.1.0 e a
referência antiga ao `codex run`; o adaptador atual usa `codex exec`.

Em TypeScript, a primeira chamada que precisa de transporte o inicializa automaticamente; `start()` continua disponível para criar uma sessão explicitamente. Assim, `subscribeEvents`, `run`, `listRuns` e `getLimits` podem ser chamados diretamente. O evento `agent.thinking` é entregue como `thinking` (e não como `error`).
