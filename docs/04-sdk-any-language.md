# 🌐 SDKs Oficiais & Desenvolvimento em Qualquer Linguagem

O Openheinerss foi projetado para que **qualquer aplicativo** (React, TSX, PHP/Laravel, Python, Rust, extensões de IDE, bots de terminal) possa ser construído com facilidade máxima.

O ecossistema se divide em duas camadas:
1. **SDKs Oficiais de Alto Nível**: Pacotes prontos que o desenvolvedor instala via `npm`, `composer` ou `pip` para usar métodos fluentes, tipagem e autocompletar na IDE.
2. **Protocolo Bruto (Sob o capô)**: Linhas JSON-RPC sobre **STDIO** ou **WebSocket**, permitindo que até linguagens sem SDK oficial se integrem em menos de 100 linhas.

---

## 1. Experiência de Uso nos SDKs Oficiais

### A. TypeScript / Node / Bun / React (`npm install @openheinerss/sdk`)

O desenvolvedor não precisa escrever código de socket nem JSON manual. Ele apenas consome a API tipada:

```typescript
import { Openheinerss } from "@openheinerss/sdk";

const agent = new Openheinerss({
  harness: "claude-code",
  mode: "sdk", // ou "cli"
  provider: "openrouter",
  model: "anthropic/claude-3.7-sonnet"
});

// Eventos tipados com autocomplete na IDE
agent.on("thinking", (thought) => console.log("Pensando:", thought));
agent.on("text", (chunk) => process.stdout.write(chunk));
agent.on("permission", async (req) => {
  // Aprova ou rejeita com base na regra do seu app
  if (req.tool === "Bash" && req.command.includes("rm -rf")) {
    await req.deny("Comando perigoso bloqueado");
  } else {
    await req.allow();
  }
});

await agent.prompt("Adicione autenticação JWT no projeto");
```

#### Hook Oficial para React / TSX (`@openheinerss/react`)
Para criar interfaces web em minutos com atualização de estado reativo:

```tsx
import React, { useState } from "react";
import { useOpenheinerss } from "@openheinerss/react";

export function AIAssistant() {
  const [input, setInput] = useState("");
  const { prompt, messages, isThinking, permissionRequest, respondPermission } = useOpenheinerss({
    endpoint: "ws://localhost:4820"
  });

  return (
    <div className="chat-container">
      {isThinking && <div className="spinner">Analisando o código...</div>}
      
      {messages.map((m) => (
        <div key={m.id} className={`message ${m.role}`}>{m.text}</div>
      ))}

      {permissionRequest && (
        <div className="permission-modal">
          <p>O agente deseja rodar: <code>{permissionRequest.command}</code></p>
          <button onClick={() => respondPermission(permissionRequest.id, true)}>Autorizar</button>
          <button onClick={() => respondPermission(permissionRequest.id, false)}>Negar</button>
        </div>
      )}

      <input value={input} onChange={(e) => setInput(e.target.value)} />
      <button onClick={() => { prompt(input); setInput(""); }}>Enviar</button>
    </div>
  );
}
```

---

### B. PHP / Laravel (`composer require openheinerss/sdk`)

Para automações de backend, scripts CLI ou painéis em PHP:

```php
use Openheinerss\Agent;

$agent = Agent::session([
    'harness'  => 'claude-code',
    'provider' => 'deepseek',
    'model'    => 'deepseek-v3.2'
]);

// Resposta direta
$result = $agent->prompt("Gere uma migration para a tabela subscriptions");
echo $result->text();

// Ou streaming em tempo real no terminal/output:
$agent->stream("Refatore o controller", function ($chunk, $type) {
    if ($type === 'text') echo $chunk;
});
```

---

### C. Python (`pip install openheinerss`)

```python
from openheinerss import Agent

agent = Agent(harness="opencode", provider="ollama", model="qwen2.5-coder")

for event in agent.stream("Escreva testes unitários para o módulo auth"):
    if event.type == "thinking":
        print(f"[Pensando] {event.delta}")
    elif event.type == "text":
        print(event.delta, end="", flush=True)
```

---

## 2. Como Funciona Sob o Capô (Protocolo Bruto)

Para qualquer linguagem que não possua um SDK publicado (como Rust, Go, C#, Ruby, Elixir), a implementação é direta porque o Openheinerss fala JSON-RPC 2.0 padrão via:
- **STDIO**: o aplicativo lança `openheinerss serve --stdio` e escreve/lê no terminal do subprocesso.
- **WebSocket**: o aplicativo conecta em `ws://127.0.0.1:4820` e troca mensagens de texto JSON.

### Exemplo em Rust (Cliente Bruto):
```rust
use std::process::{Command, Stdio};
use std::io::{BufRead, BufReader, Write};

pub struct OpenheinerssRawClient {
    child: std::process::Child,
}

impl OpenheinerssRawClient {
    pub fn new() -> Self {
        let child = Command::new("openheinerss")
            .args(["serve", "--stdio"])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .spawn()
            .expect("falha ao iniciar openheinerss");
        Self { child }
    }

    pub fn send(&mut self, json_line: &str) {
        if let Some(ref mut stdin) = self.child.stdin {
            writeln!(stdin, "{}", json_line).unwrap();
        }
    }
}
```

