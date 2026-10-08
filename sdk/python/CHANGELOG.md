# Changelog

## Não lançado
- Adiciona `WebSocketTransport` (RFC 6455, sem dependências) e `Agent(transport="websocket", host=, port=, origin=)` / `url=`; padrão continua STDIO.
- `Agent.stream`/`prompt`: erro RPC de `session.prompt` (ex.: `/compact` sem equivalente) levanta `RuntimeError` em vez de travar; teto `prompt_timeout` (3600 s) e `timeout` por chamada levantam `TimeoutError`.
- Adiciona `list_commands`, `annotate_command`, `confirm_command` (e os aliases camelCase) para `harness.comandos*`.
- Adiciona `harness_args` e `effort` no `Agent`, `harnessArgs` em `run`, `on(método, callback)` e o evento `agent.raw`.

## 1.1.0
- Corrige a porta padrão para 4820 (a 0.1.0 documentava 4799) e remove a referência quebrada a `codex run`.
- Adiciona harnesses, instâncias, rodar, limites e callbacks para eventos `orq.*`.
