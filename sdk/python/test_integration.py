import json
import os
import stat
import threading
import time
import tempfile
import unittest
from openheinerss import Agent
from openheinerss.transport import StdioTransport, TransportError

def repo_temporario():
    """Repositório git descartável: o teste nunca cria worktrees no repositório real."""
    import subprocess, tempfile
    pasta = tempfile.mkdtemp(prefix="openheinerss-sdk-py-")
    subprocess.run(["git", "init", "-q", "-b", "main", pasta], check=True)
    subprocess.run(["git", "-C", pasta, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"], check=True)
    return pasta


class SDKIntegrationTest(unittest.TestCase):
    @staticmethod
    def _esperar(condicao, segundos):
        fim = time.monotonic() + segundos
        while not condicao() and time.monotonic() < fim:
            time.sleep(0.01)
        return bool(condicao())

    def test_servidor_real(self):
        agent = Agent(harness="mock", bin_path=os.environ.get("OPENHEINERSS_BIN", "openheinerss"))
        try:
            agent.registerHarness({"name": "py-test-harness", "base": "mock"})
            self.assertTrue(any(x["id"] == "py-test-harness" for x in agent.listHarnesses()))
            eventos = []
            terminou = threading.Event()
            decisao = []
            decisao_enviada = threading.Event()
            def evento(payload):
                eventos.append(payload)
                if payload.get("opcoes"):
                    decisao.append(payload)
                    agent.decide_run(payload["id"], "permitir", run=payload["run"])
                    decisao_enviada.set()
                if "codigo" in payload:
                    terminou.set()
            agent.subscribeEvents(evento, projeto="teste-py")
            self.assertIn("instancias", agent.getLimits())
            run = agent.run({"nome": "teste-py", "motor": "mock", "texto": "responda OK", "cwd": repo_temporario(), "projeto": "teste-py"})
            self.assertTrue(run["id"].startswith("rodar-"))
            self.assertTrue(self._esperar(lambda: decisao, 3.0))
            self.assertTrue(decisao_enviada.wait(1.0))
            self.assertTrue(terminou.wait(1.5))
            self.assertGreaterEqual(len(eventos), 3)
            sessao = list(agent.stream("responda OK"))
            self.assertTrue(any(e["type"] == "agent.thinking" for e in sessao))
            self.assertIn("agentes", agent.listRuns(projeto="teste-py"))
        finally:
            agent.close()

    def test_eof_falha_requisicao_pendente(self):
        with tempfile.TemporaryDirectory() as pasta:
            falso = os.path.join(pasta, "servidor falso")
            with open(falso, "w", encoding="utf-8") as arquivo:
                arquivo.write("#!/usr/bin/env python3\nimport sys\nimport time\nsys.stdin.readline()\ntime.sleep(30)\n")
            os.chmod(falso, os.stat(falso).st_mode | stat.S_IXUSR)
            transporte = StdioTransport(bin_path=falso)
            resultado = []
            espera = threading.Thread(target=lambda: resultado.append(_request_eof(transporte)))
            espera.start()
            time.sleep(0.05)
            transporte.proc.terminate()
            espera.join(1.0)
            self.assertFalse(espera.is_alive())
            self.assertIsInstance(resultado[0], TransportError)
            self.assertIn("conexão", str(resultado[0]))
            transporte.close()


FALSO_PONTE = """#!/usr/bin/env python3
import json, sys
for linha in sys.stdin:
    req = json.loads(linha)
    with open(__import__("os").environ["PONTE_LOG"], "a") as f:
        f.write(json.dumps(req) + "\\n")
    if req["method"] == "session.create":
        print(json.dumps({"jsonrpc": "2.0", "id": req["id"], "result": {"sessionId": "s1"}}), flush=True)
    elif req["method"] == "session.prompt":
        print(json.dumps({"jsonrpc": "2.0", "id": req["id"], "result": {"accepted": True}}), flush=True)
        print(json.dumps({"jsonrpc": "2.0", "method": "agent.raw", "params": {"sessionId": "s1", "harness": "falso", "stream": "stderr", "line": "linha crua"}}), flush=True)
        print(json.dumps({"jsonrpc": "2.0", "method": "agent.complete", "params": {"reason": "completed"}}), flush=True)
"""


class PonteTest(unittest.TestCase):
    def test_harness_args_no_jsonrpc_e_agent_raw_entregue(self):
        with tempfile.TemporaryDirectory() as pasta:
            falso = os.path.join(pasta, "falso")
            with open(falso, "w", encoding="utf-8") as arquivo:
                arquivo.write(FALSO_PONTE)
            os.chmod(falso, os.stat(falso).st_mode | stat.S_IXUSR)
            registro = os.path.join(pasta, "reqs.jsonl")
            os.environ["PONTE_LOG"] = registro
            agent = Agent(harness="falso", bin_path=falso, effort="high", harness_args=["--x=a,b", "/compact", "c d"])
            try:
                recebidos = []
                agent.on("agent.raw", recebidos.append)
                eventos = list(agent.stream("/model x"))
                self.assertTrue(any(e["type"] == "agent.raw" and e["data"]["line"] == "linha crua" for e in eventos))
                self.assertTrue(StdioTransportEsperar(lambda: recebidos))
                self.assertEqual(recebidos[0]["stream"], "stderr")
            finally:
                agent.close()
                del os.environ["PONTE_LOG"]
            with open(registro, encoding="utf-8") as arquivo:
                reqs = [json.loads(l) for l in arquivo]
            criar = next(r for r in reqs if r["method"] == "session.create")
            self.assertEqual(criar["params"]["options"], {"effort": "high", "harnessArgs": ["--x=a,b", "/compact", "c d"]})
            prompt = next(r for r in reqs if r["method"] == "session.prompt")
            self.assertEqual(prompt["params"]["text"], "/model x")


def StdioTransportEsperar(condicao, segundos=1.0):
    fim = time.monotonic() + segundos
    while not condicao() and time.monotonic() < fim:
        time.sleep(0.01)
    return bool(condicao())


def _request_eof(transporte):
    try:
        transporte.request({"jsonrpc": "2.0", "id": 99, "method": "teste", "params": {}})
    except Exception as erro:  # noqa: BLE001 - a asserção verifica o erro de transporte
        return erro
    return None

if __name__ == "__main__": unittest.main()
