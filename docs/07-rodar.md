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
`run`, com os campos `nome`, `motor`, `prompt`, `retomar` e os limites.
