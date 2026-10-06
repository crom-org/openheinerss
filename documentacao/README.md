# 📚 Documentação Oficial do Openheinerss

Bem-vindo à documentação técnica e arquitetural completa do **Openheinerss** (OpenHarness), o maestro universal de orquestração de **AI Coding Agents** desenvolvido pela organização [crom-org](https://github.com/crom-org).

---

## 🗺️ Mapa de Navegação da Documentação

A documentação está estruturada em tópicos progressivos e detalhados:

| Capítulo | Título | Descrição |
| :--- | :--- | :--- |
| **[01](./01-visao-geral-e-manifesto.md)** | **Visão Geral & Manifesto** | O propósito do Openheinerss, os problemas de fragmentação de CLIs que ele resolve, por que Go foi a linguagem escolhida e os princípios fundamentais. |
| **[02](./02-arquitetura-e-design.md)** | **Arquitetura & Design do Sistema** | Detalhamento do Core em Go, concorrência com Goroutines e canais, multiplexação de transporte (STDIO e WebSocket) e modelo de ciclo de vida de subprocessos. |
| **[03](./03-especificacao-do-protocolo-rpc.md)** | **Especificação do Protocolo JSON-RPC 2.0** | Especificação formal de todos os métodos RPC (`session.start`, `session.prompt`, `session.permission`, `session.abort`, `session.resume`, `catalog.list`, `doctor.run`) e esquemas de eventos normalizados. |
| **[04](./04-guia-completo-de-harnesses.md)** | **Guia Completo dos 6 Harnesses** | Explicação exaustiva de cada motor suportado: `mock`, `claude-code` (SDK/CLI), `opencode` (CLI/API), `codex`, `agy` e `aider`. Modos de operação e configuração. |
| **[05](./05-cache-otimizacao-e-modelos-locais.md)** | **Cache, Otimização de Tokens & Modelos Locais** | Mecanismos de cache (Anthropic Prompt Caching, bypass de cabeçalhos, KV-Cache de GPU em Ollama/vLLM, Transcript JSONL e Checkpoints) e como operar 100% offline. |
| **[06](./06-hub-mcp-e-extensibilidade.md)** | **Hub Centralizado de MCP (Model Context Protocol)** | Como o Openheinerss atua como roteador universal de ferramentas MCP via `.openheinerss/mcp.json`, CLI `openheinerss mcp` e servidores integrados. |
| **[07](./07-checkpoints-seguranca-e-permissoes.md)** | **Segurança, Permissões & Checkpoints de Arquivos** | Interceptação de ferramentas com risco de segurança, handshake síncrono `agent.permission_request`, snapshots automáticos de arquivos e mecanismo de rollback (`rewindFiles`). |
| **[08](./08-sdks-oficiais.md)** | **Guia de SDKs Oficiais da Comunidade** | Manual exaustivo de integração para TypeScript/Node/React (`@openheinerss/sdk`), PHP/Laravel (`openheinerss-sdk`) e Python (`openheinerss`). |
| **[09](./09-manual-do-cli.md)** | **Manual de Referência do CLI Go** | Guia completo de comandos do binário: `doctor`, `init`, `run`, `serve`, `mcp`, `version`, flags avançadas e variáveis de ambiente. |
| **[10](./10-guia-de-desenvolvimento-e-extensao.md)** | **Guia de Desenvolvimento & Como Criar Novos Harnesses** | Passo a passo para contribuir, implementar a interface `Harness` em Go para novos motores, rodar suítes de teste e padrões do repositório. |

---

## ⚡ Guia Rápido de Instalação

```bash
# Clonar o repositório oficial
git clone https://github.com/crom-org/openheinerss.git
cd openheinerss

# Compilar o binário em Go
make build

# Instalar no PATH do usuário (~/.local/bin)
./install.sh

# Verificar o ambiente
openheinerss doctor
```
