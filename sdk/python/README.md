# Openheinerss Python SDK

SDK oficial em **Python** para integração com o maestro **Openheinerss** (crom-org).

## Instalação

```bash
pip install openheinerss
```

## Exemplo de Uso

```python
from openheinerss import Agent

# Inicia sessão com o harness desejado
agent = Agent(harness="claude-code", provider="openrouter", model="anthropic/claude-3.7-sonnet")

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
