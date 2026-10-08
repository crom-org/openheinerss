import os
import unittest
from openheinerss import Agent

class SDKIntegrationTest(unittest.TestCase):
    def test_servidor_real(self):
        agent = Agent(harness="mock", bin_path=os.environ.get("OPENHEINERSS_BIN", "openheinerss"))
        try:
            agent.registerHarness({"name": "py-test-harness", "base": "mock"})
            self.assertTrue(any(x["id"] == "py-test-harness" for x in agent.listHarnesses()))
            eventos = []
            agent.subscribeEvents(eventos.append, projeto="teste-py")
            self.assertIn("instancias", agent.getLimits())
            run = agent.run({"nome": "teste-py", "motor": "mock", "prompt": "responda OK", "cwd": os.getcwd(), "projeto": "teste-py"})
            self.assertTrue(run["id"].startswith("rodar-"))
            agent.stopRun(id=run["id"])
            self.assertIn("agentes", agent.listRuns(projeto="teste-py"))
        finally:
            agent.close()

if __name__ == "__main__": unittest.main()
