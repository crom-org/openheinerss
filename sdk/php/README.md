# Openheinerss PHP SDK 0.2.0

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
