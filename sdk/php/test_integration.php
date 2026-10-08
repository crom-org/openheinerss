<?php
require __DIR__ . '/src/Transport/StdioTransport.php';
require __DIR__ . '/src/Types.php';
require __DIR__ . '/src/Agent.php';
$agent = Openheinerss\Agent::session(['harness' => 'mock'], getenv('OPENHEINERSS_BIN') ?: 'openheinerss');
$agent->registerHarness(['name' => 'php-test-harness', 'base' => 'mock']);
$names = array_map(fn($h) => $h['id'], $agent->listHarnesses());
if (!in_array('php-test-harness', $names, true)) throw new RuntimeException('harness não registrado');
$eventos = [];
$decisao = null;
$agent->subscribeEvents(['projeto' => 'teste-php'], [
    'orq.inicio' => function (array $event) use (&$eventos): void { $eventos[] = 'orq.inicio'; },
    'orq.progresso' => function (array $event) use (&$eventos): void { $eventos[] = 'orq.progresso'; },
    'orq.fim' => function (array $event) use (&$eventos): void { $eventos[] = 'orq.fim'; },
    'orq.precisa_decisao' => function (array $event) use (&$decisao): void { $decisao = $event; },
]);
if (!isset($agent->getLimits()['instancias'])) throw new RuntimeException('limites ausentes');
$run = $agent->run(['nome' => 'teste-php', 'motor' => 'mock', 'texto' => 'responda OK', 'cwd' => getcwd(), 'projeto' => 'teste-php']);
if (!str_starts_with($run['id'], 'rodar-')) throw new RuntimeException('rodar não iniciado');
$fim = microtime(true) + 1.0;
while ($decisao === null && microtime(true) < $fim) $agent->listen(0.05);
if ($decisao === null) throw new RuntimeException('decisão não recebida');
$agent->decideRun($decisao['id'], 'permitir');
$agent->listen(1.5);
if (count($eventos) < 3) throw new RuntimeException('eventos contínuos ausentes');
$pensamentos = [];
$agent->prompt('responda OK', function (string $metodo, array $params) use (&$pensamentos): void {
    if ($metodo === 'agent.thinking') $pensamentos[] = $params['delta'] ?? '';
});
if (!$pensamentos) throw new RuntimeException('agent.thinking não recebido');
if (!isset($agent->listRuns(['projeto' => 'teste-php'])['agentes'])) throw new RuntimeException('lista ausente');
echo "PHP SDK integração OK\n";

// O caminho do executável pode conter espaços; proc_open recebe argv, não shell.
$bin = getenv('OPENHEINERSS_BIN') ?: 'openheinerss';
$binReal = realpath($bin);
if ($binReal === false) {
    $binReal = trim((string) shell_exec('command -v ' . escapeshellarg($bin)));
}
if ($binReal === '') throw new RuntimeException('binário do teste não encontrado');
$pastaEspaco = sys_get_temp_dir() . '/openheinerss teste espaço ' . getmypid();
if (!mkdir($pastaEspaco) && !is_dir($pastaEspaco)) throw new RuntimeException('falha ao criar pasta de teste');
$binEspaco = $pastaEspaco . '/openheinerss';
if (!symlink($binReal, $binEspaco)) throw new RuntimeException('falha ao criar caminho com espaço');
try {
    $comCaminhoEspaco = Openheinerss\Agent::session(['harness' => 'mock'], $binEspaco);
    if (!isset($comCaminhoEspaco->getLimits()['instancias'])) throw new RuntimeException('caminho com espaço falhou');
} finally {
    unset($comCaminhoEspaco);
    unlink($binEspaco);
    rmdir($pastaEspaco);
}
