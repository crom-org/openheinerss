# Manual do CLI — referência gerada

Este arquivo é gerado pelo comando `openheinerss docs` a partir do `--help` real. Não edite manualmente.

## `openheinerss --help`

```text
🎼 Openheinerss (crom-org)
Regendo a orquestra universal de agentes e harnesses de IA.
Unifica Claude Code, OpenCode, Codex e outros sob um único protocolo JSON-RPC de alta performance.

Usage:
  openheinerss [command]

Available Commands:
  docs        Gera ou verifica o manual do CLI em docs/09-cli.md
  doctor      Verifica ferramentas, dependências e pré-requisitos do sistema
  harness     Lista, instala e testa harnesses
  init        Inicializa o diretório .openheinerss no repositório atual
  limites     Mostra as cotas locais das instâncias Codex e Claude
  mcp         Gerencia os servidores MCP (Model Context Protocol) do projeto
  motores     Lista perfis de motores e papéis configurados
  rodar       Executa uma missão com worktree, log e retomada
  run         Executa um prompt interativo no terminal usando o harness escolhido
  serve       Inicia o servidor de orquestração Openheinerss (STDIO ou WebSocket)
  version     Exibe a versão do Openheinerss

Use "openheinerss [command] --help" for more information about a command.

```

## `openheinerss docs --help`

```text
Gera ou verifica o manual do CLI em docs/09-cli.md

Usage:
  openheinerss docs [flags]

Flags:
      --check   Falha se docs/09-cli.md não corresponder ao --help atual

```

## `openheinerss doctor --help`

```text
Verifica ferramentas, dependências e pré-requisitos do sistema

Usage:
  openheinerss doctor [flags]

Flags:
      --harness string   Harness específico para validar pré-requisitos

```

## `openheinerss harness --help`

```text
Lista, instala e testa harnesses

Usage:
  openheinerss harness [command]

Available Commands:
  add         Valida e copia um arquivo de harness para o projeto
  list        Lista harnesses embutidos e custom
  test        Executa um prompt curto e mostra eventos

Use "openheinerss harness [command] --help" for more information about a command.

```

## `openheinerss init --help`

```text
Inicializa o diretório .openheinerss no repositório atual

Usage:
  openheinerss init

```

## `openheinerss limites --help`

```text
Mostra as cotas locais das instâncias Codex e Claude

Usage:
  openheinerss limites [flags]

Flags:
      --json   Emite JSON

```

## `openheinerss mcp --help`

```text
Gerencia os servidores MCP (Model Context Protocol) do projeto

Usage:
  openheinerss mcp [command]

Available Commands:
  add         Registra um novo servidor MCP local no projeto
  list        Lista os servidores MCP configurados em .openheinerss/mcp.json

Use "openheinerss mcp [command] --help" for more information about a command.

```

## `openheinerss motores --help`

```text
Lista perfis de motores e papéis configurados

Usage:
  openheinerss motores

```

## `openheinerss rodar --help`

```text
Executa uma missão com worktree, log e retomada

Usage:
  openheinerss rodar <nome> <instância|harness> [flags]

Flags:
      --agentes string         Alias de --pasta-agentes
      --branch-base string     Branch base da worktree (padrão main)
      --carga-maxima float     Carga máxima de 1 minuto; 0 desativa
      --cota-max float         Pula instâncias com uso de cota igual ou acima deste percentual (0 desativa)
      --esforco string         Esforço de raciocínio
      --max-agentes int        Máximo de agentes simultâneos
      --modelo string          Modelo a usar
      --pasta-agentes string   Pasta dos agentes (padrão .claude/agentes)
      --prompt string          Arquivo de prompt alternativo
      --retomar                Acrescenta o texto de continuação e preserva o log
      --tentativas int         Máximo de tentativas

```

## `openheinerss run --help`

```text
Executa um prompt interativo no terminal usando o harness escolhido

Usage:
  openheinerss run [prompt] [flags]

Flags:
      --esforco string    Esforço de raciocínio do motor
      --harness string    Nome do harness ('mock', 'claude-code', 'opencode') (default "mock")
      --mode string       Modo do harness ('mock', 'sdk', 'cli') (default "mock")
      --model string      Nome do modelo
      --modelo string     Alias em português de --model
      --motor string      Harness base ou instância custom definida pelo usuário
      --papel string      Papel definido em .openheinerss/motores.yaml
      --provider string   Provedor do modelo

```

## `openheinerss serve --help`

```text
Inicia o servidor de orquestração Openheinerss (STDIO ou WebSocket)

Usage:
  openheinerss serve [flags]

Flags:
      --host string   Host de vinculação do WebSocket (default "127.0.0.1")
  -p, --port int      Porta para o servidor WebSocket (alias de --porta) (default 4820)
      --porta int     Porta para o servidor WebSocket (default 4820)
      --stdio         Executa via pipes padrão STDIO (JSON-RPC / NDJSON)

```

## `openheinerss version --help`

```text
Exibe a versão do Openheinerss

Usage:
  openheinerss version

```

## `openheinerss harness add --help`

```text
Valida e copia um arquivo de harness para o projeto

Usage:
  openheinerss harness add <arquivo>

```

## `openheinerss harness list --help`

```text
Lista harnesses embutidos e custom

Usage:
  openheinerss harness list

```

## `openheinerss harness test --help`

```text
Executa um prompt curto e mostra eventos

Usage:
  openheinerss harness test [nome] [flags]

Flags:
      --incluir-principal   Inclui claude-code (conta principal)
      --json                Emite a matriz em JSON (com --todos)
      --modo string         Modo do teste individual (cli ou sdk) (default "cli")
      --prompt string       Prompt curto para o teste
      --pular string        Nomes a pular, separados por vírgula
      --retomar             Envia um segundo prompt na mesma sessão
      --timeout duration    Tempo máximo de cada teste (default 2m0s)
      --todos               Testa todos os harnesses e instâncias em sequência

```

## `openheinerss mcp add --help`

```text
Registra um novo servidor MCP local no projeto

Usage:
  openheinerss mcp add [nome] [comando] [argumentos...]

```

## `openheinerss mcp list --help`

```text
Lista os servidores MCP configurados em .openheinerss/mcp.json

Usage:
  openheinerss mcp list

```

