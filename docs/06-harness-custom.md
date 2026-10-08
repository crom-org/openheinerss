# Harness custom

Harnesses custom são carregados no início do comando a partir de `.openheinerss/harnesses/*.(yaml|yml|json)` e de `~/.config/openheinerss/harnesses`. O diretório do projeto vence o do usuário.

Formato mínimo:

```yaml
name: meu-agente
command: ./meu-agente
prompt: stdin # ou argument; {{prompt}} também pode aparecer em args
args: []
env:
  PROVEDOR_URL: https://exemplo.invalid
model: meu-modelo
finishRegex: '"type"\s*:\s*"end"'
quotaRegex: 'quota|rate limit'
```

O comando escreve uma linha JSON por evento. O protocolo aceita `text`, `tool`/`tool_call`, `error`, `usage` e `end`/`complete`; linhas não JSON viram texto. Os eventos são normalizados para `agent.text`, `agent.tool_call`, `agent.error`, `agent.usage` e `agent.complete`.

Herança usa `base: claude-code` (ou outro harness custom já carregado) e faz merge de `env`; `command`, `args`, `model`, `prompt` e regexes podem ser substituídos. Veja `examples/cco-openrouter.yaml` e `examples/harness-ndjson.sh`.

`openheinerss harness list`, `harness add <arquivo>` e `harness test <nome> --prompt 'responda OK'` gerenciam e verificam os harnesses. Pelo protocolo e SDKs, `harness.register`/`registerHarness({...})` registra um spec em tempo de execução sem gravar arquivo.
