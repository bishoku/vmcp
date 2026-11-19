"""Real FastMCP endpoint; the modern-only deployment requires a revision header."""
import os
import socket
import sys
from pathlib import Path

from fastmcp import FastMCP
from starlette.datastructures import Headers
from starlette.responses import PlainTextResponse
import uvicorn

profile, ready = sys.argv[1:]
server = FastMCP("interop-fastmcp")

@server.tool
def echo(value: str) -> str:
    """Echo a supplied value."""
    with open(os.environ["VMCP_EXECUTIONS"], "a", encoding="utf-8") as log:
        print(value, file=log)
    return value

app = server.http_app(path="/mcp", json_response=True)

async def endpoint(scope, receive, send):
    # FastMCP 4 serves both eras. This deployment explicitly restricts HTTP
    # access to modern clients; accepted exchanges go unmodified to FastMCP.
    if profile == "modern" and scope["type"] == "http" and Headers(scope=scope).get("mcp-protocol-version") != "2026-07-28":
        await PlainTextResponse("Modern revision required", 400)(scope, receive, send)
        return
    await app(scope, receive, send)

with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    listener.listen()
    Path(ready).write_text(f"http://127.0.0.1:{listener.getsockname()[1]}/mcp")
    uvicorn.Server(uvicorn.Config(endpoint, log_level="warning")).run(sockets=[listener])
