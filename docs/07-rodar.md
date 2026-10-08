# Executar missões

`openheinerss rodar <nome> <instância|harness>` executa o prompt em
`.claude/agentes/prompts/<nome>.md`, acrescentando `prompts/_regras.md`.
Agentes comuns recebem uma worktree `agente/<nome>`; nomes `missao-*` usam
uma pasta simples. O padrão da worktree é `.claude/agentes` e a branch base é
`main`.

```bash
openheinerss rodar revisor claude-conta2 --modelo claude-sonnet --esforco high
openheinerss rodar missao-relatorio mock --retomar
```

Cada execução atualiza `logs/<nome>.log` e, atomicamente,
`logs/<nome>.meta.json`. `--retomar` (ou `RETOMAR=1`) preserva o log e adiciona
o texto de continuação. Uma instância custom pode declarar `reserva: [outra]`;
ao encontrar a regex de cota ou um erro, as reservas são tentadas até
`--tentativas` (padrão 4).

Limites opcionais: `--carga-maxima`, `--max-agentes`, `--pasta-agentes` e
`--branch-base`. O protocolo JSON-RPC oferece os mesmos recursos pelo método
`run`, com os campos `nome`, `motor`, `prompt`, `retomar` e os limites. Use
`--cota-max 95` (ou `OPENHEINERSS_COTA_MAX`) para pular uma instância que já
atingiu esse percentual e tentar sua `reserva`.

## Consultar cotas

`openheinerss limites` lê somente os eventos `rate_limits` recentes em cada
`CODEX_HOME` e os arquivos `~/.config/crom-painel/statusline-<instância>.json`
do Claude. `--json` entrega o mesmo resultado para a Central, incluindo
percentual, horário de reinício e idade do dado. Instâncias são declaradas em
`.openheinerss/harnesses` com `base: codex`/`base: claude-code` e seus envs;
nenhuma conta é embutida no programa.
