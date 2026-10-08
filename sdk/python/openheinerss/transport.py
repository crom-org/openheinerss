import subprocess
import json
import threading
import queue
from typing import Any, Optional

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
        self.on_event = None
        self._reader = threading.Thread(target=self._read_loop, name="openheinerss-leitor", daemon=True)
        self._reader.start()

    def send(self, payload: dict):
        line = json.dumps(payload) + "\n"
        self.proc.stdin.write(line)
        self.proc.stdin.flush()

    def request(self, payload: dict) -> dict[str, Any]:
        response_queue: queue.Queue[dict[str, Any]] = queue.Queue(maxsize=1)
        with self._lock:
            self._responses[payload["id"]] = response_queue
        self.send(payload)
        try:
            return response_queue.get()
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
                if self.on_event:
                    self.on_event(msg)
                self._events.put(msg)

    def read_line(self) -> Optional[str]:
        event = self.next_event()
        return json.dumps(event) if event is not None else None

    def close(self):
        self._closed = True
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
