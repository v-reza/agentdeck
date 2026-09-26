"""Fake OpenAI-compatible provider for dispatcher end-to-end proof.

Serves POST /chat/completions with a valid OpenAI-shaped body so the executor's
usage parsing and pricing path are exercised for real. POST /mode switches between
"ok" and "reject" so the failure taxonomy can be driven from the probe script
without restarting this process.

    python fake-llm.py            # listens on 127.0.0.1:9099
"""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

MODE = {"value": "ok"}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):  # keep the probe log readable
        sys.stderr.write("fake-llm: " + fmt % args + "\n")

    def _read(self):
        length = int(self.headers.get("Content-Length") or 0)
        return json.loads(self.rfile.read(length) or b"{}")

    def _send(self, code, payload):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        body = self._read()
        if self.path == "/mode":
            MODE["value"] = body.get("mode", "ok")
            return self._send(200, {"mode": MODE["value"]})

        if self.path != "/chat/completions":
            return self._send(404, {"error": {"message": "no such path"}})

        if not self.headers.get("Authorization"):
            # A credential-less call must not succeed: the point of this server is
            # to prove the agent's stored key reaches the provider.
            return self._send(401, {"error": {"message": "missing credential"}})

        if MODE["value"] == "reject":
            # 401 with the key echoed into the body: this is the leak case, and the
            # executor must redact it out of runs.error.
            key = self.headers.get("Authorization", "")
            return self._send(401, {"error": {"message": f"invalid key {key}"}})

        prompt = sum(len(m.get("content", "")) for m in body.get("messages", []))
        return self._send(200, {
            "choices": [{"message": {"content": "did the thing"}, "finish_reason": "stop"}],
            "usage": {
                "prompt_tokens": 120 + prompt,
                "completion_tokens": 42,
                "prompt_tokens_details": {"cached_tokens": 30},
                "completion_tokens_details": {"reasoning_tokens": 7},
            },
        })


if __name__ == "__main__":
    HTTPServer(("127.0.0.1", 9099), Handler).serve_forever()
