# 01 - Visão Geral & Manifesto do Openheinerss

> **"Regendo a orquestra universal de agentes e harnesses de IA."**

---

## 1. O Problema: A Torre de Babel dos AI Coding Agents

Com o rápido amadurecimento dos assistentes de código inteligentes e agentes autônomos, o ecossistema de engenharia de software foi inundado por excelentes ferramentas de linha de comando:

- **Claude Code (Anthropic)**: Ferramenta com forte raciocínio em Claude 3.5 Sonnet, integração com bash e edição atômica de arquivos.
- **OpenCode Interpreter**: CLI e API extensível com suporte a modelos open-weights (DeepSeek, Llama, Qwen) e provedores customizados.
- **OpenAI Codex / Assistants**: Arquitetura orientada a Threads e Runs na nuvem da OpenAI.
- **Aider**: Pioneiro no modelo de pair programming via terminal com auto-commits no Git e repomaps.
- **Google Antigravity (AGY)**: Motor focado em raciocínio contínuo com Gemini 2.5 Flash e Pro.

Apesar da excelência de cada projeto, eles geraram um problema crítico de **fragmentação e aprisionamento tecnológico**:

1. **Protocolos Proprietários Incompatíveis**: Cada ferramenta emite texto ou logs em formatos proprietários (ANSI escapes, NDJSON customizado, streamings não documentados ou saídas de terminal com formatação quebrando parsers).
2. **Duplicação de Ferramentas (MCP)**: Cada agente precisa de sua própria configuração de MCP (Model Context Protocol), forçando o desenvolvedor a duplicar arquivos `claude.json`, `opencode.json`, `aider.conf`, etc.
3. **Ausência de Controle de Segurança Centralizado**: Não havia uma camada neutra capaz de interceptar comandos perigosos (`rm -rf`, `DROP TABLE`, scripts de deploy) e solicitar autorização antes da execução.
4. **Isolamento de Linguagens**: Aplicações web em PHP/Laravel, serviços em Python ou painéis em React eram forçados a criar hacks de captura de stdout ou ficavam restritos a uma única biblioteca Node.js proprietária.

---

## 2. A Solução: O Maestro Universal Openheinerss

O **Openheinerss** (OpenHarness) nasceu na organização [crom-org](https://github.com/crom-org) com uma missão clara: **ser o maestro que orquestra qualquer motor de agente de codificação sem aprisionar a aplicação a nenhum fornecedor específico**.

```
                ┌────────────────────────────────────────────────────────┐
                │             APLICAÇÕES & CLIENTES                     │
                │   React / Next.js  •  PHP / Laravel  •  Python Scripts │
                └──────────────────────────┬─────────────────────────────┘
                                           │  JSON-RPC 2.0 / NDJSON
                                           │  (STDIO ou WebSocket :4799)
                ┌──────────────────────────▼─────────────────────────────┐
                │                 OPENHEINERSS GO CORE                   │
                │                                                        │
                │   • Gerenciador de Sessões & Checkpoints de Arquivos   │
                │   • Interceptador de Permissões (Low, Medium, High)   │
                │   • Hub Centralizado de Ferramentas MCP (.openheinerss)│
                │   • Roteador & Normalizador de Eventos em Tempo Real   │
                └──────────────────────────┬─────────────────────────────┘
                                           │
         ┌───────────────┬─────────────────┼────────────────┬───────────────┐
         ▼               ▼                 ▼                ▼               ▼
   [claude-code]    [opencode]          [codex]           [agy]          [aider]
   (SDK ou CLI)   (CLI ou Ollama)     (OpenAI API)     (Google AGY)    (Pair Git)
```

---

## 3. Por Que Go Foi a Linguagem Escolhida?

A fundação do Openheinerss em Go foi uma decisão arquitetural deliberada:

1. **Binário Único e Sem Dependências Externas**: Um único executável estático de ~15MB roda em Linux, macOS e Windows sem exigir runtime Node.js, Python ou JVM pré-instalados na máquina do usuário.
2. **Concorrência Primitiva com Goroutines & Canais**: O gerenciamento simultâneo de processos filhos (CLI), conexões WebSocket assíncronas, streams de stderr/stdout e handshakes de permissões é tratado com altíssima eficiência e baixo consumo de memória.
3. **Tempo de Inicialização Sub-Milissegundo**: Ideal para invocações rápidas via CLI e ferramentas que rodam em loops de CI/CD.
4. **Resiliência a Travamentos de Processos Filhos**: Se um processo do Claude Code ou Aider travar ou falhar, o Go Core captura o sinal, registra o evento de erro de forma estruturada e mantém a sessão em estado recuperável sem derrubar o servidor.

---

## 4. Princípios Fundamentais

1. **Liberdade de Motor (Engine Agnostic)**: Você pode começar uma tarefa com Claude Code, alternar para o DeepSeek no OpenCode ou refatorar localmente com o Ollama no Aider, mantendo exatamente o mesmo frontend.
2. **Contrato Universal de Eventos**: Não importa se o modelo subjacente é da Anthropic, OpenAI ou um modelo aberto local; o cliente recebe os exatos mesmos eventos normalizados (`agent.thinking`, `agent.text`, `agent.tool_call`, `agent.permission_request`, `agent.complete`).
3. **Segurança em Primeiro Lugar (Permission Interception)**: Nenhuma ferramenta destrutiva é executada às cegas. O Openheinerss bloqueia o fluxo até receber confirmação do usuário ou da regra do projeto.
4. **Cidadão de Primeira Classe em Qualquer Linguagem**: Oferece SDKs oficiais e idiomáticos para TypeScript/React, PHP e Python, garantindo que qualquer desenvolvedor consiga integrar IA avançada em seu stack favorito.
