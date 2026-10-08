# Changelog

## Não lançado
- Adiciona `listCommands`, `annotateCommand`, `confirmCommand` (`harness.comandos*`) e os tipos `HarnessCommand`/`HarnessCommandList`.
- Adiciona `harnessArgs` e `effort` nas opções de sessão, `harnessArgs` em `run` e o evento `raw` (`agent.raw`).

## 1.1.0
- Corrige a porta padrão para 4820 (0.1.0 apontava para 4799) e elimina a dependência do inexistente `codex run` na documentação.
- Adiciona harnesses, instâncias, rodar, limites e assinatura de eventos `orq.*`.
