# Openheinerss (OpenHarness)

O Openheinerss é um binário Go que normaliza sessões de agentes de código por JSON-RPC 2.0/NDJSON. Ele oferece transporte STDIO e WebSocket (`127.0.0.1:4820`), além de uma execução de missões com worktree, logs e retomada.

## Início rápido

```bash
go build -o openheinerss ./cmd/openheinerss
./openheinerss doctor
./openheinerss init
./openheinerss run --harness mock "Analise o repositório"
./openheinerss harness test mock --prompt "responda OK"
```

O `mock` é determinístico, offline e não consome tokens. Para conferir todos os comandos e flags do binário instalado, use `./openheinerss docs` ou leia [docs/09-cli.md](docs/09-cli.md). O manual é gerado automaticamente e tem teste de consistência.

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
- `limites` lê dados locais de cotas de Codex e Claude. `rodar` oferece worktree, log, metadados, retomada, reservas e limites de carga/cota.

## Comandos úteis

```bash
./openheinerss serve --stdio
./openheinerss serve --port 4820
./openheinerss run --harness opencode --mode cli --model ollama/qwen2.5-coder:32b "Escreva testes"
./openheinerss limites --json
./openheinerss mcp list
```

`motores` lista perfis e papéis definidos em `.openheinerss/motores.yaml`, no formato `papel: motor/modelo` e opcionalmente `esforco=low|medium|high`. O arquivo é necessário para o comando; sem ele, use `run` sem `--papel` ou `--motor`.

## Protocolo e SDKs

O protocolo oficial está em [docs/02-protocol-spec.md](docs/02-protocol-spec.md). Os nomes corretos incluem `session.create`, `session.prompt`, `session.permission_respond`, `session.abort`, `session.list`, `catalog.list` e `doctor.check`. Não existe `session.resume`.

Os SDKs disponíveis estão em [sdk/typescript/](sdk/typescript), [sdk/python/](sdk/python) e [sdk/php/](sdk/php). Consulte [docs/04-sdk-any-language.md](docs/04-sdk-any-language.md) para os recursos realmente expostos por cada um.

## Documentação

`docs/` é a única documentação técnica do projeto:

- [Visão geral](docs/00-overview.md) e [arquitetura](docs/01-architecture.md)
- [Protocolo](docs/02-protocol-spec.md), [harnesses](docs/03-harness-adapters.md) e [SDKs](docs/04-sdk-any-language.md)
- [Roadmap](docs/05-roadmap.md), [harness custom](docs/06-harness-custom.md), [missões](docs/07-rodar.md) e [manual do CLI](docs/09-cli.md)
- [Instalação](docs/08-instalacao.md), [testes reais](docs/TESTES-REAIS.md) e [auditoria de lacunas](docs/LACUNAS.md)

## Desenvolvimento

```bash
go build ./...
go vet ./...
go test ./...
```

Licença MIT.
