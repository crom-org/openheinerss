# 05 - Cache, Otimização de Tokens & Modelos Locais

Este guia explica como o Openheinerss gerencia a retenção de contexto, otimiza o consumo de tokens e viabiliza a execução de modelos de inteligência artificial de forma 100% offline e privada.

---

## 1. As 5 Camadas de Otimização de Cache

A orquestração de IA para código envolve o envio recorrente de grandes volumes de texto (árvore de arquivos, schemas de MCP, instruções do sistema e histórico de turnos). Sem uma estratégia sólida de cache, a latência de resposta se torna inaceitável e os custos de API disparam.

```
                  ┌──────────────────────────────────────────────┐
                  │           Prompt do Usuário / Turno         │
                  └──────────────────────┬───────────────────────┘
                                         │
        ┌────────────────────────────────┴──────────────────────────────┐
        ▼                                                               ▼
[Modelos de Nuvem (Anthropic)]                         [Modelos Locais (Ollama)]
        │                                                               │
  Prompt Caching Nativo                                     KV-Cache em GPU (VRAM)
  (Reduz 90% dos tokens de entrada)                         (Acelera TTFT de turnos contínuos)
        │                                                               │
  Bypass Automático (Terceiros)                                         │
  (Injeta DISABLE_PROMPT_CACHING=1)                                     │
        │                                                               │
        └────────────────────────────────┬──────────────────────────────┘
                                         │
                    ┌────────────────────▼───────────────────┐
                    │  Transcript Caching (.jsonl no disco) │
                    │  Checkpoints de Arquivos (Rollback)    │
                    └────────────────────────────────────────┘
```

---

### A. Prompt Caching Nativo da Anthropic (`claude-code`)
- **Como Funciona**: A API da Anthropic permite marcar blocos de texto estáveis como "ephemeral cache".
- **O que é colocado em cache**:
  1. O System Prompt base do assistente.
  2. As definições completas de todas as ferramentas do Model Context Protocol (MCP).
  3. Os turnos anteriores da conversa.
- **Resultado Prático**: Tokens de entrada reutilizados do cache têm **90% de desconto no custo** e são processados em uma fração de segundo (TTFT próximo a zero).

---

### B. O Problema de Provedores de Terceiros e o Bypass Automático
Muitos desenvolvedores utilizam gateways e proxies (como OpenRouter, Zen, LiteLLM ou Groq) fingindo ser a API da Anthropic. No entanto, a maioria desses gateways **não suporta** os cabeçalhos de controle de cache `anthropic-beta: prompt-caching-2024-07-25` e responde com erro `400 Bad Request`.

**Como o Openheinerss resolve**:
Quando uma sessão é configurada com um provedor que não seja a Anthropic direta, o Openheinerss injeta automaticamente no ambiente do subprocesso:
```bash
DISABLE_PROMPT_CACHING=1
```
Isso impede que o CLI ou SDK envie marcações de cache inválidas, garantindo compatibilidade universal sem que o usuário precise configurar nada manualmente.

---

### C. KV-Cache em GPU para Modelos Locais (Ollama / vLLM)
Ao executar modelos locais, o maior custo computacional na fase de pré-preenchimento (*prefill*) é calcular os tensores de atenção das mensagens anteriores.

- Os servidores locais de inferência (Ollama, vLLM e llama.cpp) mantêm os tensores de chave e valor (**KV-Cache**) na memória de vídeo (VRAM) enquanto o prefixo da conversa permanecer inalterado.
- Como o Openheinerss mantém o histórico estruturado e estável a cada turno, o modelo local processa turnos adicionais de forma quase instantânea, reutilizando os tensores já calculados.

---

### D. Transcript Caching em Disco (`.openheinerss/sessions/*.jsonl`)
Todas as mensagens, raciocínios e resultados de ferramentas são gravados incrementalmente em formato NDJSON no caminho do projeto:
```
.openheinerss/sessions/<session_id>.jsonl
```
Ao invocar o método RPC `session.resume`, o Openheinerss lê o transcript diretamente do disco local. Isso permite restaurar o estado da conversa sem a necessidade de reprocessar chamadas externas de bootstrap.

---

### E. Checkpoints de Arquivos & Rollback Local
Antes de executar edições críticas ou comandos de terminal que alteram arquivos do projeto, o subsistema [`pkg/checkpoint`](file:///home/j/Documentos/GitHub/openheinerss/pkg/checkpoint) gera uma fotografia do estado dos arquivos.

Se o usuário rejeitar a modificação ou se o modelo cometer um erro, o Openheinerss restaura os arquivos imediatamente (`rewindFiles`), **sem gastar mais tokens de IA** pedindo para o modelo "desfazer o que fez".

---

## 2. Passo a Passo: Executando 100% Offline com Ollama

Você pode usar o Openheinerss em máquinas desconectadas da internet ou em redes corporativas com políticas rígidas de privacidade.

### Passo 1: Instalar o Ollama e Baixar Modelos de Código
```bash
# Baixar o modelo Qwen 2.5 Coder (recomendado para código):
ollama pull qwen2.5-coder:7b
# ou para máquinas com mais GPU/RAM:
ollama pull qwen2.5-coder:32b

# Baixar o modelo DeepSeek-R1 (para raciocínio avançado):
ollama pull deepseek-r1:14b
```

### Passo 2: Executar com OpenCode e Ollama
```bash
openheinerss run \
  --harness opencode \
  --model ollama/qwen2.5-coder:7b \
  "Crie uma função em Go que calcule fibonacci com memoization"
```

### Passo 3: Executar Pair Programming com Aider e Ollama
```bash
openheinerss run \
  --harness aider \
  --model ollama/deepseek-r1:14b \
  "Refatore a camada de persistência deste projeto"
```

### Passo 4: Teste Determinístico Sem Gastar Tokens (`mock`)
Se você estiver apenas desenvolvendo uma extensão de IDE ou testando sua aplicação:
```bash
openheinerss run --harness mock "Execute uma análise completa do repositório"
```
O motor `mock` simula o ciclo completo de eventos (pensamento, texto, ferramenta e permissão) com zero consumo de tokens e sem exigir nenhuma GPU instalada.
