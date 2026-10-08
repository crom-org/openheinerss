<?php

namespace Openheinerss\Transport;

class StdioTransport
{
    private $process;
    private $pipes = [];
    /** @var callable|null */
    private $onMessage;

    public function __construct(string $binPath = "openheinerss")
    {
        $descriptors = [
            0 => ["pipe", "r"], // stdin
            1 => ["pipe", "w"], // stdout
            2 => STDERR
        ];

        $this->process = proc_open("{$binPath} serve --stdio", $descriptors, $this->pipes);

        if (!is_resource($this->process)) {
            throw new \RuntimeException("Falha ao iniciar o processo openheinerss.");
        }
    }

    public function send(array $payload): void
    {
        $line = json_encode($payload) . "\n";
        fwrite($this->pipes[0], $line);
        fflush($this->pipes[0]);
    }

    public function onMessage(?callable $callback): void
    {
        $this->onMessage = $callback;
    }

    /** Lê uma mensagem usando stream_select, sem bloquear o restante do processo. */
    public function pump(?float $timeout = null): ?array
    {
        $read = [$this->pipes[1]];
        $write = null;
        $except = null;
        $seconds = $timeout === null ? null : (int) floor($timeout);
        $microseconds = $timeout === null ? 0 : (int) (($timeout - $seconds) * 1000000);
        $ready = stream_select($read, $write, $except, $seconds, $microseconds);
        if ($ready === false || $ready === 0) return null;
        $line = fgets($this->pipes[1]);
        if ($line === false) return null;
        $message = json_decode(trim($line), true);
        if (!is_array($message)) return null;
        if (isset($message['method']) && $this->onMessage) ($this->onMessage)($message);
        return $message;
    }

    public function readLine(): ?string
    {
        $message = $this->pump(null);
        return $message === null ? null : json_encode($message);
    }

    public function close(): void
    {
        if (is_resource($this->process)) {
            fclose($this->pipes[0]);
            fclose($this->pipes[1]);
            proc_close($this->process);
            $this->process = null;
        }
    }

    public function __destruct()
    {
        $this->close();
    }
}
