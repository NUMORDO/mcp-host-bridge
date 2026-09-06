#!/usr/bin/env python3
"""Exercise only bundled demo binaries, using synthetic state and ephemeral auth."""
import argparse
import json
import os
from pathlib import Path
import platform
import select
import secrets
import socket
import statistics
import subprocess
import tempfile
import time
import urllib.request
from datetime import datetime, timezone


def message(method, params=None, ident=1):
    result = {"jsonrpc": "2.0", "method": method}
    if params is not None:
        result["params"] = params
    if ident is not None:
        result["id"] = ident
    return result


INIT = message("initialize", {"protocolVersion": "2025-11-25", "capabilities": {},
                             "clientInfo": {"name": "isolated-smoke", "version": "1"}})


def stats(pid):
    """Linux gateway-only counters; missing observations remain null."""
    try:
        status = Path(f"/proc/{pid}/stat").read_text().rsplit(") ", 1)[1].split()
        return {"cpu_seconds": (int(status[11]) + int(status[12])) / os.sysconf("SC_CLK_TCK"),
                "rss_bytes": int(status[21]) * os.sysconf("SC_PAGE_SIZE")}
    except (OSError, ValueError, IndexError):
        return {"cpu_seconds": None, "rss_bytes": None}


def run(bin_dir, iterations, slow_body=False):
    bridge = (bin_dir / "mcp-host-bridge").resolve()
    demo = (bin_dir / "mcp-bridge-demo").resolve()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}/mcp/smoke/demo"
    token = secrets.token_hex(32)
    env = {**os.environ, "BRIDGE_SMOKE_TOKEN": token}
    with tempfile.TemporaryDirectory(prefix="mcp-bridge-smoke-") as directory:
        policy = Path(directory) / "policy.json"
        policy.write_text(json.dumps({"host": "smoke", "listen": f"127.0.0.1:{port}",
            "auth": {"token_env": "BRIDGE_SMOKE_TOKEN"},
            "servers": {"demo": {"command": str(demo), "tools": ["counter"]}}}))
        policy.chmod(0o600)
        daemon = subprocess.Popen([str(bridge), "serve", "--config", str(policy)],
                                  env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        local = None

        def request(payload=None, sid=None, method="POST"):
            headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json",
                       "Accept": "application/json, text/event-stream", "MCP-Protocol-Version": "2025-11-25"}
            if sid:
                headers["Mcp-Session-Id"] = sid
            req = urllib.request.Request(endpoint, data=json.dumps(payload).encode() if payload else None,
                                         headers=headers, method=method)
            with urllib.request.urlopen(req, timeout=10) as response:
                body = response.read()
                return (json.loads(body) if body else {}, response.headers.get("Mcp-Session-Id"))

        try:
            deadline = time.monotonic() + 10
            while True:
                if daemon.poll() is not None:
                    raise RuntimeError("demo gateway exited before readiness")
                try:
                    with socket.create_connection(("127.0.0.1", port), timeout=.2):
                        break
                except OSError:
                    if time.monotonic() >= deadline:
                        raise RuntimeError("demo gateway did not become ready")
                    time.sleep(.05)
            idle = stats(daemon.pid)
            slow_status = "NOT_RUN"
            if slow_body:
                with socket.create_connection(("127.0.0.1", port), timeout=15) as slow:
                    wire = (f"POST /mcp/smoke/demo HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\n"
                            f"Authorization: Bearer {token}\r\nContent-Type: application/json\r\n"
                            "Accept: application/json, text/event-stream\r\nContent-Length: 200\r\n\r\n{")
                    start = time.monotonic()
                    slow.sendall(wire.encode())
                    response = slow.recv(4096)
                    elapsed = time.monotonic() - start
                    assert elapsed < 13 and b"200 OK" not in response.split(b"\r\n")[0]
                    slow_status = "PASS"
            sessions = []
            for _ in range(2):
                reply, sid = request(INIT)
                assert reply["result"]["protocolVersion"] == "2025-11-25" and sid
                request(message("notifications/initialized", ident=None), sid)
                sessions.append(sid)
            listing, _ = request(message("tools/list", {}), sessions[0])
            assert [item["name"] for item in listing["result"]["tools"]] == ["counter"]
            hidden, _ = request(message("tools/call", {"name": "hidden", "arguments": {}}), sessions[0])
            assert "error" in hidden

            def counter(sid):
                reply, _ = request(message("tools/call", {"name": "counter", "arguments": {}}), sid)
                return int(reply["result"]["content"][0]["text"])

            assert counter(sessions[0]) == 1 and counter(sessions[0]) == 2 and counter(sessions[1]) == 1
            local = subprocess.Popen([str(bridge), "stdio", "--config", str(policy), "--server", "demo"],
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

            def local_request(payload, expects_reply=True):
                local.stdin.write(json.dumps(payload) + "\n")
                local.stdin.flush()
                if expects_reply:
                    if not select.select([local.stdout], [], [], 10)[0]:
                        raise TimeoutError("isolated stdio response deadline")
                    return json.loads(local.stdout.readline())

            assert "result" in local_request(INIT)
            local_request(message("notifications/initialized", ident=None), False)
            reply = local_request(message("tools/call", {"name": "counter", "arguments": {}}))
            assert reply["result"]["content"][0]["text"] == "1"
            assert counter(sessions[0]) == 3
            before = stats(daemon.pid)
            samples = []
            for _ in range(iterations):
                start = time.perf_counter()
                counter(sessions[0])
                samples.append((time.perf_counter() - start) * 1000)
            after = stats(daemon.pid)
            for sid in sessions:
                request(sid=sid, method="DELETE")
            local.stdin.close()
            local.wait(timeout=5)
            assert local.returncode == 0
            daemon.terminate()
            daemon.wait(timeout=5)
            assert daemon.returncode == 0
            cpu = None if before["cpu_seconds"] is None else after["cpu_seconds"] - before["cpu_seconds"]
            return {"observed_utc": datetime.now(timezone.utc).isoformat(), "os": platform.system(),
                    "architecture": platform.machine(), "iterations": iterations, "stdio_and_http": "PASS",
                    "http_sessions_isolated": "PASS", "hidden_tool_denied": "PASS", "shutdown": "PASS",
                    "slow_body_deadline": slow_status,
                    "latency_ms_median": statistics.median(samples),
                    "latency_ms_p95": sorted(samples)[int(.95 * (len(samples) - 1))],
                    "gateway_idle_rss_bytes": idle["rss_bytes"], "gateway_warm_rss_bytes": after["rss_bytes"],
                    "gateway_cpu_seconds_for_calls": cpu,
                    "measurement_scope": "synthetic counter; sequential Python urllib over loopback; gateway RSS excludes backends"}
        finally:
            for proc in (local, daemon):
                if proc is not None and proc.poll() is None:
                    proc.terminate()
                    try:
                        proc.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        proc.kill()
                        proc.wait(timeout=5)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--bin-dir", type=Path, default=Path("bin"))
    parser.add_argument("--iterations", type=int, default=200)
    parser.add_argument("--slow-body", action="store_true")
    args = parser.parse_args()
    if not 1 <= args.iterations <= 10000:
        parser.error("iterations must be 1..10000")
    print(json.dumps(run(args.bin_dir, args.iterations, args.slow_body), indent=2))
