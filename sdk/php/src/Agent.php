<?php

namespace Openheinerss;

use Openheinerss\Transport\StdioTransport;

class Agent
{
    private StdioTransport $transport;
    private string $sessionId;
    private int $reqId = 1;

    public function __construct(array $options = [], string $binPath = "openheinerss")
    {
        $this->transport = new StdioTransport($binPath);
        $this->initSession($options);
    }

    public static function session(array $options = []): self
    {
        return new self($options);
    }

    private function initSession(array $options): void
    {
        $id = $this->reqId++;
        $this->transport->send([
            "jsonrpc" => "2.0",
            "id"      => $id,
            "method"  => "session.create",
            "params"  => [
                "harness"  => $options['harness'] ?? 'mock',
                "mode"     => $options['mode'] ?? 'cli',
                "provider" => $options['provider'] ?? null,
                "model"    => $options['model'] ?? null,
                "cwd"      => $options['cwd'] ?? getcwd(),
            ]
        ]);

        while ($line = $this->transport->readLine()) {
            $msg = json_decode($line, true);
            if (isset($msg['id']) && $msg['id'] === $id) {
                if (isset($msg['error'])) {
                    throw new \RuntimeException($msg['error']['message']);
                }
                $this->sessionId = $msg['result']['sessionId'];
                return;
            }
        }
    }

    public function prompt(string $text, ?callable $onEvent = null): string
    {
        $id = $this->reqId++;
        $this->transport->send([
            "jsonrpc" => "2.0",
            "id"      => $id,
            "method"  => "session.prompt",
            "params"  => [
                "sessionId" => $this->sessionId,
                "text"      => $text
            ]
        ]);

        $fullText = "";

        while ($line = $this->transport->readLine()) {
            $msg = json_decode($line, true);
            if (!$msg) continue;

            if (isset($msg['method'])) {
                $method = $msg['method'];
                $params = $msg['params'] ?? [];

                if ($onEvent) {
                    $onEvent($method, $params);
                }

                if ($method === 'agent.text') {
                    $fullText .= $params['delta'] ?? '';
                }

                if ($method === 'agent.permission_request') {
                    // Auto-autoriza por padrão em scripts síncronos se não houver listener
                    $this->respondPermission($params['requestId'], true);
                }

                if ($method === 'agent.complete') {
                    break;
                }
            }
        }

        return $fullText;
    }

    public function respondPermission(string $requestId, bool $allow): void
    {
        $id = $this->reqId++;
        $this->transport->send([
            "jsonrpc" => "2.0",
            "id"      => $id,
            "method"  => "session.permission_respond",
            "params"  => [
                "sessionId" => $this->sessionId,
                "requestId" => $requestId,
                "decision"  => $allow ? "allow" : "deny"
            ]
        ]);
    }
}
