# 🏛 Arquitetura do Openheinerss

A arquitetura do Openheinerss foi pensada em camadas independentes para desacoplar a **interface de usuário**, o **núcleo de controle** e os **motores de agente (harnesses)**.

---

## Diagrama Geral

```
┌────────────────────────────────────────────────────────────────────────┐
│                        Camada de Apresentação                          │
│   Web / React App  │  VSCode Extension  │  Tauri Desktop  │  CLI TUI   │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    │ JSON-RPC 2.0 / WebSocket / STDIO
                                    │ (Protocolo Unificado de Eventos)
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                    OPENHEINERSS (Go Core Binary)                       │
│                                                                        │
│  ┌───────────────────────┐  ┌───────────────────────────────────────┐  │
│  │   RPC & Transport     │  │          Session Coordinator          │  │
│  │ (STDIO, WebSocket)    │  │  - Identificador de Sessão            │  │
│  └───────────────────────┘  │  - Context / CWD Manager              │  │
│                             │  - Fila de Mensagens & Cancelamento   │  │
│  ┌───────────────────────┐  └───────────────────────────────────────┘  │
│  │ Permission Interceptor│                                             │
│  │ (Allow, Deny, Prompt) │  ┌───────────────────────────────────────┐  │
│  └───────────────────────┘  │         Storage & Transcript          │  │
│                             │  - Unificação de .jsonl               │  │
│                             │  - Checkpoints e Git rollback         │  │
│                             └───────────────────────────────────────┘  │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    │ Interface Interna Go:
                                    │ type Harness interface { ... }
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                     Camada de Adaptadores (Harnesses)                  │
│                                                                        │
│   ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐    │
│   │ ClaudeCodeHarness │ │  OpenCodeHarness  │ │   CodexHarness    │    │
│   │ (Claude Agent SDK │ │ (OpenCode CLI/RPC │ │ (OpenAI Assistant │    │
│   │  & Node Wrapper)  │ │   Native Subproc) │ │   / Codex CLI)    │    │
│   └───────────────────┘ └───────────────────┘ └───────────────────┘    │
│   ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐    │
│   │    AGY Harness    │ │   Aider Harness   │ │  Custom Harness   │    │
│   │   (Google AGY)    │ │   (Python Subproc)│ │   (Wasm / RPC)    │    │
│   └───────────────────┘ └───────────────────┘ └───────────────────┘    │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 1. Camadas do Sistema

### A. Camada de Apresentação (Clients)
Qualquer cliente comunica-se com o Openheinerss através do mesmo protocolo JSON. O cliente **não precisa saber** se o agente subjacente é Claude Code, OpenCode ou Codex. Ele apenas recebe blocos de pensamento, texto, chamadas de ferramentas e pedidos de confirmação.

### B. Go Core (O Maestro)
Escrito em Go por ser altamente concorrente (goroutines), produzir binários únicos estáticos e ter baixíssimo consumo de memória e latência:
1. **Transport Layer**: Escuta requisições do cliente por STDIO ou WebSocket.
2. **Session Coordinator**: Mantém as sessões ativas, lida com timeout, interrupções (SIGINT / cancelamentos limpos) e filas de input.
3. **Permission Interceptor**: Bloqueia ferramentas sensíveis (ex: `rm -rf`, `git push`, comandos bash perigosos) e emite eventos para o cliente aprovar ou rejeitar.
4. **Storage & Rollback**: cada evento da sessão é gravado em `.openheinerss/sessions/<id>.jsonl` (base do `session.resume`), e um checkpoint por cópia de arquivos é criado antes do primeiro prompt em `.openheinerss/checkpoints/`. O checkpoint pula `.git`, dependências (`node_modules`, `vendor`, `dist`, `build`...) e arquivos acima de 10 MB (limite total de 256 MB; o snapshot fica marcado como `parcial`). Não há rollback automático antes de cada ferramenta.

### C. Camada de Adaptadores de Harness
Cada motor de IA possui peculiaridades. O adaptador é responsável por:
- Iniciar o processo específico do motor com suas variáveis de ambiente corretas e perfis isolados.
- Mapear a saída crua do motor para os eventos padronizados do Openheinerss.
- Converter os pedidos de ferramentas do formato proprietário para o formato comum.

#### Suporte Dual: Modo SDK vs. Modo CLI
Para flexibilidade máxima do usuário e da infraestrutura:
* **Modo SDK (Programático / Headless Worker)**: Executa um worker headless isolado usando a biblioteca oficial do agente (ex: `@anthropic-ai/claude-agent-sdk`). Comunica-se com o Go Core via pipes NDJSON limpos. Oferece controle milimétrico sobre streaming, tokens, `canUseTool` e evita conflitos com sequências de escape ANSI de terminais interativos.
* **Modo CLI (Subprocesso Nativo)**: Invoca o executável binário já instalado no sistema operacional do usuário (ex: `claude`, `opencode`). Ideal para quem já tem o CLI instalado e autenticado. O Go Core gerencia o ciclo de vida, repassa sinais de cancelamento (`SIGINT`) e faz a captura estruturada de streams.

---

## 2. Padrão de Diretórios: `.openheinerss/` e `~/.openheinerss/`

O Openheinerss utiliza um modelo de dois níveis para gerenciar estado, configurações e perfis:

### A. Diretório de Projeto (`.openheinerss/`)
Localizado na raiz do repositório/workspace do projeto:
```text
.openheinerss/
├── config.yaml          # Configurações do projeto (harness padrão, permissões, provedores)
├── mcp.json             # Servidores MCP ativos especificamente para este projeto
├── harnesses/           # Instâncias e harnesses custom (um arquivo YAML por instância)
├── motores.yaml         # Opcional: papéis → motor/modelo
├── sessions/            # Transcript de cada sessão (uma linha JSON por evento)
│   └── sess_abc123.jsonl
└── checkpoints/         # Snapshots de arquivos criados antes do primeiro prompt
```

### B. Diretório Global do Usuário (`~/.openheinerss/`)
Localizado na pasta home do usuário:
```text
~/.openheinerss/
├── config.yaml          # Chaves de API globais e preferências padrão
├── profiles/            # Perfis isolados por harness e provedor (evita conflito com ~/.claude)
│   ├── claude-openrouter/
│   └── opencode-zen/
└── shims/               # Workers headless empacotados pelo Openheinerss
```

---

## 3. Subsistema Doctor & Gestão de Dependências

O Openheinerss inclui um motor de diagnóstico (`openheinerss doctor`):
- **Diagnóstico Proativo**: Analisa o ambiente antes da execução e detecta a presença de Node.js, binários de CLI (`claude`, `opencode`), Docker e chaves de API válidas.
- **Resolução de Erros**: Se um pré-requisito faltar, o protocolo pode emitir `suggestedFix`. Não existe comando automático `openheinerss setup`.

---

## 4. Hub Centralizado de Model Context Protocol (MCP)

O Openheinerss mantém uma configuração central em `.openheinerss/mcp.json`, editada por `mcp list` e `mcp add`. Hoje ele não atua como host MCP, não instancia servidores e não injeta ferramentas nos harnesses.
- As ferramentas descobertas são convertidas e injetadas no harness em execução, garantindo que qualquer motor (Claude Code, OpenCode, Codex) tenha acesso às mesmas ferramentas sem duplicar configurações.
