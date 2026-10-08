# Openheinerss (OpenHarness)

O Openheinerss é um binário Go que normaliza sessões de agentes de código por JSON-RPC 2.0/NDJSON. Ele oferece transporte STDIO e WebSocket (`127.0.0.1:4820`), além de uma execução de missões com worktree, logs e retomada.

Todo comando e flag do CLI aceita o nome padrão em português e em inglês; `rodar` também pode ser chamado de `launch` ou `dispatch`, sem alterar o comando interativo `run`.

## Início rápido

```bash
make build
bin/openheinerss doctor
bin/openheinerss init
bin/openheinerss run --harness mock "Analise o repositório"   # o mock pede permissão: responda s ou N
bin/openheinerss harness test mock --prompt "responda OK"     # nega a permissão sozinho e termina em segundos
bin/openheinerss rodar demo mock --texto "responda OK"         # missão com worktree, log e meta.json (exige repositório git)
bin/openheinerss agentes
```

O `mock` é determinístico, offline e não consome tokens. O roteiro completo, passo a passo e com o resultado esperado de cada comando, está em [docs/VERIFICACAO.md](docs/VERIFICACAO.md). Para conferir todos os comandos e flags do binário local, use `bin/openheinerss docs` ou leia [docs/09-cli.md](docs/09-cli.md). O manual é gerado automaticamente e tem teste de consistência.

## Instalação

A instalação recomendada baixa o binário do último release, verifica o SHA-256 e instala em `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/crom-org/openheinerss/main/install.sh | bash
```

Para fixar uma versão, use `curl ... | bash -s -- --versao 0.1.0`. Se o GitHub não estiver disponível e o comando for executado dentro de um clone do projeto, o instalador compila com `go build` como fallback. Se `~/.local/bin` não estiver no `PATH`, adicione-o ao shell.

Também é possível instalar pelo toolchain Go:

```bash
go install github.com/crom-org/openheinerss/cmd/openheinerss@latest
```

Ou baixe manualmente o arquivo `openheinerss_<versão>_<sistema>_<arquitetura>.tar.gz` (ou `.zip` no Windows) na página de releases, confira `checksums.txt`, extraia e coloque o binário no `PATH`. Confira a instalação com `openheinerss version`.

## O que existe hoje

- Harnesses embutidos: `mock`, `claude-code`, `opencode`, `codex`, `agy` e `aider`. Instâncias adicionais podem ser declaradas em `.openheinerss/harnesses/`.
- `claude-code` pode usar CLI ou SDK; os demais adaptadores executam seus CLIs. O `codex` usa `codex exec --json`, não a Assistants API.
- Modelos locais são possíveis quando o CLI correspondente os suporta, por exemplo `opencode --model ollama/...` e `aider --model ollama/...`; o Openheinerss não gerencia o KV-cache.
- `agent.permission_request` é emitido atualmente por `claude-code` e `mock`. O campo `risk` é informativo; não há classificador universal por criticidade. O Codex é executado com bypass de aprovações.
- O hub MCP mantém a configuração em `.openheinerss/mcp.json`; ele não hospeda servidores nem injeta ferramentas automaticamente nos harnesses.
- `limites` lê dados locais de cotas de Codex e Claude. `rodar` oferece worktree, log, metadados, retomada, reservas e limites de carga/cota, e termina com o código do `FIM` (0 ok, 1 erro, 2 sem cota, 130 parado). `agentes` lista, mostra e para os agentes pelos logs.
- Cada sessão grava o transcript em `.openheinerss/sessions/<id>.jsonl` (base do `session.resume` e de `run --retomar`) e um checkpoint por cópia de arquivos antes do primeiro prompt. Não há rollback automático antes de cada ferramenta.
- O servidor WebSocket só aceita conexões sem `Origin` ou de origens locais; outras precisam de `OPENHEINERSS_ORIGENS`.

## Comandos úteis

```bash
bin/openheinerss serve --stdio
bin/openheinerss serve --port 4820
bin/openheinerss run --harness opencode --mode cli --model ollama/qwen2.5-coder:32b "Escreva testes"
bin/openheinerss limites --json
bin/openheinerss harness list
bin/openheinerss agentes ver <nome>
bin/openheinerss mcp list
```

`motores` lista perfis e papéis definidos em `.openheinerss/motores.yaml`, no formato `papel: motor/modelo` e opcionalmente `esforco=low|medium|high`. O arquivo é opcional: sem ele o comando lista só os perfis e informa `Nenhum papel`.

## Protocolo e SDKs

O protocolo oficial está em [docs/02-protocol-spec.md](docs/02-protocol-spec.md). Os nomes corretos incluem `session.create`, `session.prompt`, `session.permission_respond`, `session.abort`, `session.list`, `session.resume`, `catalog.list`, `harness.register` e `doctor.check`, além dos métodos de orquestração `rodar.*`, `limites.obter` e `eventos.assinar` (eventos `orq.*`).

Os SDKs disponíveis estão em [sdk/typescript/](sdk/typescript), [sdk/python/](sdk/python) e [sdk/php/](sdk/php). Consulte [docs/04-sdk-any-language.md](docs/04-sdk-any-language.md) para os recursos realmente expostos por cada um.

## Documentação

`docs/` é a única documentação técnica do projeto:

- [Visão geral](docs/00-overview.md) e [arquitetura](docs/01-architecture.md)
- [Protocolo](docs/02-protocol-spec.md), [harnesses](docs/03-harness-adapters.md) e [SDKs](docs/04-sdk-any-language.md)
- [Roadmap](docs/05-roadmap.md), [harness custom](docs/06-harness-custom.md), [missões](docs/07-rodar.md) e [manual do CLI](docs/09-cli.md)
- [Instalação](docs/08-instalacao.md), [roteiro de verificação](docs/VERIFICACAO.md), [testes reais](docs/TESTES-REAIS.md) e [auditoria de lacunas](docs/LACUNAS.md)
- [Como desenvolvemos com o próprio Openheinerss](docs/COMO-DESENVOLVEMOS.md) e [CHANGELOG](CHANGELOG.md)

## Desenvolvimento

```bash
go build ./...
go vet ./...
go test ./...
```

Licença MIT.
