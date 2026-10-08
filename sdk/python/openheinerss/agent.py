import json
import os
from typing import Generator, Dict, Any, Optional, Callable, TypedDict
from .transport import StdioTransport

class RunOptions(TypedDict, total=False):
    nome: str; motor: str; modelo: str; esforco: str; prompt: str; texto: str; retomar: bool
    pasta: str; branchBase: str; cargaMax: float; maxAgentes: int; tentativas: int; cotaMax: float; cwd: str; projeto: str

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
        bin_path: str = "openheinerss"
    ):
        self.transport = StdioTransport(bin_path)
        self.req_id = 1
        self.callbacks: Dict[str, Callable[[Dict[str, Any]], None]] = {}
        self.transport.on_event = self._event
        self.session_id = self._create_session(
            harness=harness,
            mode=mode,
            provider=provider,
            model=model,
            cwd=cwd or os.getcwd()
        )

    def _create_session(self, harness: str, mode: str, provider: Optional[str], model: Optional[str], cwd: str) -> str:
        req_id = self.req_id
        self.req_id += 1

        response = self.transport.request({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": "session.create",
            "params": {
                "harness": harness,
                "mode": mode,
                "provider": provider,
                "model": model,
                "cwd": cwd
            }
        })

        if "error" in response:
            raise RuntimeError(response["error"]["message"])
        return response["result"]["sessionId"]

    def stream(self, text: str) -> Generator[Dict[str, Any], None, None]:
        req_id = self.req_id
        self.req_id += 1

        self.transport.send({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": "session.prompt",
            "params": {
                "sessionId": self.session_id,
                "text": text
            }
        })

        while True:
            msg = self.transport.next_event()
            if not msg:
                break
            if "method" in msg:
                method = msg["method"]
                params = msg.get("params", {})

                yield {"type": method, "data": params}

                if method == "agent.permission_request":
                    # Auto-autoriza em modo stream simples
                    self.respond_permission(params.get("requestId"), True)

                if method == "agent.complete":
                    break

    def prompt(self, text: str) -> str:
        output = []
        for event in self.stream(text):
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
        return msg.get("result")

    def list_harnesses(self) -> list[dict[str, Any]]:
        return self._request("harness.listar", {}).get("harnesses", [])

    def run(self, options: Dict[str, Any]) -> Dict[str, Any]:
        return self._request("rodar.iniciar", options)

    def list_runs(self, **filter: str) -> Dict[str, Any]:
        return self._request("rodar.listar", filter)

    def stop_run(self, id: Optional[str] = None, agente: Optional[str] = None) -> None:
        self._request("rodar.parar", {k: v for k, v in {"id": id, "agente": agente}.items() if v})

    def decide_run(self, id: str, resposta: str, mensagem: Optional[str] = None) -> None:
        self._request("rodar.decidir", {"id": id, "resposta": resposta, "mensagem": mensagem})

    def get_limits(self) -> Dict[str, Any]:
        return self._request("limites.obter", {})

    def subscribe_events(self, callback: Optional[Callable[[Dict[str, Any]], None]] = None, **filter: str) -> None:
        if callback:
            for name in ("orq.inicio", "orq.progresso", "orq.fim", "orq.erro", "orq.precisa_decisao"):
                self.callbacks[name] = callback
        self._request("eventos.assinar", filter)

    listHarnesses = list_harnesses
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
