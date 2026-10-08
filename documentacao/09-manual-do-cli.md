# 09 - Manual de Referência do CLI Go

O executável `openheinerss` é um binário único e autossuficiente compilado em Go, construído sobre a biblioteca Cobra.

---

## 1. Visão Geral dos Comandos

```
openheinerss [comando] [flags]
```

### Comandos Disponíveis:
- `doctor`: Diagnostica ferramentas instaladas no sistema e pré-requisitos de ambiente.
- `init`: Inicializa o workspace `.openheinerss/` no diretório atual.
- `run`: Executa um prompt interativo diretamente no terminal com qualquer harness.
- `serve`: Inicia o servidor maestro nos modos STDIO ou WebSocket.
- `mcp`: Gerencia servidores de ferramentas do Model Context Protocol.
- `version`: Exibe a versão atual do Openheinerss.

---

## 2. Detalhamento de Comandos e Exemplos

### `openheinerss doctor`
Verifica a presença de binários e configurações necessárias no sistema operacional.
```bash
openheinerss doctor
```
**Saída Típica**:
```
Openheinerss Doctor - System Diagnostics
✔ Node.js: v20.12.0 (/usr/bin/node)
✔ Claude CLI: v0.2.14 (/home/user/.local/bin/claude)
✔ OpenCode CLI: v1.1.0 (/usr/local/bin/opencode)
✔ Aider CLI: v0.45.0 (/home/user/.local/bin/aider)
✔ Ollama: Ativo em http://127.0.0.1:11434
✔ Git: v2.43.0 (/usr/bin/git)
Estado: Todos os requisitos essenciais atendidos!
```

---

### `openheinerss init`
Cria a pasta `.openheinerss/` com as definições de MCP e configurações padrão:
```bash
openheinerss init
```

---

### `openheinerss run`
Dispara uma sessão rápida no terminal para resolver uma tarefa específica:
```bash
# Execução simulada determinística (sem gastar tokens):
openheinerss run --harness mock "Analise o repositório"

# Execução com Claude Code oficial em modo CLI:
openheinerss run --harness claude-code --mode cli "Refatore a função de autenticação"

# Execução com OpenCode e modelo DeepSeek:
openheinerss run --harness opencode --model deepseek/deepseek-chat "Escreva testes unitários"

# Execução local com Ollama e Qwen:
openheinerss run --harness opencode --model ollama/qwen2.5-coder:32b "Crie uma rota HTTP em Go"

# Execução com Aider:
openheinerss run --harness aider "Adicione validação nos formulários"
```

#### Flags do Comando `run`:
- `--harness` (string): Identificador do motor (`mock`, `claude-code`, `opencode`, `codex`, `agy`, `aider`). Padrão: `mock`.
- `--mode` (string): Modo de execução (`cli`, `sdk`, `api`). Padrão depende do harness.
- `--model` (string): Modelo específico a ser utilizado.
- `--provider` (string): Provedor de API (`anthropic`, `openai`, `deepseek`, `openrouter`).
- `--permission-mode` (string): Política de permissões (`prompt`, `auto_allow`, `deny`). Padrão: `prompt`.

---

### `openheinerss serve`
Inicia o processo servidor para atender requisições de clientes, IDEs e aplicações web.

```bash
# Modo STDIO (para extensões de IDE e subprocessos locais):
openheinerss serve --stdio

# Modo WebSocket (para interfaces Web e clientes remotos):
openheinerss serve --port 4820
```

#### Flags do Comando `serve`:
- `--stdio`: Escuta requisições JSON-RPC via entrada e saída padrão (STDIO).
- `--port` (int): Porta de escuta do servidor WebSocket (padrão: `4820`).
- `--host` (string): Host de ligação (padrão: `127.0.0.1`).

---

### `openheinerss mcp`
Gerencia a configuração de servidores MCP em `.openheinerss/mcp.json`.

```bash
# Listar ferramentas configuradas:
openheinerss mcp list

# Adicionar um servidor MCP:
openheinerss mcp add sqlite uvx mcp-server-sqlite --db-path dev.db
openheinerss mcp add git uvx mcp-server-git --repository .
```

---

## 3. Variáveis de Ambiente Globais

O Openheinerss respeita variáveis de ambiente para customização do comportamento:

- `OPENHEINERSS_PORTA`: Porta padrão para o servidor WebSocket; `--porta` e `--port` podem sobrescrevê-la.
- `OPENHEINERSS_HARNESS`: Harness padrão caso não seja especificado.
- `OPENHEINERSS_LOG_LEVEL`: Nível de verbosidade de logs (`debug`, `info`, `warn`, `error`).
- `CLAUDE_CONFIG_DIR`: Sobrescrito automaticamente para isolar perfis de cada provedor em `~/.openheinerss/profiles/claude-<provider>`.
