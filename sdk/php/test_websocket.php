<?php
// Os testes de API de test_integration.php contra um `serve` WebSocket real, mais os casos do transporte.
// Usa OPENHEINERSS_BIN ou compila o binário; encerra só os processos que iniciou (PID guardado).
require __DIR__ . '/src/Transport/TransportInterface.php';
require __DIR__ . '/src/Transport/StdioTransport.php';
require __DIR__ . '/src/Transport/WebSocketTransport.php';
require __DIR__ . '/src/Types.php';
require __DIR__ . '/src/Agent.php';

use Openheinerss\Transport\WebSocketTransport;

function falha(string $m): never { throw new RuntimeException($m); }

function portaLivre(): int {
    $s = stream_socket_server('tcp://127.0.0.1:0', $e, $es);
    $porta = (int) substr(strrchr(stream_socket_get_name($s, false), ':'), 1);
    fclose($s);
    return $porta;
}

$bin = getenv('OPENHEINERSS_BIN');
if (!$bin) {
    $dir = sys_get_temp_dir() . '/openheinerss-ws-php-bin-' . bin2hex(random_bytes(4));
    mkdir($dir);
    $bin = $dir . '/openheinerss';
    exec('cd ' . escapeshellarg(dirname(__DIR__, 2)) . ' && go build -o ' . escapeshellarg($bin) . ' ./cmd/openheinerss 2>&1', $saida, $codigo);
    if ($codigo !== 0) falha('go build falhou: ' . implode("\n", $saida));
}

$servidores = [];
/** @return array{int, resource} porta e processo */
function subirServidor(string $bin, array $env = []): array {
    global $servidores;
    for ($i = 0; $i < 5; $i++) {
        $porta = portaLivre();
        $proc = proc_open([$bin, 'serve', '--porta', (string) $porta, '--host', '127.0.0.1'],
            [0 => ['file', '/dev/null', 'r'], 1 => ['file', '/dev/null', 'w'], 2 => ['file', '/dev/null', 'w']], $pipes, null, $env + getenv());
        $servidores[] = $proc;
        $fim = microtime(true) + 10;
        while (microtime(true) < $fim) {
            if (!proc_get_status($proc)['running']) break;
            $c = @stream_socket_client("tcp://127.0.0.1:$porta", $en, $es, 0.2);
            if ($c) { fclose($c); return [$porta, $proc]; }
            usleep(50000);
        }
        pararServidor($proc);
    }
    falha('serve não abriu a porta');
}
function pararServidor($proc): void {
    if (!is_resource($proc)) return;
    $pid = proc_get_status($proc)['pid'];
    if (proc_get_status($proc)['running']) posix_kill($pid, SIGTERM); // só o PID iniciado aqui
    $fim = microtime(true) + 3;
    while (proc_get_status($proc)['running'] && microtime(true) < $fim) usleep(50000);
    if (proc_get_status($proc)['running']) posix_kill($pid, SIGKILL);
    proc_close($proc);
}

try {
    [$porta, $proc] = subirServidor($bin);
    $cfg = sys_get_temp_dir() . '/openheinerss-cfg-ws-php-' . bin2hex(random_bytes(4));
    mkdir($cfg);
    [$portaCfg, $procCfg] = subirServidor($bin, ['OPENHEINERSS_CONFIG' => $cfg]);

    // 1) Todos os testes de API de test_integration.php, por WebSocket.
    $cmd = escapeshellarg(PHP_BINARY) . ' ' . escapeshellarg(__DIR__ . '/test_integration.php') . ' 2>&1';
    $env = ['TESTE_WS_PORTA' => (string) $porta, 'TESTE_WS_CFG_PORTA' => (string) $portaCfg, 'TESTE_WS_CFG' => $cfg, 'OPENHEINERSS_BIN' => $bin];
    foreach ($env as $k => $v) putenv("$k=$v");
    exec($cmd, $saida, $codigo);
    if ($codigo !== 0) falha("test_integration.php por WebSocket falhou:\n" . implode("\n", $saida));
    echo "API por WebSocket OK\n";

    // 2) URL, host/port e OPENHEINERSS_PORTA.
    $a = Openheinerss\Agent::session(['harness' => 'mock', 'url' => "ws://127.0.0.1:$porta/ws", 'origin' => 'http://localhost:3000']);
    if (!isset($a->getLimits()['instancias'])) falha('url=');
    unset($a);
    putenv("OPENHEINERSS_PORTA=$porta");
    $a = Openheinerss\Agent::session(['harness' => 'mock', 'transport' => 'websocket']);
    if (!isset($a->getLimits()['instancias'])) falha('OPENHEINERSS_PORTA');
    unset($a);
    putenv('OPENHEINERSS_PORTA');
    try { Openheinerss\Agent::session(['transport' => 'xyz']); falha('transport inválido aceito'); }
    catch (InvalidArgumentException) {}

    // 3) Origem não permitida é recusada com erro claro; liberada por OPENHEINERSS_ORIGENS.
    try { Openheinerss\Agent::session(['harness' => 'mock', 'transport' => 'websocket', 'port' => $porta, 'origin' => 'http://evil.example']); falha('origem evil aceita'); }
    catch (RuntimeException $e) { if (!str_contains($e->getMessage(), '403') || !str_contains($e->getMessage(), 'OPENHEINERSS_ORIGENS')) throw $e; }
    [$portaOrig, $procOrig] = subirServidor($bin, ['OPENHEINERSS_ORIGENS' => 'http://evil.example']);
    $a = Openheinerss\Agent::session(['harness' => 'mock', 'transport' => 'websocket', 'port' => $portaOrig, 'origin' => 'http://evil.example']);
    if (!isset($a->getLimits()['instancias'])) falha('origem liberada');
    unset($a);
    echo "origem OK\n";

    // 4) Conexão recusada.
    try { new WebSocketTransport(null, '127.0.0.1', portaLivre(), null, 1.0); falha('conexão sem servidor'); }
    catch (RuntimeException $e) { if (!str_contains($e->getMessage(), 'conectar')) throw $e; }

    // 5) Servidor falso (processo php): fragmentos, ping no meio, payload de 70 mil bytes e queda com requisição pendente.
    $falsoPhp = sys_get_temp_dir() . '/openheinerss-ws-falso-' . bin2hex(random_bytes(4)) . '.php';
    file_put_contents($falsoPhp, <<<'PHPF'
<?php
$srv = stream_socket_server('tcp://127.0.0.1:0');
echo substr(strrchr(stream_socket_get_name($srv, false), ':'), 1), "\n";
$modo = $argv[1];
$c = stream_socket_accept($srv, 15);
$req = '';
while (!str_contains($req, "\r\n\r\n")) $req .= fread($c, 4096);
preg_match('/Sec-WebSocket-Key: (\S+)/i', $req, $m);
fwrite($c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " . base64_encode(sha1($m[1] . '258EAFA5-E914-47DA-95CA-C5AB0DC85B11', true)) . "\r\n\r\n");
function ler($c, $n) { $b = ''; while (strlen($b) < $n) { $p = fread($c, $n - strlen($b)); if ($p === '' || $p === false) exit(0); $b .= $p; } return $b; }
function frame($fin, $op, $d) { $n = strlen($d); return chr(($fin ? 0x80 : 0) | $op) . ($n < 126 ? chr($n) : ($n < 65536 ? chr(126) . pack('n', $n) : chr(127) . pack('J', $n))) . $d; }
while (true) {
    $h = ler($c, 2); $op = ord($h[0]) & 15; $n = ord($h[1]) & 127;
    if (!(ord($h[1]) & 128)) exit(1); // cliente deve mascarar
    if ($n == 126) $n = unpack('n', ler($c, 2))[1]; elseif ($n == 127) $n = unpack('J', ler($c, 8))[1];
    $mk = ler($c, 4); $d = $n ? ler($c, $n) ^ str_repeat($mk, intdiv($n, 4) + 1) : '';
    if ($modo === 'cala') { if ($op == 1) { usleep(300000); exit(0); } continue; }
    if ($op == 8) exit(0);
    if ($op == 10) { file_put_contents($argv[2], 'pong'); continue; }
    if ($op == 1) {
        $r = json_decode($d, true);
        $resp = json_encode(['jsonrpc' => '2.0', 'id' => $r['id'], 'result' => $r['params']], JSON_UNESCAPED_UNICODE);
        $meio = intdiv(strlen($resp), 2);
        fwrite($c, frame(false, 1, substr($resp, 0, $meio)) . frame(true, 9, 'ping') . frame(false, 0, substr($resp, $meio, 1)) . frame(true, 0, substr($resp, $meio + 1)));
    }
}
PHPF);
    $iniciarFalso = function (string $modo, string $pong = '') use ($falsoPhp, &$servidores): array {
        $p = proc_open([PHP_BINARY, $falsoPhp, $modo, $pong], [1 => ['pipe', 'w'], 2 => ['file', '/dev/null', 'w']], $pipes);
        $servidores[] = $p;
        return [(int) trim(fgets($pipes[1])), $p];
    };
    $pongArq = sys_get_temp_dir() . '/ws-pong-' . getmypid();
    @unlink($pongArq);
    [$pf, $procF] = $iniciarFalso('eco', $pongArq);
    $t = new WebSocketTransport(null, '127.0.0.1', $pf);
    $t->send(['jsonrpc' => '2.0', 'id' => 5, 'method' => 'eco', 'params' => ['x' => 'ç' . str_repeat('a', 70000)]]);
    $r = $t->pump(3.0);
    if (($r['result']['x'] ?? '') !== 'ç' . str_repeat('a', 70000)) falha('eco fragmentado/grande');
    $t->send(['jsonrpc' => '2.0', 'id' => 6, 'method' => 'eco', 'params' => ['x' => 1]]);
    $t->pump(3.0);
    usleep(100000);
    if (@file_get_contents($pongArq) !== 'pong') falha('ping não respondido com pong');
    $t->close();
    pararServidor($procF);

    [$pf, $procF] = $iniciarFalso('cala');
    // O falso 'cala' recebe session.create e cai: a criação da sessão tem de falhar, não travar.
    try { Openheinerss\Agent::session(['harness' => 'mock', 'url' => "ws://127.0.0.1:$pf/ws"]); falha('queda não falhou o pendente'); }
    catch (RuntimeException $e) { if (!str_contains($e->getMessage(), 'encerrou a conexão')) throw $e; }
    echo "queda com requisição pendente falha OK\n";
} catch (Throwable $e) {
    foreach ($servidores as $s) pararServidor($s);
    throw $e;
}
foreach ($servidores as $s) pararServidor($s);
echo "PHP SDK WebSocket OK\n";
