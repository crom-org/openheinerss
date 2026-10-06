# 🎼 Openheinerss (OpenHarness)

> **"Regendo a orquestra universal de agentes e harnesses de IA."**

<p align="center">
  <img src="./logo.jpg" alt="Openheinerss Maestro" width="380" />
</p>

<p align="center">
  <a href="./logo_ascii.txt"><b>[Ver Logo em Arte ASCII Principal]</b></a> • 
  <a href="./logo_alt_ascii.txt"><b>[Ver Logo em Arte ASCII Alternativa]</b></a>
</p>

O **Openheinerss** é um maestro universal de orquestração de **AI Coding Agents** desenvolvido pela organização [crom-org](https://github.com/crom-org).

Em vez de prender sua aplicação ao Claude Code, OpenCode, Codex ou qualquer CLI proprietário, o Openheinerss fornece:
1. **Um binário Go de alta performance** que gerencia processos filhos, permissões bloqueantes e streaming em tempo real.
2. **Um protocolo unificado de mensagens (JSON-RPC 2.0 / NDJSON)** via STDIO e WebSocket (`:4799`).
3. **Múltiplos Harnesses plugáveis** (`claude-code`, `opencode`, `mock`) nos modos **SDK** (headless worker) e **CLI** (subprocesso nativo).
4. **Hub centralizado de MCP (Model Context Protocol)** e persistência de sessões com checkpoints de restauração em `.openheinerss/`.
5. **SDKs Oficiais multilinguagem** para TypeScript/React, PHP e Python.

---

## ⚡ Início Rápido (CLI Go)

### 1. Diagnóstico do Sistema
Verifica se as dependências do ambiente estão prontas:
```bash
openheinerss doctor
```

### 2. Inicializar um Projeto
Cria a pasta `.openheinerss/` com configurações e servidores MCP:
```bash
openheinerss init
```

### 3. Execução Interativa no Terminal
Roda uma tarefa interativa com qualquer harness:
```bash
# Teste simulado determinístico (sem gastar tokens):
openheinerss run --harness mock "Analise o repositório"

# Com Claude Code ou OpenCode:
openheinerss run --harness claude-code --mode cli "Refatore a função de autenticação"
openheinerss run --harness opencode --model deepseek/deepseek-chat "Crie testes unitários"
```

### 4. Iniciar Servidor Maestro
```bash
# Modo STDIO (para extensões de IDE e processos filhos):
openheinerss serve --stdio

# Modo WebSocket (para interfaces Web, React, Tauri, mobile):
openheinerss serve --port 4799
```

---

## 🌐 SDKs Oficiais da Comunidade

### 📦 TypeScript / Node / React (`@openheinerss/sdk`)
Localizado em [`sdk/typescript/`](./sdk/typescript):
```typescript
import { Openheinerss, useOpenheinerss } from "@openheinerss/sdk";

// Backend / Script:
const agent = new Openheinerss({ options: { harness: "claude-code" } });
agent.on("thinking", delta => console.log(delta));
agent.on("text", delta => process.stdout.write(delta));
agent.on("permission", async req => await req.allow());
await agent.prompt("Adicione endpoints na API");

// Frontend / React:
export function Chat() {
  const { messages, isThinking, prompt } = useOpenheinerss();
  return <button onClick={() => prompt("Refatore o layout")}>Enviar</button>;
}
```

### 🐘 PHP / Laravel (`openheinerss-sdk`)
Localizado em [`sdk/php/`](./sdk/php):
```php
use Openheinerss\Agent;

$agent = Agent::session(['harness' => 'opencode', 'model' => 'deepseek-coder']);
$res = $agent->prompt("Gere uma migration para a tabela faturas");
echo $res;
```

### 🐍 Python (`openheinerss`)
Localizado em [`sdk/python/`](./sdk/python):
```python
from openheinerss import Agent

agent = Agent(harness="claude-code", provider="openrouter")
for event in agent.stream("Analise este dataset"):
    if event["type"] == "agent.text":
        print(event["data"]["delta"], end="", flush=True)
```

---

## 🔌 Gerenciamento Centralizado de MCP

Gerencie os servidores de ferramentas do projeto diretamente pelo CLI:
```bash
# Listar servidores configurados:
openheinerss mcp list

# Adicionar um servidor MCP local:
openheinerss mcp add sqlite uvx mcp-server-sqlite --db-path dev.db
```

---

## 📚 Documentação Técnica

Explore os guias detalhados na pasta [`docs/`](./docs):

- [**00 - Visão Geral & Manifesto**](./docs/00-overview.md)
- [**01 - Arquitetura do Sistema**](./docs/01-architecture.md)
- [**02 - Especificação do Protocolo**](./docs/02-protocol-spec.md)
- [**03 - Adaptadores de Harness (Claude Code, OpenCode, Mock)**](./docs/03-harness-adapters.md)
- [**04 - Guia de SDKs Oficiais e Multilinguagem**](./docs/04-sdk-any-language.md)
- [**05 - Roadmap de Desenvolvimento**](./docs/05-roadmap.md)

