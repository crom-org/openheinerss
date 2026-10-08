# Openheinerss PHP SDK 1.1.0

SDK oficial em **PHP** para automações e backends consumirem o orquestrador **Openheinerss** (crom-org).

## Instalação

```bash
composer require crom-org/openheinerss-sdk
```

## Exemplo de Uso

```php
<?php

require_once __DIR__ . '/vendor/autoload.php';

use Openheinerss\Agent;

$agent = Agent::session([
    'harness'  => 'mock',
]);

$agent->registerHarness(['name' => 'meu-harness', 'base' => 'mock']);
$agent->subscribeEvents([], ['orq.fim' => fn (array $evento) => print_r($evento)]);
$execucao = $agent->run(['nome' => 'teste', 'motor' => 'mock', 'texto' => 'responda OK', 'cwd' => getcwd()]);
var_dump($agent->listHarnesses(), $agent->getLimits(), $execucao);

// Resposta com streaming no terminal:
$response = $agent->prompt("Gere um arquivo helper.php", function ($event, $params) {
    if ($event === 'agent.text') {
        echo $params['delta'];
    }
});

echo "\nResultado final recebido!";
```

## Repasse ao harness (ponte)
`harnessArgs` vai intacto e na ordem ao processo do harness, sem lista de permitidos; texto que começa com `/` vai literalmente (ver `docs/02-protocol-spec.md`). `agent.raw` chega ao callback de `prompt()` e a `on('agent.raw', fn)`.
```php
$agent = Agent::session(['harness' => 'codex', 'effort' => 'high', 'harnessArgs' => ['--add-dir', '../x']]);
$agent->on('agent.raw', fn(array $p) => print("[raw {$p['stream']}] {$p['line']}\n"));
$agent->prompt('/compact');
// run: $agent->run(['nome' => 'a', 'motor' => 'codex', 'harnessArgs' => ['--x']]);
```

## Modo WebSocket

Além do STDIO (padrão), o SDK fala com um `openheinerss serve --porta N` já rodando, sem dependências do composer:

```php
$agent = Agent::session(['harness' => 'mock', 'transport' => 'websocket', 'host' => '127.0.0.1', 'port' => 4820]); // port padrão: OPENHEINERSS_PORTA ou 4820
$agent = Agent::session(['harness' => 'mock', 'url' => 'ws://127.0.0.1:4820/ws', 'origin' => 'http://localhost:3000']);
```

A API é a mesma nos dois modos. O servidor só aceita `Origin` local ou as de `OPENHEINERSS_ORIGENS`; origem recusada lança `RuntimeException` (HTTP 403). Se a conexão cair, a chamada em andamento lança `RuntimeException`.
