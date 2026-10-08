# Harness custom

Harnesses custom são carregados no início do comando a partir de `.openheinerss/harnesses/*.(yaml|yml|json)` e de `~/.config/openheinerss/harnesses`. O diretório do projeto vence o do usuário.

### Pasta de configuração explícita (`--config`)

Para não depender da pasta onde o comando roda (ex.: a central chamando `limites --json`), aponte a configuração:

```bash
openheinerss --config /caminho/cfg limites --json      # alias PT: --configuracao
OPENHEINERSS_CONFIG=/caminho/cfg openheinerss serve
```

A pasta contém `harnesses/` e `motores.yaml`; se ela tiver uma subpasta `.openheinerss/`, essa é usada (então dá para apontar a raiz de um projeto). Ordem: **`--config`/`--configuracao` > `OPENHEINERSS_CONFIG` > busca atual** (instâncias na raiz do repositório git, ou na pasta atual fora de repositório; `motores.yaml` em `<cwd>/.openheinerss/`). `~/.config/openheinerss/harnesses` continua carregado antes (a pasta escolhida vence). Vale para todos os comandos (`limites`, `harness list/add`, `rodar`, `run --papel`, `motores`, `serve`, inclusive `session.create` com `papel`). Pasta inexistente é erro. Com a flag, o processo exporta `OPENHEINERSS_CONFIG` para os filhos. `config.yaml`, `mcp.json` e sessões continuam por projeto.

Formato mínimo:

```yaml
name: meu-agente
command: ./meu-agente
prompt: stdin # ou argument; {{prompt}} também pode aparecer em args
args: []
env:
  PROVEDOR_URL: https://exemplo.invalid
model: meu-modelo
modo: sdk # opcional; usado pelo `rodar` para instâncias base: claude-code
finishRegex: '"type"\s*:\s*"end"'
quotaRegex: 'quota|rate limit'
error_regex: 'upstream error|ServiceUnavailableError|429|5xx|temporarily overloaded'
eventosLog: '/caminho/para/eventos.log' # opcional; também há --events-log/--eventos-log
```

`~/` no início de `command` e dos valores de `env` é expandido para a pasta do usuário (o shell não faz isso por nós), com ou sem `base`.

O comando escreve uma linha JSON por evento. O protocolo aceita `text`, `tool`/`tool_call`, `error`, `usage` e `end`/`complete`; linhas não JSON viram texto. Os eventos são normalizados para `agent.text`, `agent.tool_call`, `agent.error`, `agent.usage` e `agent.complete`.

Se o processo sair com erro sem ter emitido o evento de fim, o harness emite um `agent.error` com o código de saída e o fim do stderr, e termina com `reason: "process_error"` (o `rodar` conta como falha e tenta a próxima instância). Se o stderr casar com `quotaRegex`, o erro é reportado como limite de cota. Saída com código 0 sem evento de fim termina com `process_exit` (sucesso). Para cota, use `reserva: [outra-instancia]` e veja o exemplo completo no roteiro [VERIFICACAO.md](VERIFICACAO.md).

Mensagens de provedor podem chegar como erro antes de um fim `completed`. O padrão padrão reconhece sobrecarga, `ServiceUnavailableError`, HTTP 429 e 5xx; `error_regex` (também `erro_regex`) substitui esse padrão na instância. Assim o `rodar` encerra com código 1 e tenta `reserva`, em vez de aceitar `FIM 0` sem resultado útil.

Herança usa `base: claude-code` (ou outro harness custom já carregado) e faz merge de `env`; `command`, `args`, `model`, `prompt` e regexes podem ser substituídos. Veja `examples/cco-openrouter.yaml` e `examples/harness-ndjson.sh`.

`openheinerss harness list`, `harness add <arquivo>` e `harness test <nome> --prompt 'responda OK'` gerenciam e verificam os harnesses. Pelo protocolo e SDKs, `harness.register`/`registerHarness({...})` registra um spec em tempo de execução sem gravar arquivo.
