<?php
// Repositório git descartável: o teste nunca cria worktrees no repositório real.
function repoTemporario(): string {
    $pasta = sys_get_temp_dir() . '/openheinerss-sdk-php-' . bin2hex(random_bytes(4));
    mkdir($pasta);
    exec('git init -q -b main ' . escapeshellarg($pasta) . ' && git -C ' . escapeshellarg($pasta) . ' -c user.name=t -c user.email=t@t commit -q --allow-empty -m i', $saida, $codigo);
    if ($codigo !== 0) throw new RuntimeException('git init falhou');
    return $pasta;
}
require __DIR__ . '/src/Transport/TransportInterface.php';
require __DIR__ . '/src/Transport/StdioTransport.php';
require __DIR__ . '/src/Transport/WebSocketTransport.php';
require __DIR__ . '/src/Types.php';
require __DIR__ . '/src/Agent.php';
// test_websocket.php reexecuta este arquivo com TESTE_WS_PORTA (e TESTE_WS_CFG_PORTA/TESTE_WS_CFG): os mesmos
// testes de API contra um `serve` WebSocket real. Sem elas, STDIO. Os servidores falsos abaixo são sempre STDIO.
function agenteReal(array $opcoes = [], bool $comConfig = false): Openheinerss\Agent {
    $porta = getenv($comConfig ? 'TESTE_WS_CFG_PORTA' : 'TESTE_WS_PORTA');
    if ($porta) return Openheinerss\Agent::session($opcoes + ['harness' => 'mock', 'transport' => 'websocket', 'port' => (int) $porta]);
    return Openheinerss\Agent::session($opcoes + ['harness' => 'mock'], getenv('OPENHEINERSS_BIN') ?: 'openheinerss');
}
$agent = agenteReal();
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
$run = $agent->run(['nome' => 'teste-php', 'motor' => 'mock', 'texto' => 'responda OK', 'cwd' => repoTemporario(), 'projeto' => 'teste-php']);
if (!str_starts_with($run['id'], 'rodar-')) throw new RuntimeException('rodar não iniciado');
$fim = microtime(true) + 1.0;
while ($decisao === null && microtime(true) < $fim) $agent->listen(0.05);
if ($decisao === null) throw new RuntimeException('decisão não recebida');
$agent->decideRun($decisao['id'], 'permitir', null, $decisao['run']);
$agent->listen(1.5);
if (count($eventos) < 3) throw new RuntimeException('eventos contínuos ausentes');
$pensamentos = [];
$agent->prompt('responda OK', function (string $metodo, array $params) use (&$pensamentos): void {
    if ($metodo === 'agent.thinking') $pensamentos[] = $params['delta'] ?? '';
});
if (!$pensamentos) throw new RuntimeException('agent.thinking não recebido');
if (!isset($agent->listRuns(['projeto' => 'teste-php'])['agentes'])) throw new RuntimeException('lista ausente');
// Negar com encerrar: orq.fim código 3, motivo "negado", sem nova tentativa.
$fimNegado = null;
$iniciosNegar = 0;
$decisaoNegar = null;
$agent->subscribeEvents(['projeto' => 'teste-php-negar'], [
    'orq.inicio' => function (array $event) use (&$iniciosNegar): void { $iniciosNegar++; },
    'orq.fim' => function (array $event) use (&$fimNegado): void { $fimNegado = $event; },
    'orq.precisa_decisao' => function (array $event) use (&$decisaoNegar): void { $decisaoNegar = $event; },
]);
$agent->run(['nome' => 'teste-php-negar', 'motor' => 'mock', 'texto' => 'responda OK', 'cwd' => repoTemporario(), 'projeto' => 'teste-php-negar', 'tentativas' => 2]);
$fim = microtime(true) + 3.0;
while ($decisaoNegar === null && microtime(true) < $fim) $agent->listen(0.05);
if ($decisaoNegar === null) throw new RuntimeException('decisão (negar) não recebida');
$agent->decideRun($decisaoNegar['id'], 'negar', null, $decisaoNegar['run'], true);
$fim = microtime(true) + 3.0;
while ($fimNegado === null && microtime(true) < $fim) $agent->listen(0.05);
if (($fimNegado['codigo'] ?? null) !== 3 || ($fimNegado['motivo'] ?? null) !== 'negado' || $iniciosNegar !== 1) {
    throw new RuntimeException('negar com encerrar: ' . json_encode([$fimNegado, $iniciosNegar]));
}

// Ponte: harnessArgs sai no JSON-RPC e agent.raw é entregue (servidor falso, sem depender do binário).
$pasta = sys_get_temp_dir() . '/openheinerss-ponte-php-' . bin2hex(random_bytes(4));
mkdir($pasta);
$falso = $pasta . '/falso.php';
$registro = $pasta . '/reqs.jsonl';
file_put_contents($falso, '#!/usr/bin/env php
<?php
while (($l = fgets(STDIN)) !== false) {
    $r = json_decode($l, true);
    file_put_contents(' . var_export($registro, true) . ', $l, FILE_APPEND);
    if ($r["method"] === "session.create") echo json_encode(["jsonrpc" => "2.0", "id" => $r["id"], "result" => ["sessionId" => "s1"]]) . "\n";
    if ($r["method"] === "session.prompt") {
        echo json_encode(["jsonrpc" => "2.0", "method" => "agent.raw", "params" => ["sessionId" => "s1", "harness" => "falso", "stream" => "stderr", "line" => "linha crua"]]) . "\n";
        echo json_encode(["jsonrpc" => "2.0", "method" => "agent.complete", "params" => ["reason" => "completed"]]) . "\n";
    }
}
');
chmod($falso, 0755);
$ponte = Openheinerss\Agent::session(['harness' => 'falso', 'effort' => 'high', 'harnessArgs' => ['--x=a,b', '/compact', 'c d']], $falso);
$cruas = [];
$ponte->on('agent.raw', function (array $p) use (&$cruas): void { $cruas[] = $p['line']; });
$ponte->prompt('/model x');
if ($cruas !== ['linha crua']) throw new RuntimeException('agent.raw não entregue');
$reqs = array_map(fn($l) => json_decode($l, true), file($registro, FILE_IGNORE_NEW_LINES));
$criar = array_values(array_filter($reqs, fn($r) => $r['method'] === 'session.create'))[0];
if ($criar['params']['options']['harnessArgs'] !== ['--x=a,b', '/compact', 'c d'] || $criar['params']['options']['effort'] !== 'high') throw new RuntimeException('harnessArgs fora do JSON-RPC');
$prompt = array_values(array_filter($reqs, fn($r) => $r['method'] === 'session.prompt'))[0];
if ($prompt['params']['text'] !== '/model x') throw new RuntimeException('prompt não chegou literal');
// Auditoria 26: /comando sem equivalente devolve erro RPC; o SDK esperava agent.complete para sempre.
$falsoErro = $pasta . '/falso-erro.php';
file_put_contents($falsoErro, '#!/usr/bin/env php
<?php
while (($l = fgets(STDIN)) !== false) {
    $r = json_decode($l, true);
    if ($r["method"] === "session.create") echo json_encode(["jsonrpc" => "2.0", "id" => $r["id"], "result" => ["sessionId" => "s1"]]) . "\n";
    if ($r["method"] === "session.prompt" && str_starts_with($r["params"]["text"], "/compact")) echo json_encode(["jsonrpc" => "2.0", "id" => $r["id"], "error" => ["code" => -32603, "message" => "codex não aceita /compact"]]) . "\n";
    elseif ($r["method"] === "session.prompt") echo json_encode(["jsonrpc" => "2.0", "id" => $r["id"], "result" => ["accepted" => true]]) . "\n";
}
');
chmod($falsoErro, 0755);
$semEq = Openheinerss\Agent::session(['harness' => 'codex'], $falsoErro);
$inicio = microtime(true);
try { $semEq->prompt('/compact'); throw new LogicException('prompt com erro RPC não lançou'); }
catch (RuntimeException $e) { if (!str_contains($e->getMessage(), 'não aceita /compact')) throw $e; }
if (microtime(true) - $inicio > 2) throw new RuntimeException('erro do prompt demorou');
try { $semEq->prompt('oi', null, 0.3); throw new LogicException('timeout de segurança não disparou'); }
catch (RuntimeException $e) { if (!str_contains($e->getMessage(), 'sem eventos')) throw $e; }
unset($semEq);
try { $codexReal = agenteReal(['harness' => 'codex', 'cwd' => sys_get_temp_dir()]); }
catch (RuntimeException $e) { $codexReal = null; echo "codex indisponível, pulando /compact real: {$e->getMessage()}\n"; }
if ($codexReal) {
    try { $codexReal->prompt('/compact', null, 10); throw new LogicException('/compact no codex não lançou'); }
    catch (RuntimeException $e) { if (!str_contains($e->getMessage(), '/compact')) throw $e; }
    unset($codexReal);
}
// Comandos do harness: anotar e confirmar gravam em <OPENHEINERSS_CONFIG>/comandos.yaml.
$cfgComandos = getenv('TESTE_WS_CFG') ?: sys_get_temp_dir() . '/openheinerss-cmd-php-' . bin2hex(random_bytes(4));
if (!is_dir($cfgComandos)) mkdir($cfgComandos);
$antesCfg = getenv('OPENHEINERSS_CONFIG');
putenv('OPENHEINERSS_CONFIG=' . $cfgComandos);
try {
    $cmdAgent = agenteReal([], true);
    $nota = $cmdAgent->annotateCommand('claude-code', '/compact', 'compacta o claude code; o central não usa');
    if (($nota['anotacao'] ?? '') !== 'compacta o claude code; o central não usa') throw new RuntimeException('anotação não gravada');
    if (!($cmdAgent->confirmCommand('claude-code', '/compact')['confirmado'] ?? false)) throw new RuntimeException('confirmar falhou');
    $lista = $cmdAgent->listCommands('claude-code');
    if ($lista['arquivo'] !== $cfgComandos . '/comandos.yaml') throw new RuntimeException('arquivo de comandos errado: ' . $lista['arquivo']);
    if (($cmdAgent->listCommands('codex')['desconhecido'] ?? '') !== 'sem_equivalente') throw new RuntimeException('codex deveria recusar /x desconhecido');
} finally {
    unset($cmdAgent);
    putenv($antesCfg === false ? 'OPENHEINERSS_CONFIG' : 'OPENHEINERSS_CONFIG=' . $antesCfg);
}
echo "PHP SDK integração OK\n";
if (getenv('TESTE_WS_PORTA')) exit(0); // caminho com espaço só vale para o processo filho (STDIO)

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

// MCP e risco: semMcp, mcp e classificarRisco saem em params.options.
$pastaMcp = sys_get_temp_dir() . '/openheinerss-mcp-php-' . bin2hex(random_bytes(4));
mkdir($pastaMcp);
$regMcp = $pastaMcp . '/reqs.jsonl';
$falsoMcp = $pastaMcp . '/falso.php';
file_put_contents($falsoMcp, '#!/usr/bin/env php
<?php
while (($l = fgets(STDIN)) !== false) {
    $r = json_decode($l, true);
    file_put_contents(' . var_export($regMcp, true) . ', $l, FILE_APPEND);
    if ($r["method"] === "session.create") echo json_encode(["jsonrpc" => "2.0", "id" => $r["id"], "result" => ["sessionId" => "s1"]]) . "\n";
}
');
chmod($falsoMcp, 0755);
$a1 = Openheinerss\Agent::session(['harness' => 'falso', 'mcp' => ['fs'], 'classificarRisco' => true], $falsoMcp);
$a2 = Openheinerss\Agent::session(['harness' => 'falso', 'semMcp' => true], $falsoMcp);
unset($a1, $a2);
$criarMcp = array_map(fn($l) => json_decode($l, true)['params']['options'] ?? null, file($regMcp, FILE_IGNORE_NEW_LINES));
if ($criarMcp[0] !== ['mcp' => ['fs'], 'classificarRisco' => true] || $criarMcp[1] !== ['semMcp' => true]) throw new RuntimeException('opções de MCP/risco fora do JSON-RPC: ' . json_encode($criarMcp));
array_map('unlink', glob($pastaMcp . '/*'));
rmdir($pastaMcp);
echo "MCP e risco no JSON-RPC: ok\n";
