# @openheinerss/sdk

SDK oficial em **TypeScript / JavaScript** para integração com o **Openheinerss** (crom-org).

## Instalação

```bash
npm install @openheinerss/sdk
```

## Uso no Backend / Node / Bun

```typescript
import { Openheinerss } from "@openheinerss/sdk";

const agent = new Openheinerss({
  options: {
    harness: "claude-code", // ou "opencode", "mock"
    provider: "openrouter",
    model: "anthropic/claude-3.7-sonnet"
  }
});

agent.on("thinking", (delta) => console.log("Pensando:", delta));
agent.on("text", (delta) => process.stdout.write(delta));
agent.on("permission", async (req) => {
  console.log("Permissão solicitada para:", req.command);
  await req.allow();
});

await agent.prompt("Refatore a autenticação");
```

## Uso no Frontend / React

```tsx
import { useOpenheinerss } from "@openheinerss/sdk";

export function Chat() {
  const { messages, isThinking, prompt } = useOpenheinerss();

  return (
    <div>
      {messages.map((m) => (
        <div key={m.id}><b>{m.role}:</b> {m.text}</div>
      ))}
      {isThinking && <p>Pensando...</p>}
      <button onClick={() => prompt("Olá Openheinerss!")}>Enviar</button>
    </div>
  );
}
```
