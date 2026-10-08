# Estado e próximos passos

Este documento registra o que o repositório entrega hoje. Itens futuros não devem ser marcados como concluídos antes de haver código e teste.

## Entregue

- Núcleo Go, JSON-RPC/NDJSON, transporte STDIO e WebSocket na porta padrão `4820`.
- CLI: `init`, `doctor`, `run`, `rodar`, `agentes`, `limites`, `motores`, `mcp`, `harness` (`list`, `add`, `test`), `version` e `docs`; todos com apelido em inglês.
- Seis harnesses embutidos e instâncias/harnesses custom por arquivo, também registráveis em tempo de execução (`harness.register`).
- Sessões: `session.create`, `session.prompt`, `session.permission_respond`, `session.abort`, `session.list`, `session.resume`, `catalog.list` e `doctor.check`; transcript em `.openheinerss/sessions/` e checkpoint inicial por cópia de arquivos.
- Orquestração: `rodar` (worktree, log, `meta.json`, retomada, reservas por cota, limites de carga/agentes/cota), `limites`, `agentes` e, pelo servidor, `rodar.*`, `limites.obter`, `harness.listar`, `eventos.assinar` e os eventos `orq.*`.
- SDKs TypeScript, Python e PHP 0.2.0 com as mesmas chamadas de orquestração.
- Instalação por `install.sh` (checksum) e GoReleaser; manual do CLI gerado e validado por teste.

## Parcial ou ainda não implementado

- Rollback automático antes de cada ferramenta: existe o checkpoint inicial, mas não a restauração automática.
- O hub MCP centraliza `mcp.json`, mas não hospeda servidores nem injeta ferramentas nos harnesses.
- Não há IPC/Unix socket, cliente HTTP API separado do OpenCode nem classificador universal de risco das permissões (`risk` é informativo).
- Python assíncrono (`aio`), WebSocket nos SDKs Python/PHP, `stream()` no PHP e pacote React separado não existem.
- Publicação (tag, release, Packagist, npm, PyPI) depende do ok do dono do projeto.

## Regra para evoluções

Ao implementar um item, atualize o protocolo, o manual, os READMEs dos SDKs, o `CHANGELOG.md` e os testes de integração. Até lá, a documentação deve descrevê-lo como futuro/parcial.
