# Changelog

## Não publicado

- `harness_versions(harness=None)` (`harness.versoes`) e `update_harness(harness, seco=…, esperar=…, para=…, simular_falha_teste="versao"|"login")` (`harness.atualizar`; devolve `diagnostico`/`recomendacao`, sem voltar sozinho) e `revert_harness(harness, versao=None)` (`harness.voltar`); `subscribe_events` também recebe `harness.atualizado`.
- `send_message(agente, texto)` / `sendMessage` (`rodar.mensagem`): entrega um recado ao agente vivo; o evento `orq.mensagem` passa a ser repassado.

## 1.7.0 — 2026-10-09
- Versão do pacote alinhada ao release do Openheinerss; inclui a API de orquestração disponível nesta versão.

## 1.2.0 — 2026-10-09
- `filhosObrigatorios` em `run` (o pai termina com código 4, motivo "filho falhou"); `subscribe_events` também recebe `orq.filhos_orfaos`; `orq.fim` traz `filhos`.
- Adiciona `WebSocketTransport` (RFC 6455, sem dependências) e `Agent(transport="websocket", host=, port=, origin=)` / `url=`; padrão continua STDIO.
- `Agent.stream`/`prompt`: erro RPC de `session.prompt` (ex.: `/compact` sem equivalente) levanta `RuntimeError` em vez de travar; teto `prompt_timeout` (3600 s) e `timeout` por chamada levantam `TimeoutError`.
- Adiciona `list_commands`, `annotate_command`, `confirm_command` (e os aliases camelCase) para `harness.comandos*`.
- Adiciona `harness_args` e `effort` no `Agent`, `harnessArgs` em `run`, `on(método, callback)` e o evento `agent.raw`.

## 1.1.0
- Corrige a porta padrão para 4820 (a 0.1.0 documentava 4799) e remove a referência quebrada a `codex run`.
- Adiciona harnesses, instâncias, rodar, limites e callbacks para eventos `orq.*`.
