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
    def novo_agent(self, config=None):
        """Agent do transporte sob teste (STDIO aqui; test_websocket.py troca por WebSocket).
        `config` vira OPENHEINERSS_CONFIG do servidor."""
        antes = os.environ.get("OPENHEINERSS_CONFIG")
        if config:
            os.environ["OPENHEINERSS_CONFIG"] = config
        try:
            return Agent(harness="mock", bin_path=os.environ.get("OPENHEINERSS_BIN", "openheinerss"))
        finally:
            if config:
                if antes is None:
                    os.environ.pop("OPENHEINERSS_CONFIG", None)
                else:
                    os.environ["OPENHEINERSS_CONFIG"] = antes

    def test_capacidades_e_retomar(self):
        agent = self.novo_agent()
        try:
            cap = agent.capacidades("codex")
            self.assertEqual(cap["base"], "codex")
            self.assertEqual(cap["retomar"]["estado"], "sim")
            self.assertTrue(any(i["nome"] == "AGENTS.md" and i["estado"] == "sim" for i in cap["instrucoes"]))
            self.assertEqual(len(agent.capacidades()["harnesses"]), 5)
            v = agent.harness_versions("codex")["harnesses"]
            self.assertEqual(len(v), 1)
            self.assertEqual(v[0]["harness"], "codex")
            plano = agent.update_harness("codex", seco=True)
            self.assertTrue(plano["seco"])
            self.assertIn(plano["resultado"], ("seco", "ja_na_ultima", "sem_cli", "desconhecida", "ocupado"))
            with self.assertRaises(Exception):
                agent.update_harness("nao-existe", seco=True)
        finally:
            agent.close()
        with self.assertRaisesRegex(RuntimeError, "retomada nativa não suportada"):
            Agent(harness="aider", bin_path=os.environ.get("OPENHEINERSS_BIN", "openheinerss"), retomar="qualquer")

    def test_comandos_do_harness(self):
        cfg = tempfile.mkdtemp(prefix="openheinerss-cmd-py-")
        agent = self.novo_agent(config=cfg)
        try:
            nota = agent.annotate_command("claude-code", "/compact", "compacta o claude code; o central não usa")
            self.assertEqual(nota["anotacao"], "compacta o claude code; o central não usa")
            self.assertTrue(agent.confirmCommand("claude-code", "compact")["confirmado"])
            lista = agent.list_commands("claude-code")
            self.assertEqual(lista["arquivo"], os.path.join(cfg, "comandos.yaml"))
            compact = next(c for c in lista["comandos"] if c["nome"] == "/compact")
            self.assertTrue(compact["confirmado"])
            self.assertEqual(agent.listCommands("codex")["desconhecido"], "sem_equivalente")
        finally:
            agent.close()

    @staticmethod
    def _esperar(condicao, segundos):
        fim = time.monotonic() + segundos
        while not condicao() and time.monotonic() < fim:
            time.sleep(0.01)
        return bool(condicao())

    def test_servidor_real(self):
        agent = self.novo_agent()
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
            self.assertEqual(agent.identidade("mock")["instancia"], "mock")
            run = agent.run({"nome": "teste-py", "motor": "mock", "texto": "responda OK", "cwd": repo_temporario(), "projeto": "teste-py"})
            self.assertTrue(run["id"].startswith("rodar-"))
            self.assertTrue(self._esperar(lambda: decisao, 3.0))
            self.assertTrue(decisao_enviada.wait(1.0))
            self.assertTrue(terminou.wait(3.0))
            self.assertGreaterEqual(len(eventos), 3)
            sessao = list(agent.stream("responda OK"))
            self.assertEqual(agent.session_identity["instancia"], "mock")
            self.assertTrue(any(e["type"] == "agent.thinking" for e in sessao))
            self.assertIn("agentes", agent.listRuns(projeto="teste-py"))
        finally:
            agent.close()

    def test_negar_com_encerrar(self):
        agent = self.novo_agent()
        try:
            fins, inicios = [], []
            terminou = threading.Event()
            def evento(payload):
                if payload.get("opcoes"):
                    agent.decide_run(payload["id"], "negar", run=payload["run"], encerrar=True)
                elif "codigo" in payload:
                    fins.append(payload)
                    terminou.set()
                elif "tentativa" in payload:
                    inicios.append(payload)
            agent.subscribeEvents(evento, projeto="teste-py-negar")
            agent.run({"nome": "teste-py-negar", "motor": "mock", "texto": "responda OK", "cwd": repo_temporario(), "projeto": "teste-py-negar", "tentativas": 2})
            self.assertTrue(terminou.wait(5.0))
            self.assertEqual(fins[0]["codigo"], 3)
            self.assertEqual(fins[0]["motivo"], "negado")
            self.assertEqual(len(inicios), 1)
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


FALSO_SEM_EQUIVALENTE = """#!/usr/bin/env python3
import json, sys
for linha in sys.stdin:
    req = json.loads(linha)
    if req["method"] == "session.create":
        print(json.dumps({"jsonrpc": "2.0", "id": req["id"], "result": {"sessionId": "s1"}}), flush=True)
    elif req["method"] == "session.prompt" and req["params"]["text"].startswith("/compact"):
        print(json.dumps({"jsonrpc": "2.0", "id": req["id"], "error": {"code": -32603, "message": "codex não aceita /compact"}}), flush=True)
    elif req["method"] == "session.prompt":
        print(json.dumps({"jsonrpc": "2.0", "id": req["id"], "result": {"accepted": True}}), flush=True)
"""


class ErroDoPromptTest(unittest.TestCase):
    """Auditoria 26: /comando sem equivalente devolvia erro RPC e o SDK esperava agent.complete para sempre."""

    def _falso(self, pasta):
        falso = os.path.join(pasta, "falso")
        with open(falso, "w", encoding="utf-8") as arquivo:
            arquivo.write(FALSO_SEM_EQUIVALENTE)
        os.chmod(falso, os.stat(falso).st_mode | stat.S_IXUSR)
        return falso

    def test_erro_rpc_do_prompt_vira_excecao(self):
        with tempfile.TemporaryDirectory() as pasta:
            agent = Agent(harness="codex", bin_path=self._falso(pasta))
            try:
                inicio = time.monotonic()
                with self.assertRaisesRegex(RuntimeError, "não aceita /compact"):
                    agent.prompt("/compact")
                self.assertLess(time.monotonic() - inicio, 2)
            finally:
                agent.close()

    def test_timeout_de_seguranca_sem_eventos(self):
        with tempfile.TemporaryDirectory() as pasta:
            agent = Agent(harness="codex", bin_path=self._falso(pasta))
            try:
                with self.assertRaises(TimeoutError):
                    agent.prompt("oi", timeout=0.3)
            finally:
                agent.close()

    def test_servidor_real_codex_compact(self):
        try:
            agent = Agent(harness="codex", bin_path=os.environ.get("OPENHEINERSS_BIN", "openheinerss"), cwd=tempfile.mkdtemp(prefix="oh-py-codex-"))
        except RuntimeError as erro:
            self.skipTest(f"codex indisponível: {erro}")
        try:
            inicio = time.monotonic()
            with self.assertRaisesRegex(RuntimeError, "/compact"):
                agent.prompt("/compact", timeout=10)
            self.assertLess(time.monotonic() - inicio, 5)
        finally:
            agent.close()


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

class MCPRiscoTest(unittest.TestCase):
    def test_sem_mcp_mcp_e_classificar_risco_no_jsonrpc(self):
        with tempfile.TemporaryDirectory() as pasta:
            falso = os.path.join(pasta, "falso")
            with open(falso, "w", encoding="utf-8") as arquivo:
                arquivo.write(FALSO_PONTE)
            os.chmod(falso, os.stat(falso).st_mode | stat.S_IXUSR)
            registro = os.path.join(pasta, "reqs.jsonl")
            os.environ["PONTE_LOG"] = registro
            try:
                Agent(harness="falso", bin_path=falso, mcp=["fs"], classificar_risco=True).close()
                Agent(harness="falso", bin_path=falso, sem_mcp=True).close()
            finally:
                del os.environ["PONTE_LOG"]
            with open(registro, encoding="utf-8") as arquivo:
                criar = [json.loads(l) for l in arquivo if '"session.create"' in l]
            self.assertEqual(criar[0]["params"]["options"], {"mcp": ["fs"], "classificarRisco": True})
            self.assertEqual(criar[1]["params"]["options"], {"semMcp": True})


if __name__ == "__main__": unittest.main()
