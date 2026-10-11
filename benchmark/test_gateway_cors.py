"""Isolated Nginx CORS regression; requires Docker and nginx:1.27-alpine.

Run: python3 benchmark/test_gateway_cors.py
No running demo services or database contents are modified.
"""
import pathlib
import subprocess
import unittest
import urllib.error
import urllib.request
import time


class GatewayCorsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        config = pathlib.Path(__file__).with_name("gateway.conf").resolve()
        cls.container = subprocess.check_output([
            "docker", "run", "--rm", "-d", "-p", "127.0.0.1::80",
            "--add-host", "data-plane-1:127.0.0.1", "--add-host", "data-plane-2:127.0.0.1",
            "-v", f"{config}:/etc/nginx/conf.d/default.conf:ro", "nginx:1.27-alpine",
        ], text=True).strip()
        cls.addClassCleanup(subprocess.run, ["docker", "rm", "-f", cls.container], check=True, stdout=subprocess.DEVNULL)
        address = subprocess.check_output(["docker", "port", cls.container, "80/tcp"], text=True).strip()
        cls.url = f"http://{address}/mcp/v1/virtual/demo"
        for _ in range(40):
            try:
                cls.request("http://localhost:3300")
                break
            except OSError:
                time.sleep(0.1)
        else:
            raise RuntimeError("Nginx did not start")

    @classmethod
    def request(cls, origin, method="OPTIONS", path=None):
        request = urllib.request.Request(path or cls.url, method=method, headers={
            "Origin": origin, "Access-Control-Request-Method": "POST",
            "Access-Control-Request-Headers": "authorization,content-type,mcp-protocol-version,mcp-session-id",
        })
        try:
            return urllib.request.urlopen(request, timeout=3)
        except urllib.error.HTTPError as error:
            return error

    def test_exact_allowed_origins_and_session_headers(self):
        for origin in ["http://localhost:3300", "http://localhost:8800"]:
            with self.request(origin) as response:
                self.assertEqual(response.status, 204)
                self.assertEqual(response.headers["Access-Control-Allow-Origin"], origin)
                self.assertIn("MCP-Session-Id", response.headers["Access-Control-Expose-Headers"])
                self.assertIn("Authorization", response.headers["Access-Control-Allow-Headers"])
                self.assertIn("DELETE", response.headers["Access-Control-Allow-Methods"])
                self.assertEqual(response.headers["Vary"], "Origin")
                self.assertIsNone(response.headers.get("Access-Control-Allow-Credentials"))

    def test_untrusted_or_similar_origins_are_denied(self):
        for origin in ["https://attacker.example", "http://localhost:3300.attacker.example", "null"]:
            with self.request(origin) as response:
                self.assertEqual(response.status, 403)
                self.assertIsNone(response.headers.get("Access-Control-Allow-Origin"))

    def test_cors_is_scoped_to_virtual_mcp_and_present_on_errors(self):
        with self.request("http://localhost:3300", method="POST") as response:
            self.assertEqual(response.status, 502)  # isolated fixture has no data planes
            self.assertEqual(response.headers["Access-Control-Allow-Origin"], "http://localhost:3300")
        with self.request("http://localhost:3300", path=self.url.replace("/mcp/v1/virtual/demo", "/api/v1/me")) as response:
            self.assertIsNone(response.headers.get("Access-Control-Allow-Origin"))


if __name__ == "__main__":
    unittest.main()
