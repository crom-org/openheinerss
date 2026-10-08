import base64
import hashlib
import os
import socket
import ssl
import struct
import subprocess
import json
import threading
import queue
from typing import Any, Optional
from urllib.parse import urlsplit


class TransportError(RuntimeError):
    """Erro de transporte, inclusive quando o servidor encerra o STDIO."""

class StdioTransport:
    def __init__(self, bin_path: str = "openheinerss"):
        self.proc = subprocess.Popen(
            [bin_path, "serve", "--stdio"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1
        )
        self._init_state()
        self._start_threads()

    def _init_state(self) -> None:
        self._responses: dict[int, queue.Queue[dict[str, Any]]] = {}
        self._events: queue.Queue[dict[str, Any]] = queue.Queue()
        self._lock = threading.Lock()
        self._closed = False
        self._eof_error: Optional[TransportError] = None
        self.on_event = None
        self._callbacks: queue.Queue[Optional[dict[str, Any]]] = queue.Queue()

    def _start_threads(self) -> None:
        self._reader = threading.Thread(target=self._read_loop, name="openheinerss-leitor", daemon=True)
        self._callback_thread = threading.Thread(target=self._callback_loop, name="openheinerss-callbacks", daemon=True)
        self._reader.start()
        self._callback_thread.start()

    def send(self, payload: dict):
        if self._closed or self.proc is None or self.proc.stdin is None:
            raise self._eof_error or TransportError("Transporte do openheinerss está fechado")
        line = json.dumps(payload) + "\n"
        try:
            self.proc.stdin.write(line)
            self.proc.stdin.flush()
        except (BrokenPipeError, OSError) as exc:
            self._fail_pending(f"Servidor openheinerss encerrou a conexão: {exc}")
            raise self._eof_error from exc

    def start_request(self, payload: dict) -> "queue.Queue[dict[str, Any]]":
        """Envia sem bloquear; a resposta chega na fila devolvida (encerre com end_request)."""
        response_queue: queue.Queue[dict[str, Any]] = queue.Queue(maxsize=1)
        with self._lock:
            self._responses[payload["id"]] = response_queue
        try:
            self.send(payload)
        except BaseException:
            self.end_request(payload["id"])
            raise
        return response_queue

    def end_request(self, req_id: Any) -> None:
        with self._lock:
            self._responses.pop(req_id, None)

    def request(self, payload: dict) -> dict[str, Any]:
        response_queue: queue.Queue[dict[str, Any]] = queue.Queue(maxsize=1)
        with self._lock:
            self._responses[payload["id"]] = response_queue
        try:
            self.send(payload)
            response = response_queue.get()
            if isinstance(response, BaseException):
                raise response
            return response
        finally:
            with self._lock:
                self._responses.pop(payload["id"], None)

    def next_event(self, timeout: Optional[float] = None) -> Optional[dict[str, Any]]:
        try:
            return self._events.get(timeout=timeout)
        except queue.Empty:
            return None

    def _read_loop(self) -> None:
        while not self._closed and self.proc and self.proc.stdout:
            line = self.proc.stdout.readline()
            if not line:
                break
            self._dispatch(line)
        self._fail_pending("Servidor openheinerss encerrou a conexão (EOF no STDIO)")

    def _dispatch(self, line: str) -> None:
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            return
        if not isinstance(msg, dict):
            return
        if "id" in msg:
            with self._lock:
                waiting = self._responses.get(msg["id"])
            if waiting:
                waiting.put(msg)
        elif msg.get("method"):
            self._events.put(msg)
            if self.on_event:
                self._callbacks.put(msg)

    def _callback_loop(self) -> None:
        while True:
            msg = self._callbacks.get()
            if msg is None:
                return
            try:
                if self.on_event:
                    self.on_event(msg)
            except Exception:
                # Um callback não pode matar o despachante nem o leitor.
                continue

    def _fail_pending(self, message: str) -> None:
        with self._lock:
            if self._eof_error is None:
                self._eof_error = TransportError(message)
            self._closed = True
            pending = list(self._responses.values())
        for response_queue in pending:
            try:
                response_queue.put_nowait(self._eof_error)
            except queue.Full:
                pass

    def read_line(self) -> Optional[str]:
        event = self.next_event()
        return json.dumps(event) if event is not None else None

    def close(self):
        self._fail_pending("Transporte do openheinerss foi fechado")
        self._callbacks.put(None)
        if self.proc:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=2)
            except subprocess.TimeoutExpired:
                # É o processo filho criado pelo transporte; encerra somente ele.
                self.proc.kill()
                self.proc.wait()
            for stream in (self.proc.stdin, self.proc.stdout, self.proc.stderr):
                if stream:
                    stream.close()
            self.proc = None


_WS_GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
PORTA_PADRAO = 4820


def porta_padrao() -> int:
    """Porta do serve: OPENHEINERSS_PORTA ou 4820, como no CLI."""
    try:
        porta = int(os.environ.get("OPENHEINERSS_PORTA", ""))
        if 0 < porta < 65536:
            return porta
    except ValueError:
        pass
    return PORTA_PADRAO


class WebSocketTransport(StdioTransport):
    """JSON-RPC 2.0 por WebSocket (RFC 6455) com `openheinerss serve --porta N`, sem dependências.

    Mesma interface do StdioTransport; uma mensagem JSON por frame de texto."""

    def __init__(self, url: Optional[str] = None, host: str = "127.0.0.1", port: Optional[int] = None,
                 origin: Optional[str] = None, timeout: float = 10.0):
        if url is None:
            url = f"ws://{host}:{port or porta_padrao()}/ws"
        partes = urlsplit(url)
        if partes.scheme not in ("ws", "wss") or not partes.hostname:
            raise TransportError(f"URL WebSocket inválida: {url!r}")
        self.proc = None
        self.url = url
        self._init_state()
        self._wlock = threading.Lock()
        self._buf = b""
        tls = partes.scheme == "wss"
        porta = partes.port or (443 if tls else 80)
        try:
            sock = socket.create_connection((partes.hostname, porta), timeout=timeout)
            if tls:
                sock = ssl.create_default_context().wrap_socket(sock, server_hostname=partes.hostname)
            self._sock = sock
            self._handshake(partes, origin)
        except TransportError:
            self._abort_socket()
            raise
        except (OSError, ssl.SSLError) as exc:
            self._abort_socket()
            raise TransportError(f"Não foi possível conectar em {url}: {exc}") from exc
        self._sock.settimeout(None)
        self._start_threads()

    def _abort_socket(self) -> None:
        sock = getattr(self, "_sock", None)
        if sock is not None:
            try:
                sock.close()
            except OSError:
                pass

    def _handshake(self, partes, origin: Optional[str]) -> None:
        chave = base64.b64encode(os.urandom(16)).decode()
        caminho = (partes.path or "/") + (f"?{partes.query}" if partes.query else "")
        host = partes.hostname if ":" not in partes.hostname else f"[{partes.hostname}]"
        if partes.port:
            host += f":{partes.port}"
        linhas = [f"GET {caminho} HTTP/1.1", f"Host: {host}", "Upgrade: websocket", "Connection: Upgrade",
                  f"Sec-WebSocket-Key: {chave}", "Sec-WebSocket-Version: 13"]
        if origin:
            linhas.append(f"Origin: {origin}")
        self._sock.sendall(("\r\n".join(linhas) + "\r\n\r\n").encode())
        while b"\r\n\r\n" not in self._buf:
            pedaco = self._sock.recv(4096)
            if not pedaco:
                raise TransportError("Servidor fechou a conexão durante o handshake WebSocket")
            self._buf += pedaco
            if len(self._buf) > 65536:
                raise TransportError("Resposta de handshake WebSocket grande demais")
        cabeca, _, self._buf = self._buf.partition(b"\r\n\r\n")
        cabecalho = cabeca.decode("latin-1").split("\r\n")
        partes_status = cabecalho[0].split(" ", 2)
        status = partes_status[1] if len(partes_status) > 1 else "?"
        if status != "101":
            if status == "403":
                raise TransportError(
                    f"Servidor recusou o WebSocket (HTTP 403): origem {origin or '(nenhuma)'} não permitida; "
                    "libere-a em OPENHEINERSS_ORIGENS no serve")
            raise TransportError(f"Handshake WebSocket recusado (HTTP {status})")
        campos = {}
        for linha in cabecalho[1:]:
            nome, _, valor = linha.partition(":")
            campos[nome.strip().lower()] = valor.strip()
        esperado = base64.b64encode(hashlib.sha1((chave + _WS_GUID).encode()).digest()).decode()
        if campos.get("sec-websocket-accept") != esperado:
            raise TransportError("Sec-WebSocket-Accept inválido no handshake")

    # --- E/S de frames ---
    def _recv_exact(self, n: int) -> bytes:
        while len(self._buf) < n:
            pedaco = self._sock.recv(65536)
            if not pedaco:
                raise EOFError
            self._buf += pedaco
        dados, self._buf = self._buf[:n], self._buf[n:]
        return dados

    def _write_frame(self, opcode: int, payload: bytes) -> None:
        n = len(payload)
        cab = bytearray([0x80 | opcode])
        if n < 126:
            cab.append(0x80 | n)
        elif n < 65536:
            cab.append(0x80 | 126)
            cab += struct.pack(">H", n)
        else:
            cab.append(0x80 | 127)
            cab += struct.pack(">Q", n)
        mascara = os.urandom(4)
        cab += mascara
        corpo = (int.from_bytes(payload, "big") ^ int.from_bytes((mascara * (n // 4 + 1))[:n], "big")).to_bytes(n, "big") if n else b""
        with self._wlock:
            self._sock.sendall(bytes(cab) + corpo)

    def _read_frame(self):
        b1, b2 = self._recv_exact(2)
        n = b2 & 0x7F
        if n == 126:
            n = struct.unpack(">H", self._recv_exact(2))[0]
        elif n == 127:
            n = struct.unpack(">Q", self._recv_exact(8))[0]
        mascara = self._recv_exact(4) if b2 & 0x80 else None
        payload = self._recv_exact(n) if n else b""
        if mascara and n:
            payload = (int.from_bytes(payload, "big") ^ int.from_bytes((mascara * (n // 4 + 1))[:n], "big")).to_bytes(n, "big")
        return bool(b1 & 0x80), b1 & 0x0F, payload

    # --- interface do transporte ---
    def send(self, payload: dict):
        if self._closed:
            raise self._eof_error or TransportError("Transporte do openheinerss está fechado")
        try:
            self._write_frame(0x1, json.dumps(payload).encode())
        except OSError as exc:
            self._fail_pending(f"Servidor openheinerss encerrou a conexão: {exc}")
            raise self._eof_error from exc

    def _read_loop(self) -> None:
        mensagem = bytearray()
        em_curso = False
        motivo = "Servidor openheinerss encerrou a conexão (WebSocket fechado)"
        try:
            while not self._closed:
                fin, opcode, payload = self._read_frame()
                if opcode == 0x9:
                    self._write_frame(0xA, payload)
                elif opcode == 0xA:
                    pass
                elif opcode == 0x8:
                    try:
                        self._write_frame(0x8, payload[:2])
                    except OSError:
                        pass
                    break
                elif opcode in (0x1, 0x0):
                    if opcode == 0x1:
                        mensagem = bytearray()
                        em_curso = True
                    elif not em_curso:
                        raise TransportError("Frame de continuação sem início")
                    mensagem += payload
                    if fin:
                        em_curso = False
                        self._dispatch(bytes(mensagem).decode("utf-8", "replace"))
                elif opcode == 0x2:
                    continue  # binário: o protocolo só usa texto
        except (EOFError, OSError):
            pass
        except TransportError as exc:
            motivo = str(exc)
        self._fail_pending(motivo)

    def close(self):
        already = self._closed
        self._fail_pending("Transporte do openheinerss foi fechado")
        self._callbacks.put(None)
        if not already:
            try:
                self._write_frame(0x8, struct.pack(">H", 1000))
            except OSError:
                pass
        try:
            self._sock.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        self._sock.close()
