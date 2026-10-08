# Executar missões

`openheinerss rodar <nome> <instância|harness>` executa o prompt em
`.claude/agentes/prompts/<nome>.md`, acrescentando `prompts/_regras.md`.
Agentes comuns recebem uma worktree `agente/<nome>`; nomes `missao-*` usam
uma pasta simples. O padrão da worktree é `.claude/agentes` e a branch base é
`main`.

```bash
openheinerss rodar revisor claude-conta2 --modelo claude-sonnet --esforco high
openheinerss rodar missao-relatorio mock --retomar
openheinerss rodar rapido mock --texto "responda OK"
```

`--texto` (`--text`) dá o prompt direto na linha de comando, sem arquivo; `--prompt` aponta outro arquivo. O nome do agente só aceita letras, números, `.`, `_` e `-` (ele vira nome de pasta e de branch).

Por padrão, o runner acrescenta regras curtas ao prompt: trabalhar somente dentro da pasta/worktree do agente e não fazer buscas fora dela (`find /`, `find ~`, `locate` ou varreduras de disco). Use `--sem-regras-padrao` (`--no-default-rules`) para desligá-las. Também é possível definir `regras_padroes: caminho/para/regras.txt` em `.openheinerss/config.yaml`; `sem_regras_padroes: true` desliga as regras pela configuração.

**Código de saída.** O processo termina com o código do `FIM` do log: `0` concluído, `1` erro (tentativas esgotadas), `2` falta de cota sem reserva (ou todas as instâncias acima de `--cota-max`) e `130` parado por Ctrl-C/SIGTERM (o log e o `meta.json` são fechados antes de sair) ou por `agentes parar`.

Cada execução atualiza `logs/<nome>.log` e, atomicamente,
`logs/<nome>.meta.json`. `--retomar` (ou `RETOMAR=1`) preserva o log e adiciona
o texto de continuação. Uma instância custom pode declarar `reserva: [outra]`;
ao encontrar a regex de cota ou um erro, as reservas são tentadas até
`--tentativas` (padrão 4).

Um agente interrompido por Ctrl-C deixa a worktree e o log para o `--retomar`. Limites opcionais: `--carga-maxima`, `--max-agentes`, `--pasta-agentes` e
`--branch-base`. O protocolo JSON-RPC oferece os mesmos recursos pelo método
`run`, com os campos `nome`, `motor`, `prompt`, `retomar` e os limites. Use
`--cota-max 95` (ou `OPENHEINERSS_COTA_MAX`) para pular uma instância que já
atingiu esse percentual e tentar sua `reserva`.

### Contas AGY como instâncias

Contas não ficam no código. Os exemplos `examples/harnesses/agy-conta1.yaml` e
`agy-conta2.yaml` usam o wrapper `agy-conta.sh`, a variável `AGY_CONTA` e
encadeiam `agy-conta1` para `agy-conta2` com `reserva`. Copie-os para
`.openheinerss/harnesses/` e ajuste o caminho do wrapper ao seu checkout; os
logins permanecem nas pastas isoladas do ambiente local.

## Consultar cotas

`openheinerss limites` lê somente os eventos `rate_limits` recentes em cada
`CODEX_HOME` e os arquivos `~/.config/crom-painel/statusline-<instância>.json`
do Claude. `--json` entrega o mesmo resultado para a Central, incluindo
percentual, horário de reinício e idade do dado. Instâncias são declaradas em
`.openheinerss/harnesses` com `base: codex`/`base: claude-code` e seus envs;
nenhuma conta é embutida no programa.
