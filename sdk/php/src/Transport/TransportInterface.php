<?php

namespace Openheinerss\Transport;

/** Contrato comum de StdioTransport e WebSocketTransport (JSON-RPC 2.0, uma mensagem por vez). */
interface TransportInterface
{
    public function send(array $payload): void;

    public function onMessage(?callable $callback): void;

    /** Lê uma mensagem sem bloquear além de $timeout segundos (null espera); null se nada chegou ou a conexão caiu. */
    public function pump(?float $timeout = null): ?array;

    /** true quando o servidor fechou a conexão. */
    public function encerrado(): bool;

    public function readLine(): ?string;

    public function close(): void;
}
