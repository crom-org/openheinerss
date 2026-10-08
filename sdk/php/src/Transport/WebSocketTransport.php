<?php

namespace Openheinerss\Transport;

/**
 * JSON-RPC 2.0 por WebSocket (RFC 6455) com `openheinerss serve --porta N`, sem dependências:
 * stream_socket_client, handshake com Sec-WebSocket-Accept conferido, frames de texto mascarados,
 * fragmentação, ping→pong e close. Mesma interface do StdioTransport.
 */
class WebSocketTransport implements TransportInterface
{
    private const GUID = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11';
    public const PORTA_PADRAO = 4820;

    /** @var resource|null */
    private $socket;
    private string $buf = '';
    private bool $fechado = false;
    /** Fragmentos de uma mensagem em andamento (null = nenhuma). */
    private ?string $parcial = null;
    /** @var callable|null */
    private $onMessage;

    /** Porta do serve: OPENHEINERSS_PORTA ou 4820, como no CLI. */
    public static function portaPadrao(): int
    {
        $porta = (int) (getenv('OPENHEINERSS_PORTA') ?: 0);
        return $porta > 0 && $porta < 65536 ? $porta : self::PORTA_PADRAO;
    }

    public function __construct(?string $url = null, string $host = '127.0.0.1', ?int $port = null, ?string $origin = null, float $timeout = 10.0)
    {
        $url ??= 'ws://' . $host . ':' . ($port ?: self::portaPadrao()) . '/ws';
        $p = parse_url($url);
        if (!$p || !in_array($p['scheme'] ?? '', ['ws', 'wss'], true) || empty($p['host'])) {
            throw new \RuntimeException("URL WebSocket inválida: {$url}");
        }
        $tls = $p['scheme'] === 'wss';
        $porta = $p['port'] ?? ($tls ? 443 : 80);
        $alvo = ($tls ? 'tls://' : 'tcp://') . $p['host'] . ':' . $porta;
        $socket = @stream_socket_client($alvo, $errno, $errstr, $timeout);
        if (!$socket) {
            throw new \RuntimeException("Não foi possível conectar em {$url}: {$errstr} ({$errno})");
        }
        $this->socket = $socket;
        stream_set_read_buffer($socket, 0);
        stream_set_timeout($socket, (int) ceil($timeout));
        try {
            $this->handshake($p, $origin);
        } catch (\Throwable $e) {
            fclose($socket);
            $this->socket = null;
            throw $e;
        }
        stream_set_timeout($socket, 86400);
    }

    private function handshake(array $p, ?string $origin): void
    {
        $chave = base64_encode(random_bytes(16));
        $caminho = ($p['path'] ?? '') === '' ? '/' : $p['path'];
        if (isset($p['query'])) $caminho .= '?' . $p['query'];
        $host = $p['host'] . (isset($p['port']) ? ':' . $p['port'] : '');
        $req = "GET {$caminho} HTTP/1.1\r\nHost: {$host}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
            . "Sec-WebSocket-Key: {$chave}\r\nSec-WebSocket-Version: 13\r\n"
            . ($origin ? "Origin: {$origin}\r\n" : '') . "\r\n";
        $this->escrever($req);
        while (!str_contains($this->buf, "\r\n\r\n")) {
            $pedaco = fread($this->socket, 4096);
            if ($pedaco === false || $pedaco === '') {
                throw new \RuntimeException('Servidor fechou a conexão durante o handshake WebSocket');
            }
            $this->buf .= $pedaco;
            if (strlen($this->buf) > 65536) throw new \RuntimeException('Resposta de handshake WebSocket grande demais');
        }
        [$cabeca, $this->buf] = explode("\r\n\r\n", $this->buf, 2);
        $linhas = explode("\r\n", $cabeca);
        $status = explode(' ', $linhas[0], 3)[1] ?? '?';
        if ($status !== '101') {
            if ($status === '403') {
                throw new \RuntimeException('Servidor recusou o WebSocket (HTTP 403): origem ' . ($origin ?: '(nenhuma)')
                    . ' não permitida; libere-a em OPENHEINERSS_ORIGENS no serve');
            }
            throw new \RuntimeException("Handshake WebSocket recusado (HTTP {$status})");
        }
        $campos = [];
        foreach (array_slice($linhas, 1) as $linha) {
            [$nome, $valor] = array_pad(explode(':', $linha, 2), 2, '');
            $campos[strtolower(trim($nome))] = trim($valor);
        }
        if (($campos['sec-websocket-accept'] ?? '') !== base64_encode(sha1($chave . self::GUID, true))) {
            throw new \RuntimeException('Sec-WebSocket-Accept inválido no handshake');
        }
    }

    private function escrever(string $dados): void
    {
        $total = strlen($dados);
        $enviado = 0;
        while ($enviado < $total) {
            $n = is_resource($this->socket) ? @fwrite($this->socket, substr($dados, $enviado)) : false;
            if ($n === false || $n === 0) {
                $this->fechado = true;
                throw new \RuntimeException('Servidor openheinerss encerrou a conexão (falha ao escrever)');
            }
            $enviado += $n;
        }
    }

    private function frame(int $opcode, string $payload): string
    {
        $n = strlen($payload);
        $cab = chr(0x80 | $opcode);
        if ($n < 126) $cab .= chr(0x80 | $n);
        elseif ($n < 65536) $cab .= chr(0x80 | 126) . pack('n', $n);
        else $cab .= chr(0x80 | 127) . pack('J', $n);
        $mascara = random_bytes(4);
        $corpo = $n ? $payload ^ str_repeat($mascara, intdiv($n, 4) + 1) : '';
        return $cab . $mascara . $corpo;
    }

    public function send(array $payload): void
    {
        if ($this->fechado) throw new \RuntimeException('Transporte do openheinerss está fechado');
        $this->escrever($this->frame(0x1, json_encode($payload, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES)));
    }

    public function onMessage(?callable $callback): void
    {
        $this->onMessage = $callback;
    }

    /** Extrai um frame completo do buffer, ou null se ainda incompleto. @return array{bool,int,string}|null */
    private function proximoFrame(): ?array
    {
        $tam = strlen($this->buf);
        if ($tam < 2) return null;
        $b1 = ord($this->buf[0]);
        $b2 = ord($this->buf[1]);
        $n = $b2 & 0x7F;
        $pos = 2;
        if ($n === 126) {
            if ($tam < 4) return null;
            $n = unpack('n', substr($this->buf, 2, 2))[1];
            $pos = 4;
        } elseif ($n === 127) {
            if ($tam < 10) return null;
            $n = unpack('J', substr($this->buf, 2, 8))[1];
            $pos = 10;
        }
        $mascara = '';
        if ($b2 & 0x80) {
            if ($tam < $pos + 4) return null;
            $mascara = substr($this->buf, $pos, 4);
            $pos += 4;
        }
        if ($tam < $pos + $n) return null;
        $payload = substr($this->buf, $pos, $n);
        $this->buf = substr($this->buf, $pos + $n);
        if ($mascara !== '' && $n) $payload ^= str_repeat($mascara, intdiv($n, 4) + 1);
        return [($b1 & 0x80) !== 0, $b1 & 0x0F, $payload];
    }

    /** Processa frames já bufferizados até completar uma mensagem de texto. */
    private function mensagemPronta(): ?string
    {
        while (($f = $this->proximoFrame()) !== null) {
            [$fin, $op, $payload] = $f;
            if ($op === 0x9) {
                try { $this->escrever($this->frame(0xA, $payload)); } catch (\RuntimeException) {}
            } elseif ($op === 0x8) {
                try { $this->escrever($this->frame(0x8, substr($payload, 0, 2))); } catch (\RuntimeException) {}
                $this->fechado = true;
                return null;
            } elseif ($op === 0x1 || $op === 0x0) {
                if ($op === 0x1) $this->parcial = '';
                elseif ($this->parcial === null) { $this->fechado = true; return null; }
                $this->parcial .= $payload;
                if ($fin) {
                    $texto = $this->parcial;
                    $this->parcial = null;
                    return $texto;
                }
            }
            // 0xA (pong) e binário: ignorados
        }
        return null;
    }

    public function pump(?float $timeout = null): ?array
    {
        $fim = $timeout === null ? null : microtime(true) + $timeout;
        while (true) {
            $texto = $this->mensagemPronta();
            if ($texto !== null) {
                $msg = json_decode($texto, true);
                if (!is_array($msg)) continue;
                if (isset($msg['method']) && $this->onMessage) ($this->onMessage)($msg);
                return $msg;
            }
            if ($this->fechado || !is_resource($this->socket)) return null;
            $restante = $fim === null ? null : max(0.0, $fim - microtime(true));
            $read = [$this->socket];
            $w = $e = null;
            $s = $restante === null ? null : (int) floor($restante);
            $us = $restante === null ? 0 : (int) (($restante - $s) * 1000000);
            $pronto = @stream_select($read, $w, $e, $s, $us);
            if ($pronto === false) { $this->fechado = true; return null; }
            if ($pronto === 0) return null;
            $dados = fread($this->socket, 65536);
            if ($dados === false || $dados === '') {
                $this->fechado = true;
                return null;
            }
            $this->buf .= $dados;
        }
    }

    public function encerrado(): bool
    {
        return $this->fechado || !is_resource($this->socket) || feof($this->socket);
    }

    public function readLine(): ?string
    {
        $message = $this->pump(null);
        return $message === null ? null : json_encode($message);
    }

    public function close(): void
    {
        if (is_resource($this->socket)) {
            if (!$this->fechado) {
                try { $this->escrever($this->frame(0x8, pack('n', 1000))); } catch (\RuntimeException) {}
            }
            fclose($this->socket);
        }
        $this->socket = null;
        $this->fechado = true;
    }

    public function __destruct()
    {
        $this->close();
    }
}
