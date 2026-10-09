# Plano: openheinerss completo (08/10/2026)

Pedido do usuário: "Eu quero o Open Harness completo." O openheinerss só mexe nos arquivos dele.
Sem push nem release sem o ok do usuário. Todos os agentes saem por `openheinerss rodar` (uso real do projeto).

Já pronto (AMADURECER.md 1–7): `codex exec --json`, porta 4820, harness custom e instâncias por arquivo,
`rodar` (worktree, log/FIM, meta.json, retomar, reserva, carga), `limites`, cobertura dos adaptadores ≥ 67%.

Instâncias do projeto (em `.openheinerss/harnesses/`): `codex`, `claude-conta2`, `opencode-gratis`.
Motores: código e partes difíceis → `codex`/`claude-conta2`; análise → `opencode-gratis`.
A conta principal do Claude não faz trabalho pesado.

| # | Etapa | Motor | Pronto quando |
|---|---|---|---|
| 0 | **Auditoria de lacunas**: README, `docs/` e `documentacao/` comparados com o código (cache, checkpoints, MCP, `session.resume`, modelos locais, permissões, eventos prometidos). Saída: `docs/LACUNAS.md`. | opencode-gratis (análise) + verificação no código | Cada promessa marcada como existe, parcial ou falta, com arquivo e linha. |
| 1 | **Testes reais de todos os harnesses e instâncias**: `openheinerss harness test --todos` e `scripts/teste-real.sh`. Cobre claude-code via CLI e via SDK, claude-conta2, opencode grátis, aider, agy e codex, mais a matriz de resultado em `docs/TESTES-REAIS.md`. | codex | Matriz com OK/falha e o motivo de cada um; falhas corrigidas ou explicadas (sem cota, sem CLI). |
| 2 | **Eventos ao vivo de orquestração no WebSocket** para o crom-central (`inicio`, `progresso`, `fim(codigo)`, `erro`, `precisa_decisao`). `rodar` e `limites` passam a valer pelo servidor, com assinatura por projeto/agente, e o protocolo vai para `docs/02`. | claude-conta2 | Teste e2e: um cliente WS lança `rodar` com mock e recebe a sequência completa; precisa_decisao sai de permission_request. |
| 3 | **SDKs TS, PHP e Python atualizados**: harness custom (`registerHarness`), instâncias, `rodar`, `limites`, eventos da etapa 2, porta 4820. | codex (TS) + codex (PHP/Python) | Testes de cada SDK passando contra o servidor com mock; READMEs dos SDKs com exemplos que rodam. |
| 4 | **Fechar as lacunas da etapa 0**: implementar o que vale a pena e corrigir a documentação do resto, para que nada seja prometido sem existir. | codex / claude-conta2 | `docs/LACUNAS.md` sem itens "falta" em aberto. |
| 5 | **Instalação fácil**: `install.sh` baixa o binário do release (com fallback `go build`), goreleaser e workflow conferidos (`goreleaser check`, snapshot local), `openheinerss version` com versão real, docs de instalação. | codex | `goreleaser release --snapshot --clean` local funciona; `install.sh` testado num diretório limpo. Publicar só com ok. |
| 6 | **openheinerss no próprio desenvolvimento**: `docs/COMO-DESENVOLVEMOS.md` (rodar, instâncias, limites, relatórios), regras em `.claude/agentes/prompts/_regras.md`, comando `openheinerss agentes` (lista agentes, estado e FIM, lendo logs e meta.json). | codex | Etapas 3–5 lançadas e acompanhadas só com o openheinerss. |
| 7 | **Revisão final**: unificar `docs/` e `documentacao/` (sem duplicação), README fiel ao código, CHANGELOG, `go test -race`, cobertura geral, testes reais de novo. | claude-conta2 (revisão) | Tudo verde; resumo para o usuário pedir o push/release. |

Ordem e paralelismo: 0 e 2 juntos; depois 1 e 3; depois 4, 5 e 6; por fim 7. No máximo 2 agentes juntos
(a carga da máquina passa de 10). A Central recebe um aviso no fim de cada etapa.

## Achados no uso real (para a etapa 7)
- `rodar` sem `reserva`: ao detectar falta de cota, não repetir as 4 tentativas na mesma instância; parar logo com `FIM código 2` e a hora de volta da cota (o Claude informa "resets 9am").
- Instâncias do projeto com reserva: `claude-conta2` → `codex`.
- 08/10 08:48–08:49: a detecção de cota do Claude funcionou ("You've hit your session limit"), mas a troca para o Codex foi feita À MÃO (a instância não tinha `reserva`). A troca automática por `reserva` só foi provada em teste com mock; falta prova real.
- 08/10 12:42: agentes rodando `openheinerss rodar` de dentro da própria worktree criaram worktrees aninhadas e branches `agente/*` de teste no repositório real (limpas à mão). Ideia: testes reais do `rodar` sempre em repositório temporário; avisar quando o `rodar` for chamado de dentro de uma worktree de agente.
- 08/10 12:42: uma alteração apareceu sem commit na `main` (cópia da correção de limites feita na worktree do agente) — provável efeito do PWD velho, corrigido nesta rodada.
