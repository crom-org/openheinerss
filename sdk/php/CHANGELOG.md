# Changelog

## Não lançado
- Adiciona `listCommands`, `annotateCommand`, `confirmCommand` para `harness.comandos*`.
- Adiciona `harnessArgs` e `effort` nas opções da sessão, `harnessArgs` em `run`, `on($método, $callback)` e o evento `agent.raw`.

## 1.1.0
- Corrige a porta padrão para 4820 (a 0.1.0 apontava para 4799) e documenta `codex exec` no lugar de `codex run`.
- Adiciona registro/listagem de harnesses, instâncias, rodar, limites e callbacks dos eventos `orq.*`.
