# Como desenvolvemos com o próprio Openheinerss

O repositório usa o Openheinerss para executar missões de desenvolvimento. Cada missão tem um prompt em `.claude/agentes/prompts/<nome>.md`; as regras comuns ficam em `prompts/_regras.md`. O comando é executado a partir da raiz do projeto:

```bash
bin/openheinerss rodar <nome> <instância>
```

Por exemplo, uma missão local com o harness falso é `bin/openheinerss rodar teste mock`. Instâncias reais são declaradas em `.openheinerss/harnesses/*.yaml`; elas podem herdar `claude-code`, `codex` ou outro harness, ajustar modelo e ambiente, e declarar `reserva` para trocar de motor quando a cota acabar.

## Acompanhar agentes

As execuções gravam o log ao vivo em `.claude/agentes/logs/<nome>.log` e os metadados em `.claude/agentes/logs/<nome>.meta.json`. Para listar, consultar ou parar uma execução:

```bash
bin/openheinerss agentes
bin/openheinerss agentes --json
bin/openheinerss agentes ver <nome>
bin/openheinerss agentes parar <nome>
```

Os aliases em inglês são `agents`, `list`, `show` e `stop`. A lista informa nome, projeto, motor/modelo, tentativa, estado, início, duração e última linha útil. Um agente com PID vivo, mas sem escrita no log por mais de 15 minutos, aparece como `parado`. Parar registra `FIM ... código 130` e sinaliza somente o PID/grupo daquele agente.

## Limites e carga

O comando `rodar` espera antes de criar a execução quando há limite de agentes ou de carga:

```bash
bin/openheinerss rodar <nome> <instância> --quando-carga-abaixo 10 --max-agentes 2
```

O alias é `--when-load-below`. A carga é comparada como número, não como texto. `--carga-maxima`/`--max-load` continuam aceitos. `--cota-max 95` evita instâncias já acima do percentual informado.

Sem `reserva`, uma falta de cota encerra a instância imediatamente, sem consumir as tentativas restantes: o log termina com código 2 e conserva a indicação de retorno quando o motor a informa, como `resets 9am`. Com `reserva`, as instâncias declaradas são tentadas na ordem.

## Relatórios e regras

O agente trabalha na worktree `.claude/agentes/<nome>` e deve deixar `RELATORIO-AGENTE.md` curto com o que mudou, decisões e números medidos nos testes. O relatório local é ignorado pelo Git. Não fazemos push nem publicação; não imprimimos tokens ou segredos; não usamos `pkill -f`; e builds/testes pesados são serializados com:

```bash
flock /tmp/crom-pesado.lock go test -race ./...
```

Antes do commit, a verificação obrigatória é:

```bash
go build ./... && go vet ./... && go test ./...
go run ./cmd/openheinerss docs
```

O segundo comando atualiza `docs/09-cli.md` com o help real do CLI. Depois conferimos o diff, fazemos commit em português na branch atual e não fazemos push.
