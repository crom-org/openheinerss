<?php

namespace Openheinerss;

use Openheinerss\Transport\StdioTransport;
use Openheinerss\Transport\TransportInterface;
use Openheinerss\Transport\WebSocketTransport;

class Agent
{
    private TransportInterface $transport;
    private string $sessionId;
    private int $reqId = 1;
    public ?string $generation = null;
    /** Identidade efetiva da sessão criada. */
    public ?array $identidade = null;
    /** Teto de segurança (s) sem nenhuma mensagem durante prompt(); null desliga. */
    public ?float $promptTimeout = 3600.0;
    /** @var array<string, callable> */
    private array $callbacks = [];

    public function __construct(array $options = [], string $binPath = "openheinerss")
    {
        // Padrão STDIO; transport => 'websocket' (ou url) conecta a um `serve --porta N` já rodando.
        $modo = $options['transport'] ?? (isset($options['url']) ? 'websocket' : 'stdio');
        $this->transport = match ($modo) {
            'stdio' => new StdioTransport($binPath),
            'websocket' => new WebSocketTransport($options['url'] ?? null, $options['host'] ?? '127.0.0.1', isset($options['port']) ? (int) $options['port'] : null, $options['origin'] ?? null),
            default => throw new \InvalidArgumentException("transport inválido: {$modo} (use 'stdio' ou 'websocket')"),
        };
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
                $this->identidade = $msg['result']['identidade'] ?? null;
                return;
            }
        }
        throw new \RuntimeException("Servidor encerrou a conexão durante session.create");
    }

    /** effort, harnessArgs (intactos e na ordem), semMcp, mcp e classificarRisco vão em params.options. */
    private function sessionExtras(array $options): array
    {
        $extra = [];
        if (!empty($options['effort'])) $extra['effort'] = $options['effort'];
        if (!empty($options['harnessArgs'])) $extra['harnessArgs'] = array_map('strval', array_values($options['harnessArgs']));
        // semMcp/mcp: quais servidores de mcp.json o harness recebe; classificarRisco: risco opcional (só informa).
        if (!empty($options['semMcp'])) $extra['semMcp'] = true;
        if (!empty($options['mcp'])) $extra['mcp'] = array_map('strval', array_values($options['mcp']));
        if (isset($options['classificarRisco'])) $extra['classificarRisco'] = (bool) $options['classificarRisco'];
        return $extra ? ['options' => $extra] : [];
    }

    /** Registra um callback para uma notificação, ex.: on('agent.raw', fn(array $p) => ...). */
    public function on(string $method, callable $callback): void
    {
        $this->callbacks[$method] = $callback;
    }

    /**
     * Envia o prompt e lê eventos até agent.complete. Erro na resposta de session.prompt
     * (ex.: /comando sem equivalente) vira RuntimeException, como no SDK TypeScript; sem nenhuma
     * mensagem por $timeout segundos (padrão $promptTimeout; null/0 desliga) lança RuntimeException
     * em vez de esperar para sempre.
     */
    public function prompt(string $text, ?callable $onEvent = null, ?float $timeout = null): string
    {
        $id = $this->reqId++;
        $limite = $timeout ?? $this->promptTimeout;
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
        $ultimo = microtime(true);

        while (true) {
            $msg = $this->transport->pump(0.1);
            if ($msg === null) {
                if ($this->transport->encerrado()) {
                    throw new \RuntimeException("Servidor encerrou a conexão durante session.prompt");
                }
                if ($limite && microtime(true) - $ultimo > $limite) {
                    throw new \RuntimeException("session.prompt sem eventos há {$limite}s");
                }
                continue;
            }
            $ultimo = microtime(true);

            if (($msg['id'] ?? null) === $id) {
                if (isset($msg['error'])) {
                    throw new \RuntimeException($msg['error']['message'] ?? 'session.prompt falhou');
                }
                continue;
            }

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
    /** Comandos nativos (/compact, /model…) do harness ou instância, com anotações do usuário. */
    public function listCommands(string $harness, ?string $cwd = null): array { return $this->request('harness.comandos', array_filter(['harness' => $harness, 'cwd' => $cwd])); }
    /** Grava uma anotação livre para o comando (comandos.yaml). */
    public function annotateCommand(string $harness, string $comando, string $anotacao): array { return $this->request('harness.comandos.anotar', ['harness' => $harness, 'comando' => $comando, 'anotacao' => $anotacao]); }
    /** Marca o primeiro uso do comando como já confirmado. */
    public function confirmCommand(string $harness, string $comando): array { return $this->request('harness.comandos.confirmar', ['harness' => $harness, 'comando' => $comando]); }
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
    public function identidade(string $instancia): array { return $this->request('instancia.identidade', ['instancia' => $instancia]); }
    public function listarContas(?string $cwd = null): array { return $this->request('contas.listar', array_filter(['cwd' => $cwd])); }
    public function adicionarConta(string $harness, string $nome, ?string $cwd = null, bool $iniciarLogin = false): array { return $this->request('contas.adicionar', array_filter(['harness' => $harness, 'nome' => $nome, 'cwd' => $cwd, 'iniciarLogin' => $iniciarLogin])); }
    public function renomearConta(string $antigo, string $novo, ?string $cwd = null): array { return $this->request('contas.renomear', array_filter(['antigo' => $antigo, 'novo' => $novo, 'cwd' => $cwd])); }
    public function removerConta(string $nome, ?string $cwd = null, bool $apagarPasta = false): array { return $this->request('contas.remover', array_filter(['nome' => $nome, 'cwd' => $cwd, 'apagarPasta' => $apagarPasta, 'confirmar' => true])); }
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
