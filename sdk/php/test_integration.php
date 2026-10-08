<?php
require __DIR__ . '/src/Transport/StdioTransport.php';
require __DIR__ . '/src/Types.php';
require __DIR__ . '/src/Agent.php';
$agent = Openheinerss\Agent::session(['harness' => 'mock'], getenv('OPENHEINERSS_BIN') ?: 'openheinerss');
$agent->registerHarness(['name' => 'php-test-harness', 'base' => 'mock']);
$names = array_map(fn($h) => $h['id'], $agent->listHarnesses());
if (!in_array('php-test-harness', $names, true)) throw new RuntimeException('harness não registrado');
$agent->subscribeEvents(['projeto' => 'teste-php'], ['orq.fim' => fn(array $event) => null]);
if (!isset($agent->getLimits()['instancias'])) throw new RuntimeException('limites ausentes');
$run = $agent->run(['nome' => 'teste-php', 'motor' => 'mock', 'prompt' => 'responda OK', 'cwd' => getcwd(), 'projeto' => 'teste-php']);
if (!str_starts_with($run['id'], 'rodar-')) throw new RuntimeException('rodar não iniciado');
$agent->stopRun($run['id']);
if (!isset($agent->listRuns(['projeto' => 'teste-php'])['agentes'])) throw new RuntimeException('lista ausente');
echo "PHP SDK integração OK\n";
