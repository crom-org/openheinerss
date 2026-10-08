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
execucao = agent.run({"nome": "teste", "motor": "mock", "prompt": "responda OK", "cwd": "."})
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
