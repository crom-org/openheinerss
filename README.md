# Openheinerss

O Openheinerss é um binário Go que faz a ponte entre sessões de agentes de código e harnesses locais. Expõe JSON-RPC 2.0 por STDIO ou WebSocket (`127.0.0.1:4820`), com SDKs para TypeScript, Python e PHP.

## Início rápido

```bash
make build
bin/openheinerss doctor
bin/openheinerss init
bin/openheinerss run --harness mock "responda OK"
bin/openheinerss harness test mock --prompt "responda OK"
bin/openheinerss rodar demo mock --texto "responda OK"
bin/openheinerss agentes listar
```

O `mock` é offline e determinístico. `rodar` precisa ser executado dentro de um repositório Git. Para validar a instalação, use `bin/openheinerss version` e `bin/openheinerss docs --check`.

## O que existe hoje

Há harnesses embutidos `mock`, `claude-code`, `opencode`, `codex`, `agy` e `aider`. Também é possível declarar instâncias em `.openheinerss/harnesses/` ou numa pasta passada por `--config`. Os CLIs são repassados pelo adaptador; o Openheinerss não fornece os motores, modelos, logins ou créditos deles.

O catálogo atual inclui:

- `doctor`, `init`, `version` e `docs`;
- `run` para uma sessão interativa e `serve` para JSON-RPC por STDIO/WebSocket;
- `rodar` (aliases `launch` e `dispatch`) para missões com worktree, branch, logs, metadados, tentativas e retomada;
- `agentes listar|ver|parar|checkpoints|desfazer` para acompanhar, parar e restaurar checkpoints;
- `harness list|add|test` e `capacidades` para descobrir adaptadores e recursos;
- `motores`, `config contexto`, `identidade` e `contas` para perfis, configuração, identidade efetiva e contas sem o Openheinerss ler credenciais;
- `limites` para cotas locais de Codex e Claude, com idade da informação;
- `mcp list|add|efetivos` para configuração e entrega de servidores MCP;
- `comandos` para catálogo de comandos nativos dos harnesses.

Cada item acima foi conferido com `bin/openheinerss <comando> --help` no código desta versão; o manual completo é [docs/09-cli.md](docs/09-cli.md).

## Ponte, risco e limites

A ponte encaminha prompts, argumentos, eventos, permissões e saída dos harnesses. Ela não é um classificador de segurança nem bloqueia ferramentas por conta própria. `--classificar-risco` apenas acrescenta `baixo`, `medio` ou `alto` e um motivo a eventos de ferramenta/permissão. O Codex é executado pelo CLI `codex exec --json`; não há uma API paga implícita.

`limites` lê caches/consultas locais de cotas quando configurados. `rodar --cota-max` pode pular uma instância acima do limiar e trocar por outra disponível; isso não cria contas nem renova créditos. `contas` cria e administra diretórios de login, mas deixa o login para o comando nativo do harness e nunca imprime credenciais. `identidade` mostra o vínculo efetivo entre instância, harness e diretório, sem revelar tokens.

## Missões e retomada

Uma missão pode ser simulada com `rodar --seco` (ou `--dry-run`); `--json` mostra o plano mascarado. Em execução, `rodar` mantém worktree, log, `meta.json`, transcript e checkpoints git-sombra. `--retomar`/`--sessao` retomam a missão ou sessão quando o harness suporta isso. `--limite-contexto` pode avisar ou iniciar nova sessão.

O detector `parado` avisa ou interrompe uma missão sem progresso observável; a ação é configurável e a interrupção é retomável. `--filhos-obrigatorios` e `--esperar-filhos` controlam agentes filhos. Checkpoints podem ser listados e desfeitos explicitamente; não há rollback automático antes de cada ferramenta.

## MCP e contexto por projeto

O hub lê `.openheinerss/mcp.json` global/projeto e entrega servidores por execução quando o harness tem suporte. Ele não hospeda servidores nem injeta ferramentas sem configuração. `--sem-mcp` desliga a entrega.

Configuração, motores, limites de contexto e instâncias podem ser globais ou do projeto; a configuração do projeto vence a global. `--projeto` seleciona outro projeto e `--config` seleciona uma pasta explícita. Consulte [docs/02-protocol-spec.md](docs/02-protocol-spec.md) e [docs/04-sdk-any-language.md](docs/04-sdk-any-language.md) para o protocolo e os SDKs realmente disponíveis.

## Instalação

O instalador de release valida SHA-256; dentro de um clone, `install.sh --local` ou `make build` compilam localmente. Também é possível instalar o módulo Go com `go install github.com/crom-org/openheinerss/cmd/openheinerss@latest`.

## Desenvolvimento

```bash
go build ./...
go vet ./...
go test ./...
```

O checklist para publicar os SDKs e o release está em [docs/PUBLICAR.md](docs/PUBLICAR.md). Consulte [docs/README.md](docs/README.md) para distinguir documentação pública de material interno. Licença MIT.
