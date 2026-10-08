<?php

namespace Openheinerss;

use Openheinerss\Transport\StdioTransport;

class Agent
{
    private StdioTransport $transport;
    private string $sessionId;
    private int $reqId = 1;
    public ?string $generation = null;
    /** @var array<string, callable> */
    private array $callbacks = [];

    public function __construct(array $options = [], string $binPath = "openheinerss")
    {
        $this->transport = new StdioTransport($binPath);
        $this->transport->onMessage(function (array $message): void {
            $name = $message['method'] ?? null;
            if ($name !== null && isset($this->callbacks[$name])) {
                ($this->callbacks[$name])($message['params'] ?? []);
            }
        });
        $this->initSession($options);
    }

    public static function session(array $options = [], string $binPath = "openheinerss"): self
    {
        return new self($options, $binPath);
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
            ] + $this->sessionExtras($options)
        ]);

        while ($line = $this->transport->readLine()) {
            $msg = json_decode($line, true);
            if (isset($msg['id']) && $msg['id'] === $id) {
                if (isset($msg['error'])) {
                    throw new \RuntimeException($msg['error']['message']);
                }
                $this->generation = $msg['geracao'] ?? null;
                $this->sessionId = $msg['result']['sessionId'];
                return;
            }
        }
    }

    /** effort e harnessArgs (argumentos nativos extras, intactos e na ordem) vão em params.options. */
    private function sessionExtras(array $options): array
    {
        $extra = [];
        if (!empty($options['effort'])) $extra['effort'] = $options['effort'];
        if (!empty($options['harnessArgs'])) $extra['harnessArgs'] = array_map('strval', array_values($options['harnessArgs']));
        return $extra ? ['options' => $extra] : [];
    }

    /** Registra um callback para uma notificação, ex.: on('agent.raw', fn(array $p) => ...). */
    public function on(string $method, callable $callback): void
    {
        $this->callbacks[$method] = $callback;
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

                if ($onEvent) $onEvent($method, $params);

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

    private function request(string $method, array $params): mixed
    {
        $id = $this->reqId++;
        $this->transport->send(["jsonrpc" => "2.0", "id" => $id, "method" => $method, "params" => $params]);
        while ($line = $this->transport->readLine()) {
            $msg = json_decode($line, true);
            if (($msg['id'] ?? null) === $id) {
                if (isset($msg['error'])) throw new \RuntimeException($msg['error']['message']);
                $this->generation = $msg['geracao'] ?? null;
                return $msg['result'] ?? null;
            }
        }
        throw new \RuntimeException("Servidor encerrou a conexão durante {$method}");
    }

    public function registerHarness(array $spec): void { $this->request('harness.register', $spec); }
    public function listHarnesses(): array { return $this->request('harness.listar', [])['harnesses'] ?? []; }
    public function run(array|RunOptions $options): array { return $this->request('rodar.iniciar', $options instanceof RunOptions ? $options->toArray() : $options); }
    public function listRuns(array $filter = []): array { return $this->request('rodar.listar', $filter); }
    public function stopRun(?string $id = null, ?string $agente = null): void {
        $this->request('rodar.parar', array_filter(['id' => $id, 'agente' => $agente]));
    }
    /**
     * $run (id da execução, vem em orq.precisa_decisao) é opcional; se vier, o servidor confere.
     * $encerrar: numa negação, termina a execução (orq.fim código 3, motivo "negado"); null vale serve --negar-encerra.
     */
    public function decideRun(string $id, string $resposta, ?string $mensagem = null, ?string $run = null, ?bool $encerrar = null): void {
        $params = array_filter(['id' => $id, 'resposta' => $resposta, 'mensagem' => $mensagem, 'run' => $run]);
        if ($encerrar !== null) $params['encerrar'] = $encerrar;
        $this->request('rodar.decidir', $params);
    }
    public function getLimits(): array { return $this->request('limites.obter', []); }
    /** @param array<string, callable(array): void> $callbacks */
    public function subscribeEvents(array|EventFilter $filter = [], array $callbacks = []): void {
        $this->callbacks = array_merge($this->callbacks, $callbacks);
        $this->request('eventos.assinar', $filter instanceof EventFilter ? $filter->toArray() : $filter);
    }

    /** Mantém o leitor de eventos ativo. Use em um processo que precisa observar eventos em segundo plano. */
    public function listen(?float $seconds = null): void
    {
        $until = $seconds === null ? null : microtime(true) + $seconds;
        while ($until === null || microtime(true) < $until) {
            $remaining = $until === null ? null : max(0.0, $until - microtime(true));
            $this->transport->pump($remaining === null ? null : min($remaining, 1.0));
        }
    }
}
