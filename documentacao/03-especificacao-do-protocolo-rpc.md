# 03 - Especificação do Protocolo JSON-RPC 2.0

O Openheinerss adota estritamente o padrão **JSON-RPC 2.0** com transporte em linhas delimitadas por quebra de linha (`\n`), conhecido como **NDJSON** (Newline Delimited JSON).

Toda mensagem trafegada é um objeto JSON válido em uma única linha.

---

## 1. Métodos de Entrada (Client ➔ Server)

### `session.start`
Inicializa uma nova sessão de agente com o harness escolhido.

**Requisição:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "method": "session.start",
  "params": {
    "harness": "claude-code",
    "mode": "cli",
    "cwd": "/home/user/meu-projeto",
    "provider": "anthropic",
    "model": "claude-3-5-sonnet-20241022",
    "permission_mode": "prompt",
    "system_prompt": "Você é um assistente sênior focado em Go e TDD."
  }
}
```

**Resposta:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "result": {
    "session_id": "sess-9c8e14b2",
    "status": "ready",
    "harness": "claude-code"
  }
}
```

---

### `session.prompt`
Envia uma instrução de texto e anexos opcionais para a sessão ativa.

**Requisição:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-2",
  "method": "session.prompt",
  "params": {
    "session_id": "sess-9c8e14b2",
    "prompt": "Crie um endpoint HTTP /healthz em Go",
    "attachments": [
      {
        "type": "file",
        "path": "main.go"
      }
    ]
  }
}
```

**Resposta:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-2",
  "result": {
    "status": "processing"
  }
}
```

---

### `session.permission`
Responde a uma solicitação bloqueante de autorização previamente emitida pelo agente.

**Requisição:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-3",
  "method": "session.permission",
  "params": {
    "session_id": "sess-9c8e14b2",
    "request_id": "perm-f47a",
    "allow": true
  }
}
```

**Resposta:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-3",
  "result": {
    "acknowledged": true
  }
}
```

---

### `session.abort`
Interrompe imediatamente o processamento corrente e encerra processos filhos com segurança.

**Requisição:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-4",
  "method": "session.abort",
  "params": {
    "session_id": "sess-9c8e14b2"
  }
}
```

---

### `session.resume`
Recarrega e retoma uma sessão histórica persistida a partir de seu ID.

**Requisição:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-5",
  "method": "session.resume",
  "params": {
    "session_id": "sess-9c8e14b2"
  }
}
```

---

### `catalog.list`
Lista dinamicamente todos os motores de harness registrados, seus modos e provedores aceitos.

**Requisição:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-6",
  "method": "catalog.list",
  "params": {}
}
```

**Resposta:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-6",
  "result": {
    "harnesses": [
      {
        "id": "mock",
        "display_name": "Mock Test Engine",
        "supported_modes": ["sdk", "cli"]
      },
      {
        "id": "claude-code",
        "display_name": "Claude Code (Anthropic)",
        "supported_modes": ["sdk", "cli"],
        "supported_protocols": ["anthropic"]
      },
      {
        "id": "opencode",
        "display_name": "OpenCode Interpreter",
        "supported_modes": ["cli", "api"],
        "supported_protocols": ["openai", "ollama", "deepseek"]
      },
      {
        "id": "codex",
        "display_name": "OpenAI Codex / Assistants",
        "supported_modes": ["api"]
      },
      {
        "id": "agy",
        "display_name": "Google Antigravity Suite",
        "supported_modes": ["cli"]
      },
      {
        "id": "aider",
        "display_name": "Aider Pair Programming",
        "supported_modes": ["cli"]
      }
    ]
  }
}
```

---

### `doctor.run`
Executa o diagnóstico completo de dependências do ambiente.

**Requisição:**
```json
{
  "jsonrpc": "2.0",
  "id": "req-7",
  "method": "doctor.run",
  "params": {}
}
```

---

## 2. Eventos de Notificação Streaming (Server ➔ Client)

Notificações JSON-RPC não possuem campo `id` e são emitidas pelo servidor para transmitir o fluxo de execução:

### `agent.thinking`
Transmite blocos de reflexão e deliberação interna do modelo.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.thinking",
  "params": {
    "session_id": "sess-9c8e14b2",
    "delta": "Analisando a estrutura do roteador HTTP..."
  }
}
```

### `agent.text`
Transmite partes parciais do texto de resposta para renderização imediata na tela do usuário.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.text",
  "params": {
    "session_id": "sess-9c8e14b2",
    "delta": "Aqui está o arquivo main.go atualizado:\n\n```go\n"
  }
}
```

### `agent.tool_call`
Notifica que o modelo solicitou a invocação de uma ferramenta ou MCP.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.tool_call",
  "params": {
    "session_id": "sess-9c8e14b2",
    "tool_name": "bash",
    "call_id": "call-1",
    "arguments": {
      "command": "go test -v ./..."
    }
  }
}
```

### `agent.tool_result`
Retorna a saída da ferramenta executada.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.tool_result",
  "params": {
    "session_id": "sess-9c8e14b2",
    "call_id": "call-1",
    "content": "PASS\nok  command line-arguments 0.002s\n",
    "is_error": false
  }
}
```

### `agent.permission_request`
Evento bloqueante emitido antes de executar operações arriscadas.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.permission_request",
  "params": {
    "session_id": "sess-9c8e14b2",
    "request_id": "perm-f47a",
    "severity": "high",
    "tool": "bash",
    "command": "rm -rf build/",
    "reason": "Limpar diretório de compilação antigo antes do build"
  }
}
```

### `agent.complete`
Emitido ao finalizar com sucesso o turno de execução.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.complete",
  "params": {
    "session_id": "sess-9c8e14b2",
    "duration_ms": 2350,
    "input_tokens": 1280,
    "output_tokens": 420
  }
}
```

### `agent.error`
Emitido quando ocorre um erro com mensagem e instrução de correção sugerida.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.error",
  "params": {
    "session_id": "sess-9c8e14b2",
    "code": -32001,
    "message": "Binário 'opencode' não encontrado no PATH",
    "suggestedFix": "Execute: npm install -g opencode-ai ou consulte 'openheinerss doctor'"
  }
}
```
