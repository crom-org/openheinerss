<?php
namespace Openheinerss;

/** Opções aceitas por rodar.iniciar. */
final class RunOptions
{
    public function __construct(public string $nome, public string $motor, public string $prompt = '', public array $extra = []) {}
    public function toArray(): array { return array_merge(['nome' => $this->nome, 'motor' => $this->motor, 'prompt' => $this->prompt], $this->extra); }
}

/** Filtro usado por eventos.assinar e rodar.listar. */
final class EventFilter
{
    public function __construct(public ?string $projeto = null, public ?string $agente = null, public ?string $cwd = null, public ?string $pasta = null) {}
    public function toArray(): array { return array_filter(get_object_vars($this)); }
}

/** Evento orq.* normalizado recebido pelo callback. */
final class OrchestrationEvent
{
    public function __construct(public string $method, public array $params) {}
}
