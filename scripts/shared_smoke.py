#!/usr/bin/env python3
"""Fixture-only stdio relay E2E. Run with mcp==1.29.1; no real registry reads."""
import argparse
import asyncio
from contextlib import AsyncExitStack
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import secrets
import socket
import subprocess
import tempfile

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client
from mcp.shared.exceptions import McpError


def children(pid):
    """Direct fixture children of our own daemon; never enumerate argv/env."""
    try:
        tasks = Path(f"/proc/{pid}/task")
        return sorted({int(child) for task in tasks.iterdir()
                       for child in (task / "children").read_text().split()})
    except OSError:
        return None


async def run(bin_dir):
    bridge = (bin_dir / "mcp-host-bridge").resolve()
    demo = (bin_dir / "mcp-bridge-demo").resolve()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    env = {"PATH": os.environ.get("PATH", ""), "SHARED_SMOKE_TOKEN": secrets.token_hex(32)}
    with tempfile.TemporaryDirectory(prefix="bridge-shared-fixture-") as tmp:
        policy = Path(tmp) / "policy.json"
        policy.write_text(json.dumps({
            "host": "fixture", "listen": f"127.0.0.1:{port}",
            "auth": {"token_env": "SHARED_SMOKE_TOKEN"},
            "servers": {"demo": {"command": str(demo), "tools": ["counter"],
                                  "backend_scope": "shared", "stateless": True}}}))
        policy.chmod(0o600)
        daemon = subprocess.Popen([str(bridge), "serve", "--config", str(policy)],
                                  env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            for _ in range(100):
                if daemon.poll() is not None:
                    raise RuntimeError("fixture daemon exited")
                try:
                    reader, writer = await asyncio.open_connection("127.0.0.1", port)
                    writer.close()
                    await writer.wait_closed()
                    break
                except OSError:
                    await asyncio.sleep(.05)
            else:
                raise TimeoutError("fixture daemon readiness")
            counts = []
            async with AsyncExitStack() as stack:
                clients = []
                for _ in range(5):
                    r, w = await stack.enter_async_context(stdio_client(StdioServerParameters(
                        command=str(bridge), args=["relay", "--config", str(policy), "--server", "demo"], env=env)))
                    client = await stack.enter_async_context(ClientSession(r, w))
                    await client.initialize()
                    clients.append(client)
                    result = await client.call_tool("counter", {})
                    assert not result.isError
                    counts.append(int(result.content[0].text))
                assert counts == [1, 2, 3, 4, 5], counts
                listing = await clients[0].list_tools()
                assert [tool.name for tool in listing.tools] == ["counter"]
                try:
                    await clients[0].call_tool("hidden", {})
                except McpError as error:
                    assert error.error.code == -32602
                else:
                    raise AssertionError("hidden tool admitted")
                active_children = children(daemon.pid)
                if active_children is not None:
                    assert len(active_children) == 1, active_children
            for _ in range(100):
                remaining = children(daemon.pid)
                if remaining in (None, []):
                    break
                await asyncio.sleep(.05)
            if remaining is not None:
                assert remaining == [], remaining
            result = {"observed_utc": datetime.now(timezone.utc).isoformat(),
                      "host": platform.node(), "platform": platform.platform(),
                      "gateway_sha256": hashlib.sha256(bridge.read_bytes()).hexdigest(),
                      "clients": 5, "counter_results": counts,
                      "backend_children": len(active_children) if active_children is not None else None,
                      "backend_children_after_close": len(remaining) if remaining is not None else None,
                      "tool_count": len(listing.tools),
                      "tool_schema_json_bytes": len(listing.model_dump_json().encode()),
                      "hidden_tool": "DENIED", "production_backends_started": 0,
                      "status": "PASS"}
        finally:
            daemon.terminate()
            try:
                daemon.wait(timeout=15)
            except subprocess.TimeoutExpired:
                daemon.kill()
                daemon.wait()
        return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--bin-dir", type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(asyncio.run(run(args.bin_dir)), indent=2))
