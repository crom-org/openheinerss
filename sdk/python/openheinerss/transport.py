import subprocess
import json
import threading
from typing import Callable, Optional

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

    def send(self, payload: dict):
        line = json.dumps(payload) + "\n"
        self.proc.stdin.write(line)
        self.proc.stdin.flush()

    def read_line(self) -> Optional[str]:
        line = self.proc.stdout.readline()
        if not line:
            return None
        return line.strip()

    def close(self):
        if self.proc:
            self.proc.terminate()
            self.proc.wait()
