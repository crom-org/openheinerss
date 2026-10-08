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
  agentes     Lista e controla agentes em execução
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

Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

Use "openheinerss [command] --help" for more information about a command.

```

## `openheinerss agentes --help`

```text
Lista e controla agentes em execução

Usage:
  openheinerss agentes [flags]
  openheinerss agentes [command]

Aliases:
  agentes, agents

Available Commands:
  listar      Lista os agentes e seus estados
  parar       Para somente o agente informado
  ver         Mostra o fim do log de um agente

Flags:
      --agents-dir string      Alias em inglês de --pasta-agentes (default ".claude/agentes")
      --json                   Emite JSON
      --pasta-agentes string   Pasta dos agentes (relativa à raiz do repositório) (default ".claude/agentes")

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

Use "openheinerss agentes [command] --help" for more information about a command.

```

## `openheinerss docs --help`

```text
Gera ou verifica o manual do CLI em docs/09-cli.md

Usage:
  openheinerss docs [flags]

Aliases:
  docs, documentacao

Flags:
      --check   Falha se docs/09-cli.md não corresponder ao --help atual

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss doctor --help`

```text
Verifica ferramentas, dependências e pré-requisitos do sistema

Usage:
  openheinerss doctor [flags]

Aliases:
  doctor, diagnostico

Flags:
      --harness string   Harness específico para validar pré-requisitos

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

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

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

Use "openheinerss harness [command] --help" for more information about a command.

```

## `openheinerss init --help`

```text
Inicializa o diretório .openheinerss no repositório atual

Usage:
  openheinerss init [flags]

Aliases:
  init, inicializar

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss limites --help`

```text
Mostra as cotas locais das instâncias Codex e Claude

Usage:
  openheinerss limites [flags]

Aliases:
  limites, limits

Flags:
      --json   Emite JSON

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss mcp --help`

```text
Gerencia os servidores MCP (Model Context Protocol) do projeto

Usage:
  openheinerss mcp [command]

Available Commands:
  add         Registra um novo servidor MCP local no projeto
  list        Lista os servidores MCP configurados em .openheinerss/mcp.json

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

Use "openheinerss mcp [command] --help" for more information about a command.

```

## `openheinerss motores --help`

```text
Lista perfis de motores e papéis configurados

Usage:
  openheinerss motores [flags]

Aliases:
  motores, engines

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss rodar --help`

```text
Executa uma missão com worktree, log e retomada

Usage:
  openheinerss rodar <nome> <instância|harness> [flags]

Aliases:
  rodar, launch, dispatch

Flags:
      --account string              Alias em inglês de --conta
      --agentes string              Alias de --pasta-agentes
      --agents-dir string           Alias em inglês de --pasta-agentes
      --arg stringArray             Alias de --harness-arg (default [])
      --arquivo-chaves string       Arquivo opcional de variáveis secretas (não imprime valores)
      --base-branch string          Alias em inglês de --branch-base
      --branch-base string          Branch base da worktree (padrão: main, ou a branch atual se não houver main)
      --carga-maxima float          Carga máxima de 1 minuto; 0 desativa
      --conta string                Conta/provedor da instância
      --cota-max float              Pula instâncias com uso de cota igual ou acima deste percentual (0 desativa)
      --dry-run                     Alias em inglês de --seco
      --effort string               Alias em inglês de --esforco
      --esforco string              Esforço de raciocínio
      --eventos-log string          Acrescenta FIM ao arquivo de eventos (desligado por padrão)
      --events-log string           Alias em inglês de --eventos-log
      --harness-arg stringArray     Argumento nativo extra para o harness, intacto e na ordem (repetível) (default [])
      --keys-file string            Alias em inglês de --arquivo-chaves
      --max-agentes int             Máximo de agentes simultâneos
      --max-agents int              Alias em inglês de --max-agentes
      --max-load float              Alias em inglês de --carga-maxima
      --mode string                 Alias em inglês de --modo
      --model string                Alias em inglês de --modelo
      --modelo string               Modelo a usar
      --modo string                 Modo do harness (cli ou sdk; a instância pode definir o padrão)
      --no-default-rules            Alias em inglês de --sem-regras-padrao
      --no-rules                    Alias em inglês de --sem-regras
      --pasta-agentes string        Pasta dos agentes (padrão .claude/agentes)
      --prompt string               Arquivo de prompt alternativo
      --quando-carga-abaixo float   Só começa quando a carga numérica ficar abaixo deste valor
      --quota-max float             Alias em inglês de --cota-max
      --regras string               Arquivo de regras do prompt
      --resume                      Alias em inglês de --retomar
      --retomar                     Acrescenta o texto de continuação e preserva o log
      --retries int                 Alias em inglês de --tentativas
      --rules string                Alias em inglês de --regras
      --seco                        Mostra o comando sem executá-lo
      --sem-regras                  Não acrescenta regras padrão ao prompt
      --sem-regras-padrao           Desliga as regras padrão do prompt
      --tentativas int              Máximo de tentativas
      --text string                 Alias em inglês de --texto
      --texto string                Prompt em texto, no lugar do arquivo prompts/<nome>.md
      --when-load-below float       Alias em inglês de --quando-carga-abaixo

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss run --help`

```text
Executa um prompt interativo no terminal usando o harness escolhido

Usage:
  openheinerss run [prompt] [flags]

Aliases:
  run, executar

Flags:
      --arg stringArray           Alias de --harness-arg (default [])
      --effort string             Alias em inglês de --esforco
      --engine string             Alias em inglês de --motor
      --esforco string            Esforço de raciocínio do motor
      --harness string            Nome do harness ('mock', 'claude-code', 'opencode') (default "mock")
      --harness-arg stringArray   Argumento nativo extra para o harness, intacto e na ordem (repetível) (default [])
      --interactive               Alias em inglês de --interativo
  -i, --interativo                Sessão interativa: lê um prompt por linha; linhas com / vão literalmente ao harness
      --mode string               Modo do harness ('mock', 'sdk', 'cli') (default "mock")
      --model string              Nome do modelo
      --modelo string             Alias em português de --model
      --modo string               Alias em português de --mode (default "mock")
      --motor string              Harness base ou instância custom definida pelo usuário
      --papel string              Papel definido em .openheinerss/motores.yaml
      --provedor string           Alias em português de --provider
      --provider string           Provedor do modelo
      --resume string             Alias em inglês de --retomar
      --retomar string            Retoma a sessão persistida pelo ID
      --role string               Alias em inglês de --papel

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss serve --help`

```text
Inicia o servidor de orquestração Openheinerss (STDIO ou WebSocket)

Usage:
  openheinerss serve [flags]

Aliases:
  serve, servir

Flags:
      --hospedeiro string   Alias em português de --host (default "127.0.0.1")
      --host string         Host de vinculação do WebSocket (default "127.0.0.1")
      --max-agentes int     Máximo de agentes simultâneos no servidor
      --max-agents int      Alias em inglês de --max-agentes
  -p, --port int            Porta para o servidor WebSocket (alias de --porta) (default 4820)
      --porta int           Porta para o servidor WebSocket (default 4820)
      --stdio               Executa via pipes padrão STDIO (JSON-RPC / NDJSON)

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss version --help`

```text
Exibe a versão do Openheinerss

Usage:
  openheinerss version [flags]

Aliases:
  version, versao

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss agentes listar --help`

```text
Lista os agentes e seus estados

Usage:
  openheinerss agentes listar [flags]

Aliases:
  listar, list

Global Flags:
      --agents-dir string      Alias em inglês de --pasta-agentes (default ".claude/agentes")
      --config string          Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string    Alias de --config
      --json                   Emite JSON
      --pasta-agentes string   Pasta dos agentes (relativa à raiz do repositório) (default ".claude/agentes")

```

## `openheinerss agentes parar --help`

```text
Para somente o agente informado

Usage:
  openheinerss agentes parar <nome> [flags]

Aliases:
  parar, stop

Global Flags:
      --agents-dir string      Alias em inglês de --pasta-agentes (default ".claude/agentes")
      --config string          Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string    Alias de --config
      --json                   Emite JSON
      --pasta-agentes string   Pasta dos agentes (relativa à raiz do repositório) (default ".claude/agentes")

```

## `openheinerss agentes ver --help`

```text
Mostra o fim do log de um agente

Usage:
  openheinerss agentes ver <nome> [flags]

Aliases:
  ver, show

Global Flags:
      --agents-dir string      Alias em inglês de --pasta-agentes (default ".claude/agentes")
      --config string          Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string    Alias de --config
      --json                   Emite JSON
      --pasta-agentes string   Pasta dos agentes (relativa à raiz do repositório) (default ".claude/agentes")

```

## `openheinerss harness add --help`

```text
Valida e copia um arquivo de harness para o projeto

Usage:
  openheinerss harness add <arquivo> [flags]

Aliases:
  add, adicionar

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss harness list --help`

```text
Lista harnesses embutidos e custom

Usage:
  openheinerss harness list [flags]

Aliases:
  list, listar

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss harness test --help`

```text
Executa um prompt curto e mostra eventos

Usage:
  openheinerss harness test [nome] [flags]

Aliases:
  test, testar

Flags:
      --all                 Alias em inglês de --todos
      --incluir-principal   Inclui claude-code (conta principal)
      --json                Emite a matriz em JSON (com --todos)
      --mode string         Alias em inglês de --modo (default "cli")
      --modo string         Modo do teste individual (cli ou sdk) (default "cli")
      --prompt string       Prompt curto para o teste
      --pular string        Nomes a pular, separados por vírgula
      --resume              Alias em inglês de --retomar
      --retomar             Envia um segundo prompt na mesma sessão
      --skip string         Alias em inglês de --pular
      --timeout duration    Tempo máximo de cada teste (default 2m0s)
      --todos               Testa todos os harnesses e instâncias em sequência

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss mcp add --help`

```text
Registra um novo servidor MCP local no projeto

Usage:
  openheinerss mcp add [nome] [comando] [argumentos...] [flags]

Aliases:
  add, adicionar

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

## `openheinerss mcp list --help`

```text
Lista os servidores MCP configurados em .openheinerss/mcp.json

Usage:
  openheinerss mcp list [flags]

Aliases:
  list, listar

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

```

