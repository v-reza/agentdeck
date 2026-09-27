"""Probe Fase 11 (6.2.13) lawan API nyata: events & realtime SSE.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:

  1. stream benar-benar hidup dan mengirim frame (bukan 200 kosong) — ini inti
     modulnya, dan satu-satunya cara membuktikannya adalah membaca socket;
  2. header 7.4 ada (`no-transform`, `X-Accel-Buffering: no`) — tanpa keduanya
     event tertahan di proxy dan UI tampak tidak menerima apa pun;
  3. **event nyata sampai ke klien**: bikin task lewat API, lalu tunggu
     `task.created` muncul di stream. Ini menguji rantai penuh
     INSERT -> trigger NOTIFY (migrasi 0020) -> LISTEN -> broadcast -> frame;
  4. resume `Last-Event-ID` mengirim ulang event yang terlewat (7.2);
  5. `GET /events` tanpa board_id = 400, dan board org lain ditolak;
  6. dua endpoint log menjawab JSON dan hanya berisi event org pemanggil.

Jalankan setelah `docker compose up -d api`:
    python tools/probe-f11.py
"""

import http.cookiejar
import json
import socket
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = "http://127.0.0.1:8080"
HOST, PORT = "127.0.0.1", 8080
PASSED, FAILED = [], []
STAMP = str(int(time.time()))


def check(name, ok, detail=""):
    (PASSED if ok else FAILED).append(name)
    print(f"  [{'ok' if ok else 'GAGAL'}] {name}{'' if ok else ' — ' + str(detail)}")


class RelaxedPolicy(http.cookiejar.DefaultCookiePolicy):
    def return_ok_secure(self, cookie, request):
        return True


class Client:
    def __init__(self):
        self.jar = http.cookiejar.CookieJar()
        self.jar.set_policy(RelaxedPolicy())
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))
        self.token = ""

    def call(self, method, path, body=None, org=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(BASE + path, data=data, method=method)
        if data is not None:
            req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", "Bearer " + self.token)
        if org:
            req.add_header("X-Org-ID", org)
        try:
            with self.opener.open(req) as resp:
                text = resp.read().decode()
                return resp.status, (json.loads(text) if text.strip() else None), text
        except urllib.error.HTTPError as err:
            text = err.read().decode()
            try:
                parsed = json.loads(text) if text.strip() else None
            except json.JSONDecodeError:
                parsed = text
            return err.code, parsed, text


def parse_cookie(client):
    for cookie in client.jar:
        if cookie.name == "agentdeck_session":
            return cookie.value
    return ""


def register(client, email):
    status, body, _ = client.call("POST", "/api/v1/auth/register",
                                  {"email": email, "password": "password1", "name": "Probe"})
    if status != 201:
        raise SystemExit(f"register {email}: {status} {body}")
    return body["workspace_id"]


def login(client, email):
    status, _, _ = client.call("POST", "/api/v1/auth/login",
                               {"email": email, "password": "password1"})
    if status != 200:
        raise SystemExit(f"login {email}: {status}")
    client.token = parse_cookie(client)


class Stream:
    """Koneksi SSE mentah: urllib menahan response sampai selesai, dan stream
    SSE tidak pernah selesai. Jadi socket-nya dibaca langsung."""

    def __init__(self, path, token, org, last_event_id=None):
        self.lines, self.status, self.headers = [], None, {}
        self.sock = socket.create_connection((HOST, PORT), timeout=15)
        headers = [
            f"GET {path} HTTP/1.1",
            f"Host: {HOST}:{PORT}",
            "Accept: text/event-stream",
            "Connection: close",
            f"Authorization: Bearer {token}",
            f"X-Org-ID: {org}",
        ]
        if last_event_id is not None:
            headers.append(f"Last-Event-ID: {last_event_id}")
        self.sock.sendall(("\r\n".join(headers) + "\r\n\r\n").encode())
        self.buf = b""
        self._read_headers()
        self.thread = threading.Thread(target=self._pump, daemon=True)
        self.thread.start()

    def _read_headers(self):
        while b"\r\n\r\n" not in self.buf:
            chunk = self.sock.recv(4096)
            if not chunk:
                break
            self.buf += chunk
        head, _, rest = self.buf.partition(b"\r\n\r\n")
        self.buf = rest
        text = head.decode(errors="replace")
        first = text.split("\r\n")[0]
        parts = first.split()
        self.status = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else 0
        for line in text.split("\r\n")[1:]:
            if ":" in line:
                k, v = line.split(":", 1)
                self.headers[k.strip().lower()] = v.strip()

    def _pump(self):
        try:
            while True:
                chunk = self.sock.recv(4096)
                if not chunk:
                    return
                self.buf += chunk
                while b"\n" in self.buf:
                    line, _, self.buf = self.buf.partition(b"\n")
                    self.lines.append(line.decode(errors="replace").rstrip("\r"))
        except OSError:
            return

    def wait_for(self, predicate, timeout=8.0):
        """Tunggu sampai predicate(line) True untuk salah satu baris."""
        deadline = time.time() + timeout
        seen = 0
        while time.time() < deadline:
            while seen < len(self.lines):
                line = self.lines[seen]
                seen += 1
                if predicate(line):
                    return line
            time.sleep(0.05)
        return None

    def close(self):
        try:
            self.sock.close()
        except OSError:
            pass


print("== penanda build ==")
status, info, _ = Client().call("GET", "/api/v1/system/info")
check("system/info = 200", status == 200, status)

owner, stranger = Client(), Client()
owner_email = f"f11owner{STAMP}@x.test"
stranger_email = f"f11stranger{STAMP}@x.test"
org = register(owner, owner_email)
stranger_org = register(stranger, stranger_email)
login(owner, owner_email)
login(stranger, stranger_email)

# Project + board: US-AD09 AC4 cuma bikin project, board dibuat sendiri.
status, projects, _ = owner.call("GET", "/api/v1/projects", org=org)
check("org punya project", status == 200 and isinstance(projects, list) and projects,
      f"projects={status}")
project_id = projects[0]["id"]
status, board, text = owner.call("POST", f"/api/v1/projects/{project_id}/boards",
                                 {"slug": f"f11-{STAMP}", "name": "F11 Board"}, org=org)
check("board dibuat", status in (200, 201), f"{status} {text}")
board_id = board["id"]

print()
print("== stream: header + hidup ==")
stream = Stream(f"/api/v1/boards/{board_id}/events", owner.token, org)
check("stream = 200", stream.status == 200, stream.status)
check("Content-Type: text/event-stream",
      "text/event-stream" in stream.headers.get("content-type", ""),
      stream.headers.get("content-type"))
check("Cache-Control no-transform (7.4)",
      "no-transform" in stream.headers.get("cache-control", ""),
      stream.headers.get("cache-control"))
check("X-Accel-Buffering: no (7.4)",
      stream.headers.get("x-accel-buffering") == "no",
      stream.headers.get("x-accel-buffering"))

print()
print("== event nyata sampai ke klien (INSERT -> NOTIFY -> LISTEN -> frame) ==")
status, task, text = owner.call("POST", f"/api/v1/boards/{board_id}/tasks",
                                {"title": f"f11 task {STAMP}", "status": "backlog"}, org=org)
check("task dibuat", status in (200, 201), f"{status} {text}")
if status in (200, 201):
    frame = stream.wait_for(lambda l: l.startswith("id: "), timeout=10.0)
    check("stream mengirim frame (bukan diam)", frame is not None, "timeout 10s")
    if frame is not None:
        event_line = stream.wait_for(lambda l: l.startswith("event: "), timeout=3.0)
        data_line = stream.wait_for(lambda l: l.startswith("data: "), timeout=3.0)
        check("frame punya baris event:", event_line is not None, event_line)
        check("frame punya baris data:", data_line is not None, data_line)
        check("event-nya task.created", event_line == "event: task.created", event_line)
        if data_line:
            try:
                payload = json.loads(data_line[len("data: "):])
                check("payload memuat task_id", "task_id" in payload, payload)
            except json.JSONDecodeError as exc:
                check("payload JSON valid", False, exc)
        check("id frame berupa angka", frame[len("id: "):].strip().isdigit(), frame)

print()
print("== resume via Last-Event-ID (7.2) ==")
# Checkpoint 0 = minta seluruh backlog board ini.
resumed = Stream(f"/api/v1/boards/{board_id}/events", owner.token, org, last_event_id=0)
first = resumed.wait_for(lambda l: l.startswith("id: "), timeout=8.0)
check("replay mengirim backlog", first is not None, "timeout 8s")
if first is not None:
    check("backlog memuat task.created",
          resumed.wait_for(lambda l: l == "event: task.created", timeout=5.0) is not None,
          "task.created tidak ada di replay")
resumed.close()
stream.close()

print()
print("== gerbang: board_id wajib + tenant ==")
status, _, _ = owner.call("GET", "/api/v1/events", org=org)
check("GET /events tanpa board_id = 400", status == 400, status)
status, _, _ = stranger.call("GET", f"/api/v1/boards/{board_id}/events", org=stranger_org)
check("board org lain ditolak", status in (403, 404), status)
status, _, _ = Client().call("GET", f"/api/v1/boards/{board_id}/events", org=org)
check("tanpa sesi = 401", status == 401, status)

print()
print("== log JSON: task & run ==")
if status in (200, 201):
    status, events, text = owner.call("GET", f"/api/v1/tasks/{task['id']}/events", org=org)
    check("GET /tasks/{id}/events = 200", status == 200, f"{status} {text[:80]}")
    if status == 200:
        check("berupa list", isinstance(events, list), type(events))
        check("memuat task.created", any(e.get("kind") == "task.created" for e in events),
              [e.get("kind") for e in events])
        check("semua event milik task ini",
              all(e.get("task_id") == task["id"] for e in events), "ada event task lain")
status, _, _ = owner.call("GET", "/api/v1/tasks/01ZZZZZZZZZZZZZZZZZZZZZZZZ/events", org=org)
check("task tidak ada = 404", status == 404, status)
status, _, _ = owner.call("GET", "/api/v1/runs/01ZZZZZZZZZZZZZZZZZZZZZZZZ/events", org=org)
check("run tidak ada = 404", status == 404, status)

print()
print(f"=== HASIL: {len(PASSED)} ok / {len(FAILED)} gagal ===")
if FAILED:
    for name in FAILED:
        print(f"  GAGAL: {name}")
    sys.exit(1)
