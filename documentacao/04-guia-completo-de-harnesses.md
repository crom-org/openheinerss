# 04 - Guia Completo dos 6 Harnesses

O coração do Openheinerss é a sua coleção de **Harnesses** (adaptadores de motores de IA). Cada harness encapsula as particularidades de um ecossistema específico e expõe a interface única `harness.Harness`.

---

## 1. Tabela Comparativa de Recursos

| Harness | ID | Modos | Provedores Principais | Modelos Locais? | Checkpoints? | Auto Git Commit? |
| :--- | :--- | :--- | :--- | :---: | :---: | :---: |
| **Mock Engine** | `mock` | SDK, CLI | Simulado (offline) | Sim (0 tokens) | Sim | Não |
| **Claude Code** | `claude-code` | SDK, CLI | Anthropic, OpenRouter, Zen | Não | Sim | Não |
| **OpenCode** | `opencode` | CLI, API | Ollama, DeepSeek, OpenAI, Groq | Sim (Ollama) | Sim | Não |
| **OpenAI Codex** | `codex` | API | OpenAI (GPT-4o, o1, o3-mini) | Não | Sim | Não |
| **Google AGY** | `agy` | CLI | Google Gemini 2.5 Pro/Flash | Não | Sim | Não |
| **Aider** | `aider` | CLI | Ollama, OpenAI, Anthropic, OpenRouter | Sim (Ollama) | Sim | Sim |

---

## 2. Detalhamento de Cada Motor

### A. Mock Engine (`mock`)
- **Pacote**: `pkg/harness/mock`
- **Quando usar**: Desenvolvimento local da interface (React, Vue), testes automatizados em CI/CD, testes de estresse de WebSocket sem gastar nenhum token e sem depender de conexão à internet.
- **Comportamento**:
  - Emite `agent.thinking` determinístico.
  - Envia deltas de `agent.text`.
  - Dispara uma solicitação simulada de ferramenta `agent.tool_call`.
  - Se a ferramenta exigir permissão, emite `agent.permission_request` e pausa o fluxo até o usuário responder via `session.permission`.
  - Conclui com métricas simuladas de tokens.

---

### B. Claude Code (`claude-code`)
- **Pacote**: `pkg/harness/claudecode`
- **Quando usar**: Quando se deseja a precisão e o raciocínio avançado do Claude 3.5 Sonnet com o ecossistema oficial da Anthropic.
- **Modos de Operação**:
  1. **Modo SDK (`mode: "sdk"`)**:
     - Executa um worker Node empacotado que roda a biblioteca `@anthropic-ai/claude-agent-sdk`.
     - Utiliza uma fila de entrada assíncrona (`InputQueue`) e o callback de autorização síncrono `canUseTool` interligado ao Go Core.
  2. **Modo CLI (`mode: "cli"`)**:
     - Dispara diretamente o executável nativo `claude` instalado no sistema operacional.
     - Isola perfis de configuração via `CLAUDE_CONFIG_DIR=~/.openheinerss/profiles/claude-<provider>`, garantindo que credenciais de produção e testes não entrem em conflito.

---

### C. OpenCode Interpreter (`opencode`)
- **Pacote**: `pkg/harness/opencode`
- **Quando usar**: Projetos que necessitam de modelos de pesos abertos (DeepSeek-Coder, Qwen 2.5 Coder, Llama 3) ou provedores de baixo custo.
- **Modos de Operação**:
  1. **Modo CLI (`mode: "cli"`)**: Invoca o comando `opencode run` capturando saídas padronizadas.
  2. **Modo API (`mode: "api"`)**: Conecta-se diretamente aos servidores HTTP locais ou remotos compatíveis com o formato OpenAI/Ollama, sem conversão forçada de esquemas.

---

### D. OpenAI Codex / Assistants (`codex`)
- **Pacote**: `pkg/harness/codex`
- **Quando usar**: Fluxos corporativos baseados na plataforma de Assistants da OpenAI e modelos como GPT-4o, o1 e o3-mini.
- **Mecanismo**:
  - Cria e orquestra **Threads** e **Runs** assíncronos.
  - Mapeia automaticamente as tool calls nativas da OpenAI para o protocolo `agent.tool_call` do Openheinerss.
  - Streaming de deltas de texto diretamente para o cliente.

---

### E. Google Antigravity Suite (`agy`)
- **Pacote**: `pkg/harness/agy`
- **Quando usar**: Ambientes que utilizam o ecossistema Google Antigravity e modelos da família Gemini.
- **Mecanismo**:
  - Inicia o processo `agy` em modo de streaming estruturado.
  - Encaminha instruções do usuário e mapeia os pensamentos em tempo real e chamadas de MCP do Gemini para os eventos universais do Openheinerss.

---

### F. Aider Pair Programming (`aider`)
- **Pacote**: `pkg/harness/aider`
- **Quando usar**: Edições em múltiplos arquivos em que o desenvolvedor deseja histórico granular de commits do Git e mapas semânticos do repositório.
- **Destaques**:
  - Suporte completo a modelos locais via Ollama (`openheinerss run --harness aider --model ollama/qwen2.5-coder`).
  - Auto-commit semântico no repositório a cada bloco aprovado.
  - Mapeamento avançado da árvore de código (`repomap`).
