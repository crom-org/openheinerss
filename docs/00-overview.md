# 🎼 Openheinerss (OpenHarness)

> **"Regendo a orquestra universal de agentes e harnesses de IA."**

![Openheinerss Logo](../logo.jpg)

## 1. O que é o Openheinerss?

O **Openheinerss** é um maestro universal de orquestração de **AI Coding Agents**. 

Hoje em dia, a indústria está fragmentada com diversos executáveis, CLIs e SDKs proprietários ou semi-abertos:
- **Claude Code** (Anthropic CLI & Claude Agent SDK)
- **OpenCode** (OpenCode Interpreter / CLI)
- **Codex / OpenAI CLI**
- **Antigravity (AGY)**
- **Aider / SWE-agent / Continue**

Cada um desses ecossistemas inventou:
1. Sua própria forma de rodar processos e subprocessos.
2. Seu próprio formato de mensagens e streaming de eventos.
3. Seu próprio sistema de permissão de ferramentas (`Bash`, `FileEdit`, `Glob`, `Grep`).
4. Seus próprios arquivos de configuração e armazenamento de histórico de sessões.

O **Openheinerss** nasceu para unificar tudo isso sob uma única camada de controle poderosa, escrita em **Go**.

---

## 2. Pilares Fundamentais

### 1. Núcleo Rápido e Portátil (Go Core)
Um único binário Go compilado nativamente, sem dependências pesadas de runtime, que gerencia processos filhos, canais de streaming de alta performance e consumo mínimo de memória.

### 2. SDK Universal Multilinguagem (Agnóstico)
Você não precisa reescrever sua interface se mudar a linguagem da sua aplicação. O Openheinerss se comunica via:
- **STDIO (JSON-RPC 2.0)**: Ideal para plugins de IDE, CLIs e wrappers rápidos.
- **IPC / Unix Sockets**: Comunicação ultrarrápida local entre processos.
- **WebSocket / Server**: Perfeito para interfaces Web, Tauri, Electron e aplicativos mobile.

Qualquer linguagem (Node.js, TypeScript, Python, Rust, Go, C#, PHP) pode consumir o SDK do Openheinerss em minutos.

### 3. Harness Pluggable (Conectores de Execução)
O desenvolvedor ou a interface pode alternar o **Harness** de execução com um único parâmetro:
```json
{
  "harness": "claude-code",
  "provider": "openrouter",
  "model": "qwen/qwen3.8-27b:free"
}
```
ou
```json
{
  "harness": "opencode",
  "provider": "deepseek",
  "model": "deepseek-v3.2"
}
```

---

## 3. Estrutura da Documentação

Nesta pasta `/docs`, você encontrará o detalhamento completo para planejar o desenvolvimento:

- [**01-architecture.md**](./01-architecture.md): A arquitetura do sistema, diagrama de componentes e ciclo de vida.
- [**02-protocol-spec.md**](./02-protocol-spec.md): Especificação do protocolo unificado de mensagens e eventos (JSON-RPC).
- [**03-harness-adapters.md**](./03-harness-adapters.md): Como funcionam os adaptadores de Harness (Claude Code, OpenCode, Codex, AGY).
- [**04-sdk-any-language.md**](./04-sdk-any-language.md): Como implementar clientes SDK em TypeScript, Python, Rust, etc.
- [**05-roadmap.md**](./05-roadmap.md): Roadmap de desenvolvimento, prioridades e primeiros passos.
