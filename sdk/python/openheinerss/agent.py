import json
import os
import queue
import time
from typing import Generator, Dict, Any, Optional, Callable, TypedDict
from .transport import StdioTransport

class RunOptions(TypedDict, total=False):
    nome: str; motor: str; modelo: str; esforco: str; prompt: str; texto: str; retomar: bool
    pasta: str; branchBase: str; cargaMax: float; maxAgentes: int; tentativas: int; cotaMax: float; cwd: str; projeto: str
    harnessArgs: list

class OrchestrationEvent(TypedDict):
    type: str
    data: Dict[str, Any]

class Agent:
    def __init__(
        self,
        harness: str = "mock",
        mode: str = "cli",
        provider: Optional[str] = None,
        model: Optional[str] = None,
        cwd: Optional[str] = None,
        bin_path: str = "openheinerss",
        effort: Optional[str] = None,
        harness_args: Optional[list] = None,
        prompt_timeout: Optional[float] = 3600.0,
    ):
        # Teto de segurança sem nenhum evento durante um prompt (None/0 desliga).
        self.prompt_timeout = prompt_timeout
        self.transport = StdioTransport(bin_path)
        self.req_id = 1
        self.generation: Optional[str] = None
        self.callbacks: Dict[str, Callable[[Dict[str, Any]], None]] = {}
        self.transport.on_event = self._event
        self.session_id = self._create_session(
            harness=harness,
            mode=mode,
            provider=provider,
            model=model,
            cwd=cwd or os.getcwd(),
            effort=effort,
            harness_args=harness_args,
        )

    def _create_session(self, harness: str, mode: str, provider: Optional[str], model: Optional[str], cwd: str, effort: Optional[str] = None, harness_args: Optional[list] = None) -> str:
        req_id = self.req_id
        self.req_id += 1

        params: Dict[str, Any] = {
            "harness": harness,
            "mode": mode,
            "provider": provider,
            "model": model,
            "cwd": cwd
        }
        options: Dict[str, Any] = {}
        if effort:
            options["effort"] = effort
        if harness_args:
            # Intactos e na ordem, sem filtro: a ponte não esconde nada do harness.
            options["harnessArgs"] = [str(a) for a in harness_args]
        if options:
            params["options"] = options

        response = self.transport.request({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": "session.create",
            "params": params
        })

        if "error" in response:
            raise RuntimeError(response["error"]["message"])
        self.generation = response.get("geracao")
        return response["result"]["sessionId"]

    def stream(self, text: str, timeout: Optional[float] = None) -> Generator[Dict[str, Any], None, None]:
        """Eventos do prompt até agent.complete. Erro na resposta de session.prompt (ex.: /comando
        sem equivalente) vira RuntimeError, como no SDK TypeScript; sem nenhum evento por
        `timeout` segundos (padrão self.prompt_timeout) levanta TimeoutError em vez de esperar para sempre."""
        req_id = self.req_id
        self.req_id += 1
        limite = self.prompt_timeout if timeout is None else timeout

        resposta = self.transport.start_request({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": "session.prompt",
            "params": {
                "sessionId": self.session_id,
                "text": text
            }
        })
        respondeu = False
        ultimo = time.monotonic()
        try:
            while True:
                if not respondeu:
                    try:
                        r = resposta.get_nowait()
                    except queue.Empty:
                        r = None
                    if r is not None:
                        respondeu = True
                        if isinstance(r, BaseException):
                            raise r
                        if r.get("error"):
                            raise RuntimeError(r["error"].get("message", "session.prompt falhou"))
                msg = self.transport.next_event(timeout=0.05)
                if not msg:
                    if self.transport._closed and self.transport._events.empty():
                        raise self.transport._eof_error or RuntimeError("transporte fechado")
                    if limite and time.monotonic() - ultimo > limite:
                        raise TimeoutError(f"session.prompt sem eventos há {limite:g}s")
                    continue
                ultimo = time.monotonic()
                if "method" in msg:
                    method = msg["method"]
                    params = msg.get("params", {})

                    yield {"type": method, "data": params}

                    if method == "agent.permission_request":
                        # Auto-autoriza em modo stream simples
                        self.respond_permission(params.get("requestId"), True)

                    if method == "agent.complete":
                        break
        finally:
            self.transport.end_request(req_id)

    def prompt(self, text: str, timeout: Optional[float] = None) -> str:
        output = []
        for event in self.stream(text, timeout):
            if event["type"] == "agent.text":
                output.append(event["data"].get("delta", ""))
        return "".join(output)

    def register_harness(self, spec: Dict[str, Any]) -> None:
        """Registra um harness custom em tempo de execução."""
        req_id = self.req_id
        self.req_id += 1
        response = self.transport.request({"jsonrpc": "2.0", "id": req_id, "method": "harness.register", "params": spec})
        if "error" in response:
            raise RuntimeError(response["error"]["message"])

    # Alias alinhado ao nome usado pelos SDKs TypeScript e pelo protocolo.
    def registerHarness(self, spec: Dict[str, Any]) -> None:
        self.register_harness(spec)

    def on(self, method: str, callback: Callable[[Dict[str, Any]], None]) -> None:
        """Registra um callback para uma notificação, ex.: on("agent.raw", fn) recebe sessionId, harness, stream e line."""
        self.callbacks[method] = callback

    def _event(self, msg: Dict[str, Any]) -> None:
        callback = self.callbacks.get(msg.get("method", ""))
        if callback:
            callback(msg.get("params", {}))

    def _request(self, method: str, params: Dict[str, Any]) -> Any:
        req_id = self.req_id
        self.req_id += 1
        msg = self.transport.request({"jsonrpc": "2.0", "id": req_id, "method": method, "params": params})
        if "error" in msg:
            raise RuntimeError(msg["error"]["message"])
        if msg.get("geracao"):
            self.generation = msg["geracao"]
        return msg.get("result")

    def list_harnesses(self) -> list[dict[str, Any]]:
        return self._request("harness.listar", {}).get("harnesses", [])

    def list_commands(self, harness: str, cwd: Optional[str] = None) -> Dict[str, Any]:
        """Comandos nativos (/compact, /model…) do harness ou instância, com anotações do usuário."""
        return self._request("harness.comandos", {k: v for k, v in {"harness": harness, "cwd": cwd}.items() if v})

    def annotate_command(self, harness: str, comando: str, anotacao: str) -> Dict[str, Any]:
        """Grava uma anotação livre para o comando (comandos.yaml)."""
        return self._request("harness.comandos.anotar", {"harness": harness, "comando": comando, "anotacao": anotacao})

    def confirm_command(self, harness: str, comando: str) -> Dict[str, Any]:
        """Marca o primeiro uso do comando como já confirmado."""
        return self._request("harness.comandos.confirmar", {"harness": harness, "comando": comando})

    def run(self, options: Dict[str, Any]) -> Dict[str, Any]:
        return self._request("rodar.iniciar", options)

    def list_runs(self, **filter: str) -> Dict[str, Any]:
        return self._request("rodar.listar", filter)

    def stop_run(self, id: Optional[str] = None, agente: Optional[str] = None) -> None:
        self._request("rodar.parar", {k: v for k, v in {"id": id, "agente": agente}.items() if v})

    def decide_run(self, id: str, resposta: str, mensagem: Optional[str] = None, run: Optional[str] = None, encerrar: Optional[bool] = None) -> None:
        # run (id da execução, vem em orq.precisa_decisao) é opcional; se vier, o servidor confere.
        # encerrar: numa negação, termina a execução (orq.fim código 3, motivo "negado"); None vale serve --negar-encerra.
        params = {"id": id, "resposta": resposta, "mensagem": mensagem}
        if run:
            params["run"] = run
        if encerrar is not None:
            params["encerrar"] = encerrar
        self._request("rodar.decidir", params)

    def get_limits(self) -> Dict[str, Any]:
        return self._request("limites.obter", {})

    def subscribe_events(self, callback: Optional[Callable[[Dict[str, Any]], None]] = None, **filter: str) -> None:
        if callback:
            for name in ("orq.inicio", "orq.progresso", "orq.fim", "orq.erro", "orq.precisa_decisao"):
                self.callbacks[name] = callback
        self._request("eventos.assinar", filter)

    listHarnesses = list_harnesses
    listCommands = list_commands
    annotateCommand = annotate_command
    confirmCommand = confirm_command
    listRuns = list_runs
    stopRun = stop_run
    decideRun = decide_run
    getLimits = get_limits
    subscribeEvents = subscribe_events

    def respond_permission(self, request_id: str, allow: bool):
        req_id = self.req_id
        self.req_id += 1

        self.transport.send({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": "session.permission_respond",
            "params": {
                "sessionId": self.session_id,
                "requestId": request_id,
                "decision": "allow" if allow else "deny"
            }
        })

    def close(self):
        self.transport.close()

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()
