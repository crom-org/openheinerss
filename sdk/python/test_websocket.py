"""Os testes de API do SDK contra um `serve` WebSocket real, mais os casos próprios do transporte.

Compila (ou usa OPENHEINERSS_BIN), sobe `serve --porta P` numa porta livre e encerra só o PID iniciado."""
import base64
import hashlib
import os
import socket
import struct
import subprocess
import tempfile
import threading
import time
import unittest

from openheinerss import Agent
from openheinerss.transport import TransportError, WebSocketTransport
import test_integration
from test_integration import SDKIntegrationTest

RAIZ = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
_GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def binario() -> str:
    if os.environ.get("OPENHEINERSS_BIN"):
        return os.environ["OPENHEINERSS_BIN"]
    pasta = tempfile.mkdtemp(prefix="openheinerss-ws-py-bin-")
    destino = os.path.join(pasta, "openheinerss")
    subprocess.run(["go", "build", "-o", destino, "./cmd/openheinerss"], cwd=RAIZ, check=True)
    return destino


BIN = None


def porta_livre() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class Servidor:
    def __init__(self, env=None):
        global BIN
        if BIN is None:
            BIN = binario()
        ambiente = dict(os.environ)
        ambiente.update(env or {})
        self.proc = None
        for _ in range(5):
            self.porta = porta_livre()
            self.proc = subprocess.Popen([BIN, "serve", "--porta", str(self.porta), "--host", "127.0.0.1"],
                                         env=ambiente, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if self._esperar():
                return
            self.parar()
        raise RuntimeError("serve não abriu a porta")

    def _esperar(self) -> bool:
        fim = time.monotonic() + 10
        while time.monotonic() < fim:
            if self.proc.poll() is not None:
                return False
            try:
                socket.create_connection(("127.0.0.1", self.porta), timeout=0.2).close()
                return True
            except OSError:
                time.sleep(0.05)
        return False

    def parar(self):
        if self.proc and self.proc.poll() is None:
            self.proc.terminate()  # só o PID que este teste iniciou
            try:
                self.proc.wait(timeout=3)
            except subprocess.TimeoutExpired:
                self.proc.kill()
                self.proc.wait()


class WebSocketIntegrationTest(SDKIntegrationTest):
    """Reexecuta os testes de API do STDIO por WebSocket."""

    def setUp(self):
        self.servidores = []

    def tearDown(self):
        for s in self.servidores:
            s.parar()

    def novo_agent(self, config=None):
        srv = Servidor({"OPENHEINERSS_CONFIG": config} if config else None)
        self.servidores.append(srv)
        return Agent(harness="mock", transport="websocket", port=srv.porta)

    def test_url_e_porta_padrao_por_ambiente(self):
        srv = Servidor()
        self.servidores.append(srv)
        antes = os.environ.get("OPENHEINERSS_PORTA")
        os.environ["OPENHEINERSS_PORTA"] = str(srv.porta)
        try:
            with Agent(harness="mock", transport="websocket") as a:
                self.assertIn("instancias", a.get_limits())
        finally:
            if antes is None:
                os.environ.pop("OPENHEINERSS_PORTA")
            else:
                os.environ["OPENHEINERSS_PORTA"] = antes
        with Agent(harness="mock", url=f"ws://127.0.0.1:{srv.porta}/ws", origin="http://localhost:3000") as a:
            self.assertEqual(a.prompt("oi")[:0], "")

    def test_origem_nao_permitida_e_recusada(self):
        srv = Servidor()
        self.servidores.append(srv)
        with self.assertRaisesRegex(TransportError, "403.*origem.*OPENHEINERSS_ORIGENS"):
            Agent(harness="mock", transport="websocket", port=srv.porta, origin="http://evil.example")
        srv2 = Servidor({"OPENHEINERSS_ORIGENS": "http://evil.example"})
        self.servidores.append(srv2)
        with Agent(harness="mock", transport="websocket", port=srv2.porta, origin="http://evil.example") as a:
            self.assertIn("instancias", a.get_limits())

    def test_conexao_recusada_vira_transport_error(self):
        with self.assertRaises(TransportError):
            WebSocketTransport(port=porta_livre(), timeout=1)

    def test_queda_do_servidor_falha_pendentes(self):
        # Servidor falso que aceita o handshake e cala; a queda é a do socket dele.
        resultado = []
        falso = _ServidorFalso()
        try:
            t = WebSocketTransport(port=falso.porta)
            th = threading.Thread(target=lambda: resultado.append(_req(t)))
            th.start()
            time.sleep(0.1)
            falso.derrubar()
            th.join(2)
            self.assertFalse(th.is_alive())
            self.assertIsInstance(resultado[-1], TransportError)
            with self.assertRaises(TransportError):
                t.send({"jsonrpc": "2.0", "id": 1, "method": "x"})
        finally:
            t.close()
            falso.fechar()

    def test_servidor_real_derrubado_falha_prompt(self):
        srv = Servidor()
        self.servidores.append(srv)
        agent = Agent(harness="mock", transport="websocket", port=srv.porta)
        try:
            srv.parar()
            with self.assertRaises((TransportError, RuntimeError)):
                for _ in range(50):
                    agent.get_limits()
                    time.sleep(0.05)
        finally:
            agent.close()

    def test_frames_fragmentados_ping_e_payload_grande(self):
        """Servidor falso: ecoa JSON-RPC em fragmentos, com ping no meio e resposta de 70 mil bytes (64 bits)."""
        falso = _ServidorFalso(modo="eco")
        try:
            t = WebSocketTransport(port=falso.porta)
            r = t.request({"jsonrpc": "2.0", "id": 5, "method": "eco", "params": {"x": "ç" * 10}})
            self.assertEqual(r["result"]["x"], "ç" * 10)
            r = t.request({"jsonrpc": "2.0", "id": 6, "method": "eco", "params": {"x": "a" * 70000}})
            self.assertEqual(len(r["result"]["x"]), 70000)
            self.assertTrue(falso.recebeu_pong.wait(2))
            t.close()
        finally:
            falso.fechar()


def _req(t):
    try:
        return t.request({"jsonrpc": "2.0", "id": 9, "method": "x", "params": {}})
    except Exception as e:  # noqa: BLE001
        return e


class _ServidorFalso:
    """WebSocket mínimo para testar o cliente: aceita um cliente; modo 'mudo' ou 'eco'."""

    def __init__(self, modo="mudo"):
        self.modo = modo
        self.recebeu_pong = threading.Event()
        self.srv = socket.socket()
        self.srv.bind(("127.0.0.1", 0))
        self.srv.listen(1)
        self.porta = self.srv.getsockname()[1]
        self.conn = None
        threading.Thread(target=self._servir, daemon=True).start()

    def _ler(self, n):
        b = b""
        while len(b) < n:
            p = self.conn.recv(n - len(b))
            if not p:
                raise EOFError
            b += p
        return b

    def _frame(self, fin, op, dados):
        n = len(dados)
        cab = bytes([(0x80 if fin else 0) | op])
        cab += bytes([n]) if n < 126 else (bytes([126]) + struct.pack(">H", n) if n < 65536 else bytes([127]) + struct.pack(">Q", n))
        self.conn.sendall(cab + dados)

    def _servir(self):
        try:
            self.conn, _ = self.srv.accept()
            req = b""
            while b"\r\n\r\n" not in req:
                req += self.conn.recv(4096)
            chave = [l.split(b":", 1)[1].strip() for l in req.split(b"\r\n") if l.lower().startswith(b"sec-websocket-key")][0]
            accept = base64.b64encode(hashlib.sha1(chave + _GUID.encode()).digest())
            self.conn.sendall(b"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + b"\r\n\r\n")
            while True:
                b1, b2 = self._ler(2)
                assert b2 & 0x80, "cliente deve mascarar"
                n = b2 & 0x7F
                if n == 126:
                    n = struct.unpack(">H", self._ler(2))[0]
                elif n == 127:
                    n = struct.unpack(">Q", self._ler(8))[0]
                m = self._ler(4)
                d = bytes(c ^ m[i % 4] for i, c in enumerate(self._ler(n)))
                op = b1 & 0x0F
                if op == 0xA:
                    self.recebeu_pong.set()
                elif op == 0x8:
                    return
                elif op == 0x1 and self.modo == "eco":
                    import json
                    req = json.loads(d)
                    resp = json.dumps({"jsonrpc": "2.0", "id": req["id"], "result": req["params"]}, ensure_ascii=False).encode()
                    meio = len(resp) // 2
                    self._frame(False, 0x1, resp[:meio])
                    self._frame(True, 0x9, b"ping")  # controle entre fragmentos
                    self._frame(False, 0x0, resp[meio:meio + 1])
                    self._frame(True, 0x0, resp[meio + 1:])
        except (EOFError, OSError):
            pass

    def derrubar(self):
        if self.conn:
            try:
                self.conn.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            self.conn.close()

    def fechar(self):
        self.derrubar()
        self.srv.close()


if __name__ == "__main__":
    unittest.main()
