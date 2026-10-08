import os
import threading
import time
import unittest
from openheinerss import Agent

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
            def evento(payload):
                eventos.append(payload)
                if payload.get("opcoes"):
                    decisao.append(payload)
                if "codigo" in payload:
                    terminou.set()
            agent.subscribeEvents(evento, projeto="teste-py")
            self.assertIn("instancias", agent.getLimits())
            run = agent.run({"nome": "teste-py", "motor": "mock", "texto": "responda OK", "cwd": os.getcwd(), "projeto": "teste-py"})
            self.assertTrue(run["id"].startswith("rodar-"))
            self.assertTrue(self._esperar(lambda: decisao, 1.0))
            agent.decide_run(decisao[0]["id"], "permitir")
            self.assertTrue(terminou.wait(1.5))
            self.assertGreaterEqual(len(eventos), 3)
            sessao = list(agent.stream("responda OK"))
            self.assertTrue(any(e["type"] == "agent.thinking" for e in sessao))
            self.assertIn("agentes", agent.listRuns(projeto="teste-py"))
        finally:
            agent.close()

if __name__ == "__main__": unittest.main()
