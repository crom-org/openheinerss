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
  comandos    Lista os comandos nativos (/compact, /model…) de um harness ou instância e como são repassados
  config      Mostra a configuração efetiva (global + projeto)
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

## `openheinerss comandos --help`

```text
Lista os comandos nativos do harness (catálogo embutido da base, mais comandos e skills achados
nos arquivos do harness) mesclados com as anotações do usuário em comandos.yaml.
Repasse: literal (vai como está), traduzido (a ponte troca por flag/opção), sem_equivalente (só existe na tela).
Anotações ficam em ~/.config/openheinerss/comandos.yaml; com --config/OPENHEINERSS_CONFIG, em <pasta>/comandos.yaml.

Usage:
  openheinerss comandos <harness> [flags]
  openheinerss comandos [command]

Aliases:
  comandos, commands

Available Commands:
  anotar      Grava uma anotação livre para o comando (vale para as instâncias que herdam do harness)
  confirmar   Marca o primeiro uso do comando como já confirmado

Flags:
      --cwd string   Pasta do projeto onde procurar comandos/skills do harness (padrão: pasta atual)
      --json         Emite JSON

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

Use "openheinerss comandos [command] --help" for more information about a command.

```

## `openheinerss config --help`

```text
Mostra a configuração efetiva (global + projeto)

Usage:
  openheinerss config [command]

Available Commands:
  contexto    Mostra o limite de contexto efetivo e de onde vem cada campo

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

Use "openheinerss config [command] --help" for more information about a command.

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
  efetivos    Lista os servidores que cada harness recebe nesta pasta (global + projeto; valores de env/headers ocultos)
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
      --acao-contexto string        O que fazer ao passar do limite: aviso ou nova-sessao; vence contexto: do config.yaml
      --account string              Alias em inglês de --conta
      --agentes string              Alias de --pasta-agentes
      --agents-dir string           Alias em inglês de --pasta-agentes
      --arg stringArray             Alias de --harness-arg (default [])
      --arquivo-chaves string       Arquivo opcional de variáveis secretas (não imprime valores)
      --base-branch string          Alias em inglês de --branch-base
      --branch-base string          Branch base da worktree (padrão: main, ou a branch atual se não houver main)
      --carga-maxima float          Carga máxima de 1 minuto; 0 desativa
      --child-rounds int            Alias em inglês de --rodadas-filhos
      --conta string                Conta/provedor da instância
      --context-action string       Alias em inglês de --acao-contexto
      --context-limit int           Alias em inglês de --limite-contexto
      --cota-max float              Pula instâncias com uso de cota igual ou acima deste percentual (0 desativa)
      --dry-run                     Alias em inglês de --seco
      --effort string               Alias em inglês de --esforco
      --esforco string              Esforço de raciocínio
      --esperar-filhos string       Espera os agentes filhos e retoma a sessão: sim (padrão, até 2h), nao, ou a espera máxima (ex.: 30m)
      --eventos-log string          Acrescenta FIM ao arquivo de eventos (desligado por padrão)
      --events-log string           Alias em inglês de --eventos-log
      --filhos-obrigatorios         Falha (código 4, motivo "filho falhou") se algum agente filho terminou com código ≠ 0, sem FIM ou ainda rodando
      --harness-arg stringArray     Argumento nativo extra para o harness, intacto e na ordem (repetível) (default [])
      --keys-file string            Alias em inglês de --arquivo-chaves
      --limite-contexto int         Limite de tokens de contexto da sessão (0 desliga); vence contexto: do config.yaml
      --max-agentes int             Máximo de agentes simultâneos
      --max-agents int              Alias em inglês de --max-agentes
      --max-load float              Alias em inglês de --carga-maxima
      --mcp strings                 Entrega só estes servidores de mcp.json (repita ou separe por vírgula; padrão: todos)
      --mode string                 Alias em inglês de --modo
      --model string                Alias em inglês de --modelo
      --modelo string               Modelo a usar
      --modo string                 Modo do harness (cli ou sdk; a instância pode definir o padrão)
      --no-default-rules            Alias em inglês de --sem-regras-padrao
      --no-mcp                      Alias em inglês de --sem-mcp
      --no-rules                    Alias em inglês de --sem-regras
      --pasta-agentes string        Pasta dos agentes (padrão .claude/agentes)
      --prompt string               Arquivo de prompt alternativo
      --quando-carga-abaixo float   Só começa quando a carga numérica ficar abaixo deste valor
      --quota-max float             Alias em inglês de --cota-max
      --regras string               Arquivo de regras do prompt
      --require-children            Alias em inglês de --filhos-obrigatorios
      --resume                      Alias em inglês de --retomar
      --retomar                     Acrescenta o texto de continuação e preserva o log
      --retries int                 Alias em inglês de --tentativas
      --rodadas-filhos int          Máximo de retomadas automáticas depois dos filhos (padrão 5)
      --rules string                Alias em inglês de --regras
      --seco                        Mostra o comando sem executá-lo
      --sem-mcp                     Não entrega ao harness os servidores de mcp.json (global e do projeto)
      --sem-regras                  Não acrescenta regras padrão ao prompt
      --sem-regras-padrao           Desliga as regras padrão do prompt
      --tentativas int              Máximo de tentativas
      --text string                 Alias em inglês de --texto
      --texto string                Prompt em texto, no lugar do arquivo prompts/<nome>.md
      --wait-children string        Alias em inglês de --esperar-filhos
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
      --classificar-risco         Acrescenta risco (baixo|medio|alto) e motivo a tool_call e permission_request; só informa, nunca bloqueia
      --classify-risk             Alias em inglês de --classificar-risco
      --effort string             Alias em inglês de --esforco
      --engine string             Alias em inglês de --motor
      --esforco string            Esforço de raciocínio do motor
      --harness string            Nome do harness ('mock', 'claude-code', 'opencode') (default "mock")
      --harness-arg stringArray   Argumento nativo extra para o harness, intacto e na ordem (repetível) (default [])
      --interactive               Alias em inglês de --interativo
  -i, --interativo                Sessão interativa: lê um prompt por linha; linhas com / vão literalmente ao harness
      --mcp strings               Entrega só estes servidores de mcp.json (repita ou separe por vírgula; padrão: todos)
      --mode string               Modo do harness ('mock', 'sdk', 'cli') (default "mock")
      --model string              Nome do modelo
      --modelo string             Alias em português de --model
      --modo string               Alias em português de --mode (default "mock")
      --motor string              Harness base ou instância custom definida pelo usuário
      --no-mcp                    Alias em inglês de --sem-mcp
      --papel string              Papel definido em .openheinerss/motores.yaml
      --provedor string           Alias em português de --provider
      --provider string           Provedor do modelo
      --resume string             Alias em inglês de --retomar
      --retomar string            Retoma a sessão persistida pelo ID
      --role string               Alias em inglês de --papel
      --sem-mcp                   Não entrega ao harness os servidores de mcp.json (global e do projeto)

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
      --classificar-risco   Acrescenta risco (baixo|medio|alto) e motivo a tool_call e permission_request; só informa, nunca bloqueia
      --classify-risk       Alias em inglês de --classificar-risco
      --deny-ends           Alias em inglês de --negar-encerra
      --hospedeiro string   Alias em português de --host (default "127.0.0.1")
      --host string         Host de vinculação do WebSocket (default "127.0.0.1")
      --max-agentes int     Máximo de agentes simultâneos no servidor
      --max-agents int      Alias em inglês de --max-agentes
      --negar-encerra       Negar em rodar.decidir encerra a execução (código 3, motivo negado) sem nova tentativa
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

## `openheinerss comandos anotar --help`

```text
Grava uma anotação livre para o comando (vale para as instâncias que herdam do harness)

Usage:
  openheinerss comandos anotar <harness> </comando> <texto> [flags]

Aliases:
  anotar, annotate

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config
      --cwd string            Pasta do projeto onde procurar comandos/skills do harness (padrão: pasta atual)
      --json                  Emite JSON

```

## `openheinerss comandos confirmar --help`

```text
Marca o primeiro uso do comando como já confirmado

Usage:
  openheinerss comandos confirmar <harness> </comando> [flags]

Aliases:
  confirmar, confirm

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config
      --cwd string            Pasta do projeto onde procurar comandos/skills do harness (padrão: pasta atual)
      --json                  Emite JSON

```

## `openheinerss config contexto --help`

```text
Mostra a regra de contexto (contexto: no config.yaml) que o rodar usaria.
Ordem: projeto.harnesses[instância] > projeto.harnesses[base] > projeto.padrao >
global.harnesses[instância] > global.harnesses[base] > global.padrao, campo a campo.
O projeto é <repo>/.openheinerss/config.yaml; o global é --config/OPENHEINERSS_CONFIG ou, sem eles,
~/.config/openheinerss/config.yaml e ~/.openheinerss/config.yaml (este vence).

Usage:
  openheinerss config contexto [flags]

Flags:
      --harness string   Instância ou harness a resolver (padrão: o padrão e os citados no config)

Global Flags:
      --config string         Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence OPENHEINERSS_CONFIG e a busca pela pasta atual
      --configuracao string   Alias de --config

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

## `openheinerss mcp efetivos --help`

```text
Lista os servidores que cada harness recebe nesta pasta (global + projeto; valores de env/headers ocultos)

Usage:
  openheinerss mcp efetivos [flags]

Aliases:
  efetivos, effective

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

