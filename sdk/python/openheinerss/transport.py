import subprocess
import json
import threading
import queue
from typing import Any, Optional


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
        self._responses: dict[int, queue.Queue[dict[str, Any]]] = {}
        self._events: queue.Queue[dict[str, Any]] = queue.Queue()
        self._lock = threading.Lock()
        self._closed = False
        self._eof_error: Optional[TransportError] = None
        self.on_event = None
        self._callbacks: queue.Queue[Optional[dict[str, Any]]] = queue.Queue()
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
            try:
                msg = json.loads(line)
            except json.JSONDecodeError:
                continue
            if "id" in msg:
                with self._lock:
                    waiting = self._responses.get(msg["id"])
                if waiting:
                    waiting.put(msg)
            elif msg.get("method"):
                self._events.put(msg)
                if self.on_event:
                    self._callbacks.put(msg)
        self._fail_pending("Servidor openheinerss encerrou a conexão (EOF no STDIO)")

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
