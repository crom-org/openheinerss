# Changelog

## Não publicado

- `harnessVersions(?string $harness)` (`harness.versoes`) e `updateHarness(string $harness, array $options)` (`harness.atualizar`; devolve `diagnostico`/`recomendacao`, sem voltar sozinho) e `revertHarness(string $harness, ?string $versao)` (`harness.voltar`).
- `sendMessage($agente, $texto)` (`rodar.mensagem`): entrega um recado ao agente vivo; o evento `orq.mensagem` passa a ser repassado.

## 1.7.0 — 2026-10-09
- Versão do pacote alinhada ao release do Openheinerss; inclui a API de orquestração disponível nesta versão.

## 1.2.0 — 2026-10-09
- `filhosObrigatorios` em `run` (o pai termina com código 4, motivo "filho falhou"); callback para `orq.filhos_orfaos` em `subscribeEvents`; `orq.fim` traz `filhos`.
- Adiciona `WebSocketTransport` (RFC 6455, sem dependências), `TransportInterface` e as opções `transport => 'websocket'`, `host`, `port`, `origin`, `url`; padrão continua STDIO.
- Corrige `Agent::session()` retornando sem erro quando o servidor encerra durante `session.create`.
- `Agent::prompt`: erro RPC de `session.prompt` (ex.: `/compact` sem equivalente) lança `RuntimeException` em vez de travar; teto `$promptTimeout` (3600 s) e `$timeout` por chamada.
- Adiciona `listCommands`, `annotateCommand`, `confirmCommand` para `harness.comandos*`.
- Adiciona `harnessArgs` e `effort` nas opções da sessão, `harnessArgs` em `run`, `on($método, $callback)` e o evento `agent.raw`.

## 1.1.0
- Corrige a porta padrão para 4820 (a 0.1.0 apontava para 4799) e documenta `codex exec` no lugar de `codex run`.
- Adiciona registro/listagem de harnesses, instâncias, rodar, limites e callbacks dos eventos `orq.*`.
