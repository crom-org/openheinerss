# Relatório do agente

## Mudanças

- Adaptador Codex usa `codex exec --json`, modelo, esforço via `-c model_reasoning_effort=...` e bypass solicitado; converte JSONL em texto, ferramentas, erro, fim e uso de tokens.
- Suporte a retomada com `codex exec resume` quando informado `codex_session_id` ou `resume_session`; `CODEX_HOME` é propagado pelo ambiente da sessão.
- Porta padrão alterada de 4799 para 4820. O CLI aceita `--porta`, mantém `--port` como alias e lê `OPENHEINERSS_PORTA` (também mantém o alias de ambiente antigo).
- SDK TypeScript, exemplo web, README e documentação indicada foram atualizados.
- Criados testes do parser com amostras JSONL, montagem de argumentos/ambiente, flags/env da porta e processo falso no PATH.

## Decisões

- O CLI é o modo padrão do Codex; o modo API explícito permanece compatível como caminho opcional/simulado existente.
- O teste real com `codex exec` não foi executado para não consumir cota; o processo falso cobre o fluxo sem rede nem gasto.

## Verificação

- `go build ./...`: passou.
- `go vet ./...`: passou.
- `go test ./...`: passou — 10 pacotes com testes aprovados; 5 pacotes sem arquivos de teste compilados.
- `git diff --check`: passou.

