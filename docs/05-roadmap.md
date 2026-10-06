# 🗺 Roadmap do Openheinerss

Plano mestre de fases refinado para a construção do projeto, integrando os aprendizados de arquitetura do CCO, suporte dual SDK/CLI, gestão de ambiente e SDKs oficiais.

---

## Fase 1: Fundação do Go Core, Mock Engine & Workspace `.openheinerss` (Semanas 1 e 2)
- [x] Inicializar o módulo Go (`go mod init github.com/crom-org/openheinerss`).
- [x] Estrutura base de pacotes (`cmd/openheinerss`, `pkg/protocol`, `pkg/server`, `pkg/session`, `pkg/harness`, `pkg/config`).
- [x] Implementar a estrutura de comandos CLI com Cobra/pflag:
  - `openheinerss serve --stdio` (transporte via pipes padrão)
  - `openheinerss serve --port 4799` (servidor WebSocket local para web/apps)
  - `openheinerss init` (cria diretório `.openheinerss/` com `config.yaml` e diretórios base)
  - `openheinerss run --harness ...` (execução CLI interativa rápida)
- [x] Motor de transporte JSON-RPC 2.0 / NDJSON bidirecional (STDIO e WebSocket).
- [x] Interface universal Go `Harness` e canais de streaming assíncronos.
- [x] **`MockHarness`**: motor simulado completo para desenvolvimento e testes de ponta a ponta sem internet e sem gastar tokens.

---

## Fase 2: Doctor de Dependências & Claude Code Dual Mode (Semanas 3 e 4)
- [x] **Subsistema `Doctor`**:
  - `openheinerss doctor`: checagem de binários (`node`, `claude`, `opencode`, `docker`) e chaves de API.
  - Validação de pré-requisitos antes de iniciar sessão (`harness.ValidatePrerequisites()`).
  - Códigos de erro RPC padronizados com campo `suggestedFix` e comando de correção guiada (`openheinerss setup`).
- [x] **Claude Code - Modo SDK (`claude-code-sdk`)**:
  - Worker Node empacotado que roda o `@anthropic-ai/claude-agent-sdk`.
  - Canal NDJSON Go ⇄ Worker com handshake de permissões (`canUseTool` ⇄ `agent.permission_request`).
  - Isolamento de perfis em `~/.openheinerss/profiles/claude-<provider>` (`CLAUDE_CONFIG_DIR`).
- [x] **Claude Code - Modo CLI (`claude-code-cli`)**:
  - Subprocesso direto do executável `claude` com flags headless/streaming.
  - Captura de streams e propagação limpa de cancelamento (`SIGINT`/`session.abort`).
- [x] Testes de integração automatizados em Go.

---

## Fase 3: OpenCode Dual Mode & Catálogo Dinâmico de Provedores (Semanas 5 e 6)
- [x] **OpenCode - Modo CLI & API (`opencode`)**:
  - Adaptador para o OpenCode Interpreter / CLI.
  - Conexão direta com modelos OpenAI, DeepSeek, Ollama e Groq sem conversão obrigatória para Anthropic.
- [x] **Catálogo Dinâmico de Modelos (`catalog.list`)**:
  - Detecção e exposição dos provedores e modelos válidos por harness.
- [x] **Harness Selector & Alternância Dinâmica**:
  - Troca de motor na criação de sessão ou migração de contexto entre Claude Code e OpenCode.

---

## Fase 4: Hub Centralizado de MCP, Persistência & Git Rollback (Semanas 7 e 8)
- [x] **Hub Centralizado de MCP (`.openheinerss/mcp.json`)**:
  - Go Core gerencia o ciclo de vida dos servidores MCP (locais via stdio ou remotos via SSE).
  - Repassa as ferramentas MCP padronizadas para qualquer harness conectado.
- [x] **Persistência de Sessões (`.openheinerss/sessions/`)**:
  - Armazenamento em NDJSON/JSONL das mensagens, ferramentas e resultados.
  - Endpoints `session.list` e `session.resume`.
- [x] **Checkpoints & Git Rollback**:
  - Snapshots de arquivos e integração com o sistema de `rewind` antes de comandos arriscados.

---

## Fase 5: SDKs Oficiais da Comunidade & Integração com Front-end CCO (Semanas 9 e 10)
- [x] **TypeScript / JavaScript SDK (`@openheinerss/sdk`)**:
  - Cliente tipado com suporte a Node, Deno, Bun e navegadores.
  - **React Hook (`@openheinerss/react`)**: hooks `useOpenheinerss()` e componentes de chat prontos.
- [x] **PHP SDK (`openheinerss/sdk` no Packagist/Composer)**:
  - Cliente fluente para scripts, Laravel e APIs.
- [x] **Python SDK (`openheinerss` no PyPI)**:
  - Suporte a síncrono e assíncrono (`asyncio`).
- [ ] **Integração com a Interface Web do CCO**:
  - Conectar a interface React + assistant-ui do CCO ao WebSocket do Openheinerss (`ws://localhost:4799`).
- [ ] Extensão VSCode oficial.

