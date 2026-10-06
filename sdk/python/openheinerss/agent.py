import json
import os
from typing import Generator, Dict, Any, Optional
from .transport import StdioTransport

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

        self.transport.send({
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

        while True:
            line = self.transport.read_line()
            if not line:
                raise RuntimeError("Falha ao receber resposta de session.create")
            msg = json.loads(line)
            if msg.get("id") == req_id:
                if "error" in msg:
                    raise RuntimeError(msg["error"]["message"])
                return msg["result"]["sessionId"]

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
            line = self.transport.read_line()
            if not line:
                break
            msg = json.loads(line)

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
