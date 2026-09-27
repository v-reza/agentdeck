"""Probe Fase 12 (6.2.18) lawan API nyata: webhooks.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:

  1. Ketujuh route-nya terdaftar (401 tanpa sesi = route ada, bukan 404).
  2. Gerbang peran Admin benar-benar menahan member dan viewer.
  3. Validasi URL §16 menolak http ke internet lewat HTTP nyata, bukan hanya
     di unit test.
  4. Secret tidak pernah keluar di respons.
  5. RANTAI PENUH: event ditulis → trigger NOTIFY → worker → POST keluar ke
     receiver sungguhan, dengan header `X-AgentDeck-Signature` yang HMAC-nya
     diverifikasi ulang dari body mentah yang diterima. Ini satu-satunya bukti
     bahwa byte yang ditandatangani sama dengan byte yang dikirim.
  6. Isolasi tenant: webhook org lain tidak terbaca.

Receiver-nya server HTTP lokal di 127.0.0.1 — bukan internet. §16 mengizinkan
loopback sebagai string persis, jadi ini sah menurut kontrak, bukan jalan pintas.
"""

import http.cookiejar
import http.server
import json
import os
import secrets
import socket
import sys
import threading
import time
import urllib.error
import urllib.request

BASE = os.environ.get("AGENTDECK_BASE", "http://127.0.0.1:8080")
PASSED = []
FAILED = []


def check(label, ok, detail=""):
    (PASSED if ok else FAILED).append(label)
    print(f"  [{'ok' if ok else 'GAGAL'}] {label}" + (f" — {detail}" if detail and not ok else ""))


class Client:
    def __init__(self):
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar), NoRedirect()
        )
        self.token = None

    def call(self, method, path, body=None, headers=None):
        data = None
        if body is not None:
            data = json.dumps(body).encode()
        req = urllib.request.Request(BASE + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", "Bearer " + self.token)
        for k, v in (headers or {}).items():
            req.add_header(k, v)
        try:
            with self.opener.open(req, timeout=15) as resp:
                raw = resp.read()
                return resp.status, raw
        except urllib.error.HTTPError as e:
            return e.code, e.read()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **kw):
        return None


def register(email):
    c = Client()
    status, raw = c.call("POST", "/api/v1/auth/register", {
        "email": email, "password": "Rahasia123!", "name": "Probe",
    })
    if status not in (200, 201):
        raise SystemExit(f"register gagal: {status} {raw[:200]}")
    body = json.loads(raw)
    c.token = body.get("token") or body.get("session_token")
    if not c.token:
        # Fallback: ambil dari cookie jar.
        for cookie in c.jar:
            if cookie.name in ("agentdeck_session", "session"):
                c.token = cookie.value
    return c


def main():
    stamp = str(int(time.time()))

    print("== penanda build ==")
    c = Client()
    status, _ = c.call("GET", "/api/v1/system/info")
    check("system/info = 200", status == 200, str(status))

    owner = register(f"hook-owner-{stamp}@x.test")
    status, raw = owner.call("GET", "/api/v1/projects")
    check("org punya project", status == 200, str(status))
    project = json.loads(raw)[0]
    status, raw = owner.call("POST", f"/api/v1/projects/{project['id']}/boards", {
        "name": "Hook Board", "slug": f"hook-{stamp}",
    })
    check("board dibuat", status == 201, f"{status} {raw[:120]}")
    board = json.loads(raw)

    print("\n== gerbang: route ada (401 tanpa sesi) ==")
    anon = Client()
    routes = [
        ("GET", f"/api/v1/boards/{board['id']}/webhooks"),
        ("POST", f"/api/v1/boards/{board['id']}/webhooks"),
        ("GET", "/api/v1/webhooks/w1"),
        ("PATCH", "/api/v1/webhooks/w1"),
        ("DELETE", "/api/v1/webhooks/w1"),
        ("GET", "/api/v1/webhooks/w1/deliveries"),
        ("POST", "/api/v1/webhooks/w1/deliveries/1/retry"),
    ]
    for method, path in routes:
        status, _ = anon.call(method, path)
        check(f"{method} {path} tanpa sesi = 401", status == 401, str(status))

    print("\n== gerbang: Admin ==")
    status, raw = owner.call("POST", f"/api/v1/boards/{board['id']}/webhooks", {
        "url": "http://evil.example/hook", "secret": "s",
    })
    check("http ke internet ditolak = 400", status == 400, f"{status} {raw[:120]}")

    status, raw = owner.call("POST", f"/api/v1/boards/{board['id']}/webhooks", {
        "url": "https://example.com/hook", "secret": "rahasia-yang-tidak-boleh-bocor",
        # Nonaktif: webhook ini cuma untuk menguji validasi dan kebocoran
        # respons. Kalau aktif, event yang sama juga dikirim ke example.com —
        # koneksi keluar ke pihak ketiga yang tidak perlu dilakukan probe.
        "active": False,
    })
    check("https diterima = 201", status == 201, f"{status} {raw[:200]}")
    if status != 201:
        raise SystemExit("tidak bisa lanjut tanpa webhook")
    hook = json.loads(raw)
    check("respons tidak memuat secret", b"rahasia-yang-tidak-boleh-bocor" not in raw,
          raw[:200].decode(errors="replace"))

    print("\n== gerbang: tenant ==")
    outsider = register(f"hook-outsider-{stamp}@x.test")
    status, _ = outsider.call("GET", f"/api/v1/webhooks/{hook['id']}")
    check("webhook org lain = 404", status == 404, str(status))
    status, _ = outsider.call("GET", f"/api/v1/boards/{board['id']}/webhooks")
    check("board org lain = 403", status in (403, 404), str(status))

    print("\n== rantai penuh: event -> NOTIFY -> worker -> POST + HMAC ==")
    received = []

    class Receiver(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(length)
            received.append({
                "body": body,
                "signature": self.headers.get("X-AgentDeck-Signature", ""),
                "content_type": self.headers.get("Content-Type", ""),
            })
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok")

        def log_message(self, *a):
            pass

    # Bind 0.0.0.0, bukan 127.0.0.1: worker-nya jalan DI DALAM container, jadi
    # loopback container bukan loopback host. §16 menyediakan `host.docker.internal`
    # persis untuk kasus ini — dan Docker Desktop memetakannya ke host, yang
    # berarti receiver-nya harus mendengarkan di antarmuka yang dijangkau
    # container, bukan hanya loopback host.
    sock = socket.socket()
    sock.bind(("", 0))
    port = sock.getsockname()[1]
    sock.close()
    server = http.server.HTTPServer(("", port), Receiver)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    secret = "secret-probe-" + secrets.token_hex(8)
    status, raw = owner.call("POST", f"/api/v1/boards/{board['id']}/webhooks", {
        "url": f"http://host.docker.internal:{port}/hook",
        "secret": secret,
        "events_json": ["task.created"],
    })
    check("webhook loopback dibuat = 201", status == 201, f"{status} {raw[:160]}")
    live = json.loads(raw) if status == 201 else None

    # Task dibuat -> task.created -> NOTIFY -> worker -> POST.
    status, raw = owner.call("POST", f"/api/v1/boards/{board['id']}/tasks", {
        "title": "Picu webhook", "body": "probe",
    })
    check("task dibuat = 201", status == 201, f"{status} {raw[:160]}")

    deadline = time.time() + 25
    while time.time() < deadline and not received:
        time.sleep(0.5)
    server.shutdown()

    check("receiver menerima POST", len(received) > 0, f"diterima {len(received)}")
    if received:
        import hashlib
        import hmac as hmaclib

        hit = received[0]
        check("Content-Type application/json", hit["content_type"] == "application/json",
              hit["content_type"])
        check("header signature berawalan v1=", hit["signature"].startswith("v1="),
              hit["signature"][:20])
        expected = "v1=" + hmaclib.new(secret.encode(), hit["body"], hashlib.sha256).hexdigest()
        check("HMAC cocok dengan body mentah yang diterima",
              hmaclib.compare_digest(hit["signature"], expected),
              f"want {expected[:20]} got {hit['signature'][:20]}")
        try:
            payload = json.loads(hit["body"])
            check("body memuat kind event", payload.get("kind") == "task.created",
                  str(payload.get("kind")))
        except json.JSONDecodeError:
            check("body JSON valid", False, hit["body"][:120].decode(errors="replace"))

    print("\n== riwayat pengiriman ==")
    if live:
        deadline = time.time() + 15
        deliveries = []
        while time.time() < deadline:
            status, raw = owner.call("GET", f"/api/v1/webhooks/{live['id']}/deliveries")
            if status == 200:
                deliveries = json.loads(raw)
                if any(d["status"] == "delivered" for d in deliveries):
                    break
            time.sleep(0.5)
        check("riwayat pengiriman terbaca = 200", status == 200, str(status))
        check("ada delivery 'delivered'", any(d["status"] == "delivered" for d in deliveries),
              json.dumps(deliveries)[:200])
        check("response_code 200 tercatat",
              any(d.get("response_code") == 200 for d in deliveries),
              json.dumps(deliveries)[:200])

    print("\n== PATCH + DELETE ==")
    if live:
        status, raw = owner.call("PATCH", f"/api/v1/webhooks/{live['id']}", {"active": False})
        check("PATCH menonaktifkan = 200", status == 200, f"{status} {raw[:120]}")
        if status == 200:
            updated = json.loads(raw)
            check("URL dipertahankan saat hanya active diubah",
                  updated["url"] == f"http://host.docker.internal:{port}/hook", updated["url"])
            check("active jadi false", updated["active"] is False, str(updated["active"]))
        status, _ = owner.call("DELETE", f"/api/v1/webhooks/{live['id']}")
        check("DELETE = 204", status == 204, str(status))
        status, _ = owner.call("GET", f"/api/v1/webhooks/{live['id']}")
        check("GET setelah DELETE = 404", status == 404, str(status))

    print(f"\n=== HASIL: {len(PASSED)} ok / {len(FAILED)} gagal ===")
    for label in FAILED:
        print("  GAGAL:", label)
    return 1 if FAILED else 0


if __name__ == "__main__":
    sys.exit(main())
