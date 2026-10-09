# Changelog

## Não publicado

- `sendMessage(agente, texto)` (`rodar.mensagem`): entrega um recado ao agente vivo; o evento `orq.mensagem` passa a ser repassado.

## 1.7.0 — 2026-10-09
- Versão do pacote alinhada ao release do Openheinerss; inclui a API de orquestração disponível nesta versão.

## 1.2.0 — 2026-10-09
- `filhosObrigatorios` em `run` (o pai termina com código 4, motivo "filho falhou") e o evento `orq.filhos_orfaos` em `subscribeEvents`; `orq.fim` traz `filhos`.
- Adiciona `listCommands`, `annotateCommand`, `confirmCommand` (`harness.comandos*`) e os tipos `HarnessCommand`/`HarnessCommandList`.
- Adiciona `harnessArgs` e `effort` nas opções de sessão, `harnessArgs` em `run` e o evento `raw` (`agent.raw`).

## 1.1.0
- Corrige a porta padrão para 4820 (0.1.0 apontava para 4799) e elimina a dependência do inexistente `codex run` na documentação.
- Adiciona harnesses, instâncias, rodar, limites e assinatura de eventos `orq.*`.
