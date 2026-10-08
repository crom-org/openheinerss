# Estado e próximos passos

Este documento registra o que o repositório entrega hoje. Itens futuros não devem ser marcados como concluídos antes de haver código e teste.

## Entregue

- Núcleo Go, JSON-RPC/NDJSON, transporte STDIO e WebSocket na porta padrão `4820`.
- `init`, `doctor`, `run`, `rodar`, `limites`, `motores`, `mcp`, `harness` e `version`.
- Seis harnesses embutidos, harnesses custom por arquivo e `harness.register` no RPC/SDKs que o suportam.
- `session.create`, `session.prompt`, `session.permission_respond`, `session.abort`, `session.list`, `catalog.list` e `doctor.check`.
- Execução de missões com worktree, logs, metadados, retomada do trabalho e reservas de instância.
- Manual gerado do CLI em [09-cli.md](09-cli.md), validado por teste.

## Parcial ou ainda não implementado

- `pkg/storage` e `pkg/checkpoint` existem, mas não estão conectados ao ciclo da sessão; não há transcript/rollback automático.
- Não há método RPC `session.resume`.
- O hub MCP centraliza `mcp.json`, mas não hospeda servidores nem injeta ferramentas.
- Não há IPC/Unix socket, cliente HTTP API separado do OpenCode, classificador universal de risco ou modo `deny` documentado como política global.
- SDKs ainda não expõem a orquestração `rodar`/`limites` de forma uniforme; Python assíncrono, PHP WebSocket e pacote React separado também não existem.

## Regra para evoluções

Ao implementar um item, atualize o protocolo, o manual, os READMEs dos SDKs e testes de integração. Até lá, a documentação deve descrevê-lo como futuro/parcial.
