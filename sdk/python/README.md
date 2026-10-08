# Openheinerss Python SDK 1.1.0

SDK oficial em **Python** para integração com o maestro **Openheinerss** (crom-org).

## Instalação

```bash
pip install openheinerss
```

## Exemplo de Uso

```python
from openheinerss import Agent

# Inicia sessão com o harness desejado
agent = Agent(harness="mock")  # OPENHEINERSS_PORTA é respeitada pelo servidor; padrão: 4820
agent.registerHarness({"name": "meu-harness", "base": "mock"})
agent.subscribeEvents(lambda evento: print("orq:", evento))
execucao = agent.run({"nome": "teste", "motor": "mock", "texto": "responda OK", "cwd": "."})
print(agent.listHarnesses(), agent.getLimits(), execucao)

# Streaming de pensamentos e texto
for event in agent.stream("Escreva uma função que calcula fibonacci"):
    if event["type"] == "agent.thinking":
        print(f"🤔 Pensando: {event['data']['delta']}")
    elif event["type"] == "agent.text":
        print(event["data"]["delta"], end="", flush=True)

# Ou resposta direta:
resultado = agent.prompt("Refatore a função para usar memoização")
print(resultado)
```

## Repasse ao harness (ponte)
`harness_args` vai intacto e na ordem ao processo do harness, sem lista de permitidos; texto que começa com `/` vai literalmente (ver `docs/02-protocol-spec.md`). `agent.raw` aparece no `stream()` e em `on("agent.raw", fn)`.
```python
agent = Agent(harness="codex", effort="high", harness_args=["--add-dir", "../x"])
agent.on("agent.raw", lambda p: print("[raw]", p["stream"], p["line"]))
agent.prompt("/compact")
# run: agent.run({"nome": "a", "motor": "codex", "harnessArgs": ["--x"]})
```

## Modo WebSocket

Além do STDIO (padrão), o SDK fala com um `openheinerss serve --porta N` já rodando, sem dependências externas:

```python
agent = Agent(harness="mock", transport="websocket", host="127.0.0.1", port=4820)  # port padrão: OPENHEINERSS_PORTA ou 4820
agent = Agent(harness="mock", url="ws://127.0.0.1:4820/ws", origin="http://localhost:3000")
```

A API é a mesma nos dois modos. O servidor só aceita `Origin` local ou as de `OPENHEINERSS_ORIGENS`; origem recusada levanta `TransportError` (HTTP 403). Se a conexão cair, as chamadas pendentes falham com `TransportError`.
