# 08 - Guia de SDKs Oficiais da Comunidade

O Openheinerss fornece três SDKs oficiais completos e tipados para permitir que desenvolvedores em qualquer stack integrem o maestro de IA em suas aplicações.

---

## 1. SDK TypeScript & React (`@openheinerss/sdk`)

Localizado em [`sdk/typescript/`](file:///home/j/Documentos/GitHub/openheinerss/sdk/typescript). Compatível com Node.js 18+, Bun, Deno e navegadores modernos.

### A. Uso em Scripts Node.js / Backend
```typescript
import { Openheinerss } from "@openheinerss/sdk";

// Conectar ao servidor Openheinerss via WebSocket ou subprocesso STDIO:
const agent = new Openheinerss({
  endpoint: "ws://localhost:4820",
  options: {
    harness: "claude-code",
    model: "claude-3-5-sonnet-20241022",
    permissionMode: "prompt",
  },
});

// Registrar ouvintes de eventos de streaming:
agent.on("thinking", (delta) => {
  console.log(`[Raciocínio] ${delta}`);
});

agent.on("text", (delta) => {
  process.stdout.write(delta);
});

// Tratar solicitações de permissão:
agent.on("permission", async (req) => {
  console.log(`[Segurança] Solicitação para executar: ${req.command}`);
  // Aprovar ou rejeitar:
  await req.allow();
});

agent.on("complete", (meta) => {
  console.log(`\n[Fim] Duração: ${meta.duration_ms}ms, Tokens: ${meta.output_tokens}`);
});

// Enviar uma tarefa para o agente:
await agent.prompt("Adicione autenticação JWT nos endpoints da API");
```

### B. Hook Reativo para React (`useOpenheinerss`)
```tsx
import React, { useState } from "react";
import { useOpenheinerss } from "@openheinerss/sdk";

export function AssistantChat() {
  const [input, setInput] = useState("");
  const { messages, isThinking, prompt, pendingPermission, respondPermission } = useOpenheinerss({
    endpoint: "ws://localhost:4820",
    harness: "opencode",
  });

  return (
    <div className="chat-container">
      <div className="message-list">
        {messages.map((m, idx) => (
          <div key={idx} className={`message ${m.role}`}>
            {m.content}
          </div>
        ))}
        {isThinking && <p className="thinking">O agente está raciocinando...</p>}
      </div>

      {pendingPermission && (
        <div className="permission-modal">
          <p>O agente deseja executar: <code>{pendingPermission.command}</code></p>
          <button onClick={() => respondPermission(pendingPermission.id, true)}>Autorizar</button>
          <button onClick={() => respondPermission(pendingPermission.id, false)}>Negar</button>
        </div>
      )}

      <form onSubmit={(e) => { e.preventDefault(); prompt(input); setInput(""); }}>
        <input value={input} onChange={(e) => setInput(e.target.value)} placeholder="O que deseja construir?" />
        <button type="submit">Enviar</button>
      </form>
    </div>
  );
}
```

---

## 2. SDK PHP & Laravel (`crom-org/openheinerss-sdk`)

Localizado em [`sdk/php/`](file:///home/j/Documentos/GitHub/openheinerss/sdk/php). Compatível com PHP 8.1+ e Laravel 10/11.

### A. Uso em Scripts PHP Puros
```php
<?php

require_once __DIR__ . '/vendor/autoload.php';

use Openheinerss\Agent;

// Inicializa a sessão com Claude Code ou OpenCode:
$agent = Agent::session([
    'harness'  => 'claude-code',
    'provider' => 'anthropic',
    'cwd'      => __DIR__,
]);

// Executa um prompt síncrono:
$resposta = $agent->prompt("Gere um arquivo de migração para a tabela de pedidos");
echo $resposta . PHP_EOL;
```

### B. Streaming e Eventos em PHP
```php
<?php

use Openheinerss\Agent;

$agent = Agent::session(['harness' => 'opencode', 'model' => 'deepseek/deepseek-coder']);

$agent->stream("Escreva testes unitários para o Model User", function($event) {
    if ($event['type'] === 'agent.text') {
        echo $event['data']['delta'];
        flush();
    } elseif ($event['type'] === 'agent.permission_request') {
        // Aceitar automaticamente em ambiente local ou registrar log:
        $event->allow();
    }
});
```

---

## 3. SDK Python (`openheinerss`)

Localizado em [`sdk/python/`](file:///home/j/Documentos/GitHub/openheinerss/sdk/python). Compatível com Python 3.9+.

### A. Uso Síncrono e Streaming com Gerador
```python
from openheinerss import Agent

# Iniciar agente via subprocesso do binário openheinerss ou WebSocket:
agent = Agent(harness="aider", model="ollama/qwen2.5-coder:7b")

# Streaming iterativo com gerador Python:
print("Resposta do agente:")
for event in agent.stream("Refatore a classe PaymentGateway para usar Strategy"):
    if event["type"] == "agent.thinking":
        print(f"[Pensando: {event['data']['delta']}]", flush=True)
    elif event["type"] == "agent.text":
        print(event["data"]["delta"], end="", flush=True)
    elif event["type"] == "agent.permission_request":
        # Autorizar execução:
        event["responder"].allow()
```

### B. Uso Assíncrono com `asyncio`
```python
import asyncio
from openheinerss.aio import AsyncAgent

async def main():
    agent = AsyncAgent(harness="mock")
    async for event in agent.stream("Analise o projeto"):
        if event["type"] == "agent.text":
            print(event["data"]["delta"], end="", flush=True)

asyncio.run(main())
```
