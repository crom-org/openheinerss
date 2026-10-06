<?php

namespace Openheinerss\Transport;

class StdioTransport
{
    private $process;
    private $pipes = [];

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

    public function readLine(): ?string
    {
        if (feof($this->pipes[1])) {
            return null;
        }
        $line = fgets($this->pipes[1]);
        return $line !== false ? trim($line) : null;
    }

    public function close(): void
    {
        if (is_resource($this->process)) {
            fclose($this->pipes[0]);
            fclose($this->pipes[1]);
            proc_close($this->process);
        }
    }

    public function __destruct()
    {
        $this->close();
    }
}
