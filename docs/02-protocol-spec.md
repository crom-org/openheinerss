# 📡 Especificação do Protocolo (Openheinerss Protocol)

O protocolo de comunicação entre o cliente e o binário Openheinerss é baseado no padrão **JSON-RPC 2.0 / Linhas JSON (NDJSON)**. Ele funciona de forma bidirecional (full-duplex).

---

## 1. Comandos do Cliente -> Openheinerss (Requests)

### `session.create`
Inicia uma nova sessão de agente com o harness escolhido.
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session.create",
  "params": {
    "harness": "claude-code", // ou "opencode", "codex", "agy"
    "cwd": "/home/user/projeto",
    "provider": "openrouter",
    "model": "anthropic/claude-3.7-sonnet",
    "options": {
      "effort": "high",
      "permissionMode": "ask",
      "systemPrompt": "Você é um assistente sênior..."
    }
  }
}
```

### `session.prompt`
Envia uma instrução ou mensagem do usuário para a sessão ativa.
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session.prompt",
  "params": {
    "sessionId": "sess_abc123",
    "text": "Refatore o arquivo main.go para separar as rotas",
    "images": [
      {
        "mediaType": "image/png",
        "data": "iVBORw0KGgoAAAANSUhEUgAAAA..."
      }
    ]
  }
}
```

### `session.permission_respond`
Responde a um pedido de autorização de execução de ferramenta.
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "session.permission_respond",
  "params": {
    "sessionId": "sess_abc123",
    "requestId": "perm_987",
    "decision": "allow" // ou "deny"
  }
}
```

### `session.abort`
Interrompe imediatamente o processamento atual do agente.
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "session.abort",
  "params": {
    "sessionId": "sess_abc123"
  }
}
```

---

## 2. Eventos do Openheinerss -> Cliente (Notifications / Streams)

Todos os eventos gerados pelos diferentes harnesses são normalizados para estes tipos:

### `agent.thinking`
Streaming do raciocínio interno do modelo (Extended Thinking / CoT).
```json
{
  "jsonrpc": "2.0",
  "method": "agent.thinking",
  "params": {
    "sessionId": "sess_abc123",
    "delta": "Analisando a estrutura do arquivo main.go..."
  }
}
```

### `agent.text`
Streaming da resposta em texto para o usuário.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.text",
  "params": {
    "sessionId": "sess_abc123",
    "delta": "Vou criar o arquivo `routes.go` para modularizar as rotas."
  }
}
```

### `agent.tool_call`
Notificação de chamada de ferramenta em andamento.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.tool_call",
  "params": {
    "sessionId": "sess_abc123",
    "callId": "call_123",
    "tool": "FileEdit",
    "input": {
      "path": "main.go",
      "action": "replace"
    }
  }
}
```

### `agent.tool_result`
Resultado retornado após a execução da ferramenta.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.tool_result",
  "params": {
    "sessionId": "sess_abc123",
    "callId": "call_123",
    "status": "success",
    "output": "1 arquivo alterado: +25 -10 linhas."
  }
}
```

### `agent.permission_request`
Quando uma ferramenta requer aprovação do usuário.
```json
{
  "jsonrpc": "2.0",
  "method": "agent.permission_request",
  "params": {
    "sessionId": "sess_abc123",
    "requestId": "perm_987",
    "tool": "Bash",
    "command": "rm -rf build/",
    "risk": "high"
  }
}
```

---

## 3. Comandos de Sistema e Catálogo

### `catalog.list`
Lista os harnesses disponíveis, modos suportados (`sdk`, `cli`) e modelos compatíveis.
```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "catalog.list"
}
```

### `doctor.check`
Executa o diagnóstico de dependências do ambiente.
```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "doctor.check",
  "params": {
    "harness": "claude-code" // opcional
  }
}
```

---

## 4. Padrão de Erros JSON-RPC Enriquecidos

Quando ocorre um erro de execução ou dependência ausente, a resposta de erro segue o padrão:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": 4010,
    "message": "Pré-requisito ausente: Node.js 18+ não encontrado no PATH",
    "data": {
      "harness": "claude-code",
      "mode": "sdk",
      "missing": ["node"],
      "suggestedFix": "Instale o Node.js v20+ ou execute no modo 'cli' com o binário claude instalado."
    }
  }
}
```

### Códigos de Erro Padronizados
| Código | Nome | Descrição |
| :--- | :--- | :--- |
| `4001` | `SESSION_NOT_FOUND` | ID de sessão inexistente ou expirado |
| `4002` | `HARNESS_NOT_FOUND` | Harness solicitado não existe ou não está registrado |
| `4010` | `HARNESS_DEPENDENCY_MISSING` | Falta um binário ou runtime no SO (Node, Docker, CLI) |
| `4011` | `AUTH_TOKEN_MISSING` | Chave de API ou token de autenticação não configurado |
| `4020` | `PERMISSION_REJECTED` | A ferramenta foi negada pelo usuário ou política de segurança |
| `4030` | `PROCESS_CRASHED` | O subprocesso do agente finalizou inesperadamente |

