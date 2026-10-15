"""End-to-end fixtures: the simulated controller and the real heliocontrol server."""

from __future__ import annotations

from collections.abc import Iterator
from dataclasses import dataclass
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import time
import urllib.request

import pytest
import pytest_socket

# Home Assistant's harness blocks sockets before every test. These end-to-end
# tests talk to a local heliocontrol server (and on Windows the asyncio event
# loop itself needs a socket pair), so keep sockets enabled.
pytest_socket.disable_socket = lambda *args, **kwargs: None

ROOT = Path(__file__).resolve().parents[2]
EXE = ".exe" if sys.platform == "win32" else ""

# make custom_components/ importable for Home Assistant's loader
sys.path.insert(0, str(ROOT))


@pytest.fixture(autouse=True)
def auto_enable_custom_integrations(enable_custom_integrations):
    yield



@pytest.fixture(scope="session")
def binaries(tmp_path_factory) -> dict[str, Path]:
    out = tmp_path_factory.mktemp("bin")
    paths = {}
    for name in ("heliocontrol", "heliosim"):
        p = out / (name + EXE)
        subprocess.run(["go", "build", "-o", str(p), f"./cmd/{name}"], cwd=ROOT, check=True)
        paths[name] = p
    return paths


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@dataclass
class Server:
    url: str
    sim_port: int

    def get(self, path: str):
        with urllib.request.urlopen(self.url + path, timeout=10) as r:
            return json.loads(r.read() or b"null")

    def post(self, path: str, body=None, method="POST"):
        req = urllib.request.Request(
            self.url + path, data=json.dumps(body or {}).encode(), method=method, headers={"Content-Type": "application/json"}
        )
        with urllib.request.urlopen(req, timeout=10) as r:
            data = r.read()
            return json.loads(data) if data else None


def _start(binaries, tmp_path: Path, read_only: bool) -> Iterator[Server]:
    sim_port, http_port = _free_port(), _free_port()
    cfg = tmp_path / "heliocontrol.yaml"
    cfg.write_text(
        f"listen: 127.0.0.1:{http_port}\ndataDir: {tmp_path.as_posix()}/data\n"
        f"device:\n  type: tcp\n  host: 127.0.0.1\n  port: {sim_port}\n  autoConnect: true\n"
        f"pollSeconds: 10\nreadOnly: {'true' if read_only else 'false'}\n"
    )
    sim = subprocess.Popen([binaries["heliosim"], "-listen", f"127.0.0.1:{sim_port}"], stderr=subprocess.DEVNULL)
    srv = subprocess.Popen([binaries["heliocontrol"], "-config", str(cfg)], stderr=subprocess.DEVNULL)
    server = Server(url=f"http://127.0.0.1:{http_port}", sim_port=sim_port)
    try:
        deadline = time.time() + 30
        while True:
            try:
                state = server.get("/api/state")
                if state.get("status") == "online" and state.get("updated"):
                    break
            except OSError:
                pass
            if time.time() > deadline:
                raise RuntimeError("heliocontrol did not come online")
            time.sleep(0.2)
        yield server
    finally:
        for p in (srv, sim):
            p.terminate()
            p.wait(timeout=10)


@pytest.fixture
def server(binaries, tmp_path) -> Iterator[Server]:
    """A heliocontrol server in its default read-only mode."""
    yield from _start(binaries, tmp_path, read_only=True)


@pytest.fixture
def writable_server(binaries, tmp_path) -> Iterator[Server]:
    """A heliocontrol server that may write to the (simulated) heat pump."""
    yield from _start(binaries, tmp_path, read_only=False)


def pytest_configure(config):
    if not shutil.which("go"):
        pytest.exit("Go is required to build heliocontrol for these tests")
    os.environ.setdefault("TZ", "Europe/Vienna")
