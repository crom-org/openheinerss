# Amadurecer o openheinerss (anotado pela Central em 08/10/2026)

Leia junto com `PROXIMOS-PASSOS-CROM.md`, que tem os problemas e a ordem já levantados.

**Agente: você deve alterar e melhorar o código do openheinerss livremente.** Corrija, refatore e crie testes
e adaptadores. Commit em português na sua branch; sem push nem publicação sem o ok do usuário.

## Por que importa
A Crom quer trocar fácil de motor e modelo (Codex, Claude conta2, cco com OpenRouter/Zen, modelos grátis ou pagos).
Hoje isso é feito pelo `rodar.sh` (`~/Documentos/GitHub/crom-painel/rodar/rodar.sh`). O openheinerss deve
virar essa camada, mas ainda não está pronto.

## O que falta para substituir o rodar.sh
1. Corrigir o adaptador do Codex: usar `codex exec` (com `--json` para streaming), não `codex run`. O modo API pago fica opcional.
2. Trocar a porta 4799 (conflita com o painel) por 4820 e aceitar `--porta`/env.
3. Suportar como motores os mesmos do `rodar.sh`: `codex` com instâncias configuráveis (`CODEX_HOME`),
   `claude` com `CLAUDE_CONFIG_DIR` (conta2), `cco --provider openrouter|opencode-zen` (modelos `--free`) e `opencode` grátis.
4. Escolher motor e modelo por arquivo de configuração (ex.: `papel: motor/modelo`), para trocar com uma linha.
5. Fazer o que o rodar.sh já faz:
   - trocar de conta ou motor quando a cota acaba;
   - retomar com `RETOMAR=1`;
   - worktree por agente;
   - limite de carga e de agentes simultâneos;
   - `logs/<nome>.log` com `FIM HH:MM código N`, mais o `meta.json`.
6. [x] Método `limites` com as cotas de Codex e Claude, para a Central de Tarefas.
   CLI (`limites`/`--json`), JSON-RPC, idade do dado e `cota_max` no `rodar`.
7. [x] Testes de processo, streaming, retomada, adaptadores com binário falso e
   fixtures anonimizados de limites; verificação com `-race` concluída nesta etapa.

## Primeiro uso real previsto
O "estúdio" de vídeos do crom-videos (papéis roteirista, montador, revisor e finalizador, cada um com motor configurável).
Ele vai começar usando o `rodar.sh` por baixo. Quando o openheinerss passar nos itens acima, troca-se só essa
peça por baixo, sem mudar o estúdio.

## Harness "custom" (pedido do usuário, 08/10)
Hoje só dá para adicionar harness escrevendo Go e chamando `harness.Register` (pkg/harness/harness.go). Falta um harness **custom**:
- Declarado num arquivo (ex.: `.openheinerss/harnesses/<nome>.yaml`), sem recompilar.
- Pode **herdar** de um harness existente (`base: claude-code`) e sobrescrever só o que muda: comando, args, env
  (ex.: `ANTHROPIC_BASE_URL`, `CLAUDE_CONFIG_DIR`), modelo, como ler a saída/eventos e como detectar o fim e a falta de cota.
- Ou ser um harness novo do zero: um comando qualquer que lê o prompt e escreve NDJSON no protocolo.
- Também via código (SDK TS/Python/Go: `registerHarness({...})`).
- Modelo de inspiração: o `cco` (Claude Code com outro provedor). Ex.: `cco-openrouter` = `base: claude-code` + env do provedor.
- Comandos: `openheinerss harness list | add | test <nome>` (teste com prompt curto).

## Ideias (não implementar)
- **Passar o bastão entre motores** (pedido do usuário, 08/10 10:30): quando trocar de motor/conta por cota, o motor novo recebe um resumo do que o anterior fez. Inspirado no ai-memory 2.6 do Akita (github.com/akitaonrails/ai-memory; análise em ~/Documentos/Central/analises/08-akita-post.md). Cuidado: memória grava prompts; excluir pastas sensíveis.
- ✅ **FEITO (Não publicado): Avisar/limitar contexto grande por agente** (dado da Central, 08/10): 63% do uso da conta2 nas últimas 24 h foi com contexto acima de 150k. Ideia: o openheinerss avisar ou limitar o contexto por agente (missões menores; retomar em sessão nova com resumo em vez de continuar a mesma sessão).
