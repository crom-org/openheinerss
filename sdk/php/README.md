# Openheinerss PHP SDK

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
    'harness'  => 'claude-code',
    'provider' => 'openrouter',
    'model'    => 'anthropic/claude-3.7-sonnet'
]);

// Resposta com streaming no terminal:
$response = $agent->prompt("Gere um arquivo helper.php", function ($event, $params) {
    if ($event === 'agent.text') {
        echo $params['delta'];
    }
});

echo "\nResultado final recebido!";
```
