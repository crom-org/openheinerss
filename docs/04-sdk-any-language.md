# SDKs e protocolo bruto

O transporte comum é JSON-RPC 2.0 em NDJSON via `openheinerss serve --stdio`. O servidor WebSocket também existe para clientes compatíveis, mas os SDKs Python e PHP atuais usam STDIO.

## TypeScript

O pacote em `sdk/typescript/` exporta `Openheinerss` e o hook React pelo próprio pacote `@openheinerss/sdk` (não há pacote separado `@openheinerss/react`). A configuração usa `options`, `wsEndpoint` e `transport`.

```ts
import { Openheinerss } from "@openheinerss/sdk";

const agent = new Openheinerss({ options: { harness: "mock" } });
agent.on("text", delta => process.stdout.write(delta));
await agent.prompt("responda OK");
```

O SDK também expõe `registerHarness` para o método `harness.register`. Consulte o README do pacote para a assinatura exata.

## Python

`sdk/python/` oferece `Agent.stream()` e `Agent.prompt()`. Os eventos do stream são dicionários com chaves `type` e `data`; não são objetos com atributo `event.type`.

```python
from openheinerss import Agent

agent = Agent(harness="mock")
for event in agent.stream("responda OK"):
    if event["type"] == "agent.text":
        print(event["data"]["delta"], end="")
```

Não há módulo `openheinerss.aio` nem transporte WebSocket implementado neste SDK.

## PHP

`sdk/php/` oferece `Agent::session()`, `prompt()` e `respondPermission()`. `prompt()` devolve uma string e aceita callback opcional de eventos; não existe método `stream()` separado, nem `registerHarness` ou transporte WebSocket.

## RPC mínimo em qualquer linguagem

```json
{"jsonrpc":"2.0","id":1,"method":"session.create","params":{"harness":"mock","mode":"mock","cwd":"."}}
{"jsonrpc":"2.0","id":2,"method":"session.prompt","params":{"sessionId":"sess_id","text":"responda OK"}}
```

Os nomes oficiais e campos estão em [02-protocol-spec.md](02-protocol-spec.md). Os métodos de sessão disponíveis são `session.create`, `session.prompt`, `session.permission_respond`, `session.abort` e `session.list`; `session.resume` não existe.
