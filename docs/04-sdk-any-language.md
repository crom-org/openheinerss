# SDKs e protocolo bruto

O transporte comum é JSON-RPC 2.0 em NDJSON via `openheinerss serve --stdio`. O servidor WebSocket (`ws://127.0.0.1:4820/ws`; `OPENHEINERSS_PORTA` muda a porta) também existe; só o SDK TypeScript o usa. Os SDKs Python e PHP usam STDIO.

Os três SDKs (versão 0.2.0) expõem a mesma orquestração: `registerHarness`, `listHarnesses`, `run` (`rodar.iniciar`), `listRuns`, `stopRun`, `decideRun`, `getLimits` e `subscribeEvents`. A assinatura aceita `projeto`, `agente`, `cwd` e `pasta`; os callbacks recebem `orq.inicio`, `orq.progresso`, `orq.fim`, `orq.erro` e `orq.precisa_decisao`. Em `run`, `texto` é o prompt em texto e `prompt` é o caminho de um arquivo de prompt.

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

Não há módulo `openheinerss.aio` nem transporte WebSocket neste SDK.

## PHP

`sdk/php/` oferece `Agent::session()`, `prompt()` (devolve uma string e aceita callback opcional de eventos) e `respondPermission()`, além dos métodos de orquestração. Não existe método `stream()` separado nem transporte WebSocket.

## RPC mínimo em qualquer linguagem

```json
{"jsonrpc":"2.0","id":1,"method":"session.create","params":{"harness":"mock","mode":"mock","cwd":"."}}
{"jsonrpc":"2.0","id":2,"method":"session.prompt","params":{"sessionId":"sess_id","text":"responda OK"}}
```

Os nomes oficiais e campos estão em [02-protocol-spec.md](02-protocol-spec.md). Para um cliente WebSocket completo e curto veja `scripts/ws-cliente.mjs`; o passo a passo de verificação está em [VERIFICACAO.md](VERIFICACAO.md).
