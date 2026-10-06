# 🔌 Adaptadores de Harness (Harness Adapters)

No coração do Openheinerss em Go está a interface `Harness`. Cada motor suportado implementa este contrato.

---

## 1. O Contrato em Go (`pkg/harness/harness.go`)

```go
package harness

import (
	"context"
	"io"
)

// EventType define o tipo normalizado de evento emitido pelo agente
type EventType string

const (
	EventThinking   EventType = "thinking"
	EventText       EventType = "text"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventPermission EventType = "permission"
	EventComplete   EventType = "complete"
	EventError      EventType = "error"
)

type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
}

type SessionConfig struct {
	CWD            string            `json:"cwd"`
	Provider       string            `json:"provider"`
	Model          string            `json:"model"`
	Env            map[string]string `json:"env"`
	PermissionMode string            `json:"permission_mode"`
	SystemPrompt   string            `json:"system_prompt,omitempty"`
}

type Mode string

const (
	ModeSDK Mode = "sdk"
	ModeCLI Mode = "cli"
)

type PrerequisiteResult struct {
	Satisfied    bool     `json:"satisfied"`
	MissingItems []string `json:"missing_items,omitempty"`
	SuggestedFix string   `json:"suggested_fix,omitempty"`
}

// Harness é a interface universal que todo motor precisa satisfazer
type Harness interface {
	Name() string
	Mode() Mode
	ValidatePrerequisites(ctx context.Context) PrerequisiteResult
	Start(ctx context.Context, cfg SessionConfig) error
	SendPrompt(ctx context.Context, text string, attachments []Attachment) error
	RespondPermission(ctx context.Context, reqID string, allow bool) error
	Events() <-chan Event
	Stop() error
}
```

---

## 2. Motores Iniciais e suas Estratégias

### A. Mock Harness (`mock`)
- **Finalidade**: Testes automatizados de loopback e desenvolvimento de frontends (React, CLI, PHP, TS) sem gastar tokens nem precisar de conexão com a internet.
- **Comportamento**: Simula de forma determinística raciocínio (`agent.thinking`), resposta de texto com streaming (`agent.text`), chamada de ferramenta (`agent.tool_call`) e requisição de permissão de alta criticidade (`agent.permission_request`).

### B. Claude Code Harness (`claude-code`)
Possui implementação em dois modos:
1. **Modo SDK (`claude-code-sdk`)**:
   - Executa um worker Node empacotado que inicializa o `@anthropic-ai/claude-agent-sdk`.
   - Utiliza isolamento estrito de perfis via `CLAUDE_CONFIG_DIR` em `~/.openheinerss/profiles/claude-<provider>`.
   - Gerencia a fila assíncrona `InputQueue` e o handshake síncrono `canUseTool` ⇄ `agent.permission_request`.
   - Suporta checkpoints nativos (`enableFileCheckpointing`) e `rewindFiles`.
2. **Modo CLI (`claude-code-cli`)**:
   - Invoca diretamente o binário `claude` instalado no sistema com flags de streaming NDJSON.
   - Ideal para ambientes onde o usuário já possui o CLI oficial autenticado e não quer gerenciar runtime Node adicional.

### C. OpenCode Harness (`opencode`)
1. **Modo CLI (`opencode-cli`)**:
   - Dispara o binário nativo do OpenCode Interpreter.
2. **Modo API / Local Server (`opencode-api`)**:
   - Conecta-se diretamente aos endpoints e APIs compatíveis com OpenAI, DeepSeek, Ollama e Groq.

### D. OpenAI Codex / Assistants Harness (`codex`)
- **Finalidade**: Integração com a API OpenAI Assistants e modelos Codex / GPT-4o / o1 / o3-mini.
- **Mecanismo**: Gerencia o ciclo de Threads, Runs e RunSteps. Mapeia automaticamente as tool calls (funções e code interpreter) para `agent.tool_call` e normaliza os deltas de texto para streaming contínuo.

### E. Google Antigravity Suite Harness (`agy`)
- **Finalidade**: Integração com a suíte de ferramentas do Google Antigravity e agy CLI.
- **Mecanismo**: Conexão com subprocessos `agy` em modo streaming JSON, expondo raciocínio avançado Gemini Pro/Flash e MCP tools compartilhados.

### F. Aider Pair Programming Harness (`aider`)
- **Finalidade**: Integração com o renomado motor de pair programming Aider.
- **Mecanismo**: Executa o processo `aider` com flags headless e formatação NDJSON. Suporta auto-commits Git granulares, mapa de repositório inteligente e suporte out-of-the-box para modelos locais via Ollama (`--model ollama/...`).

---

## 3. Controle de Cache e Eficiência de Tokens

Cada harness possui estratégias de cache tratadas pelo Openheinerss:

1. **Anthropic Prompt Caching**:
   - O adaptador `claude-code` preserva blocos estáveis de contexto (instruções de sistema, schemas de ferramentas MCP e turnos anteriores) usando marcadores de cache ephemeral.
   - Reduz a latência de TTFT (Time to First Token) e corta até 90% do consumo de tokens de entrada em conversas contínuas.
   - **Bypass Automático**: Provedores de terceiros que retornam erro ao receber headers de cache da Anthropic recebem automaticamente a variável `DISABLE_PROMPT_CACHING=1`.

2. **KV-Cache para Modelos Locais (Ollama / vLLM)**:
   - Em modelos locais rodando no Ollama (ex: `qwen2.5-coder`, `deepseek-r1`), o Openheinerss mantém sessões com prefixo imutável. O engine do Ollama mantém a memória Key-Value (KV-cache) na VRAM, evitando reprocessar o contexto do projeto.

3. **Transcript Persistence Cache**:
   - Cada sessão grava cada delta e tool call no formato JSONL em `.openheinerss/sessions/<id>.jsonl`. Ao invocar `session.resume`, o Openheinerss recarrega o estado localmente sem precisar reenviar todo o histórico pela rede.

---

## 4. Modelos Locais e Execução Offline

O Openheinerss oferece 3 opções para rodar sem dependência de nuvem:
- **`opencode` com Ollama**: `--model ollama/qwen2.5-coder:32b` conecta-se a `http://127.0.0.1:11434/v1`.
- **`aider` com Ollama**: `--model ollama/deepseek-r1:14b` utiliza o runtime do Aider para raciocínio local e edição de código.
- **`mock` Determinístico**: 100% offline, ideal para CI/CD, testes de regressão e validação de interfaces React/TypeScript/PHP/Python.

---

## 5. Catálogo Dinâmico de Provedores e Modelos por Harness

Cada adaptador expõe quais provedores ele aceita:

```go
type HarnessMeta struct {
	ID                 string     `json:"id"`
	DisplayName        string     `json:"display_name"`
	SupportedModes     []Mode     `json:"supported_modes"`     // ["sdk", "cli"]
	SupportedProtocols []string   `json:"supported_protocols"` // ["anthropic", "openai", "ollama"]
	DefaultProviders   []Provider `json:"default_providers"`
}
```

Quando o usuário seleciona **OpenCode**, o Openheinerss automaticamente disponibiliza os provedores compatíveis com OpenCode. Quando seleciona **Claude Code**, ele expõe os endpoints compatíveis com o formato Anthropic.


