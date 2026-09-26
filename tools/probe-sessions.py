"""Probe Fase 3 (6.2.2) lawan API nyata: sesi aktif, ganti password, tutup akun.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:
  1. keempat route-nya terdaftar (401 tanpa sesi = route ada, bukan 404);
  2. sesi yang direkam benar-benar menyimpan user agent + IP dari request;
  3. mencabut sesi mematikan token itu di request berikutnya (bukan cuma
     menghapusnya dari daftar);
  4. ganti password mencabut sesi LAIN dan mempertahankan sesi ini;
  5. tutup akun mencabut semua sesi dan menghentikan login;
  6. batas tenant: admin workspace A tidak bisa mencabut sesi orang di B.

Jalankan setelah `docker compose up -d api`:
    python tools/probe-sessions.py
"""

import http.cookiejar
import json
import sys
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"
PASSED, FAILED = [], []


def check(name, ok, detail=""):
    (PASSED if ok else FAILED).append(name)
    print(f"  [{'ok' if ok else 'GAGAL'}] {name}{'' if ok else ' — ' + str(detail)}")


class Client:
    """Satu klien = satu cookie jar = satu perangkat."""

    def __init__(self, user_agent="probe/1.0"):
        self.jar = http.cookiejar.CookieJar()
        # Cookie sesinya `Secure`, dan cookiejar Python menolak mengirim cookie
        # Secure lewat HTTP. Browser memperlakukan loopback sebagai trustworthy;
        # Python tidak. Policy ini menyamakan keduanya untuk probe lokal.
        self.jar.set_policy(RelaxedPolicy())
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))
        self.user_agent = user_agent
        self.token = ""

    def call(self, method, path, body=None, token=None, org=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(BASE + path, data=data, method=method)
        req.add_header("User-Agent", self.user_agent)
        if data is not None:
            req.add_header("Content-Type", "application/json")
        if token:
            req.add_header("Authorization", "Bearer " + token)
        if org:
            req.add_header("X-Org-ID", org)
        try:
            with self.opener.open(req) as resp:
                raw = resp.read().decode()
                return resp.status, (json.loads(raw) if raw.strip() else None), resp.headers
        except urllib.error.HTTPError as err:
            raw = err.read().decode()
            try:
                parsed = json.loads(raw) if raw.strip() else None
            except json.JSONDecodeError:
                parsed = raw
            return err.code, parsed, err.headers


class RelaxedPolicy(http.cookiejar.DefaultCookiePolicy):
    """Cookiejar yang mengirim cookie `Secure` ke loopback lewat HTTP."""

    def return_ok_secure(self, cookie, request):
        return True


def register(client, email, password="password1"):
    status, body, headers = client.call("POST", "/api/v1/auth/register",
                                        {"email": email, "password": password, "name": "Probe"})
    if status != 201:
        raise SystemExit(f"register {email}: {status} {body}")
    client.token = parse_cookie(headers)
    return body


def login(client, email, password="password1"):
    status, body, headers = client.call("POST", "/api/v1/auth/login",
                                        {"email": email, "password": password})
    if status != 200:
        raise SystemExit(f"login {email}: {status} {body}")
    client.token = parse_cookie(headers)


def parse_cookie(headers):
    for value in headers.get_all("Set-Cookie") or []:
        for part in value.split(";"):
            name, _, val = part.strip().partition("=")
            if name == "agentdeck_session":
                return val
    return ""


def main():
    import time
    suffix = str(int(time.time()))
    email = f"probe.sessions.{suffix}@example.com"
    other = f"probe.other.{suffix}@example.com"

    print("== route terdaftar (401 tanpa sesi, bukan 404) ==")
    anon = Client()
    for method, path in [
        ("GET", "/api/v1/auth/sessions"),
        ("DELETE", "/api/v1/auth/sessions/01ABC"),
        ("POST", "/api/v1/auth/password/change"),
        ("DELETE", "/api/v1/auth/me"),
    ]:
        status, _, _ = anon.call(method, path, {} if method != "GET" else None)
        check(f"{method} {path} menolak tanpa sesi", status == 401, f"status={status}")

    print("== rekam perangkat + daftar sesi (US-AD90 AC2) ==")
    device_one = Client("AgentDeckProbe/one")
    workspace = register(device_one, email)
    org = workspace["workspace_id"]

    device_two = Client("AgentDeckProbe/two")
    login(device_two, email)

    status, rows, _ = device_two.call("GET", "/api/v1/auth/sessions", org=org)
    check("GET /auth/sessions → 200", status == 200, f"status={status} body={rows}")
    if status == 200:
        check("dua sesi terdaftar", len(rows) == 2, f"rows={rows}")
        agents = {r["user_agent"] for r in rows}
        check("user agent direkam dari request",
              "AgentDeckProbe/one" in agents and "AgentDeckProbe/two" in agents, agents)
        check("ip direkam dari request", all(r["ip"] for r in rows), rows)
        check("last_seen_at terisi", all(r["last_seen_at"] for r in rows), rows)
        check("tepat satu sesi ditandai current",
              sum(1 for r in rows if r["current"]) == 1, rows)

    print("== cabut sesi sendiri mematikan tokennya (US-AD90 AC4) ==")
    if status == 200:
        older = [r for r in rows if not r["current"]][0]["id"]
        code, _, _ = device_two.call("DELETE", f"/api/v1/auth/sessions/{older}", org=org)
        check("DELETE sesi lain → 204", code == 204, f"status={code}")
        # Token perangkat satu harus mati di request berikutnya.
        code, _, _ = device_one.call("GET", "/api/v1/auth/sessions", org=org)
        check("token yang dicabut ditolak 401", code == 401, f"status={code}")
        # Dan perangkat dua masih hidup.
        code, _, _ = device_two.call("GET", "/api/v1/auth/sessions", org=org)
        check("sesi yang tidak dicabut tetap hidup", code == 200, f"status={code}")

    print("== batas tenant (US-AD05 AC2) ==")
    outsider = Client("AgentDeckProbe/outsider")
    register(outsider, other)
    code, rows_b, _ = outsider.call("GET", "/api/v1/auth/sessions", org=None)
    # outsider tidak punya X-Org-ID; pakai workspace-nya sendiri dari register.
    outsider_org = None
    status_o, body_o, _ = outsider.call("GET", "/api/v1/auth/me")
    if status_o == 200:
        outsider_org = body_o.get("workspaces", [{}])[0].get("id")
    if outsider_org and rows_b:
        victim = rows_b[0]["id"]
        code, _, _ = device_two.call("DELETE", f"/api/v1/auth/sessions/{victim}", org=org)
        check("admin lintas tenant tidak bisa mencabut (404)",
              code == 404, f"status={code}")
        code, _, _ = outsider.call("GET", "/api/v1/auth/sessions", org=outsider_org)
        check("sesi korban lintas tenant masih hidup", code == 200, f"status={code}")

    print("== ganti password (US-AD90 AC1/AC3) ==")
    code, _, _ = device_two.call("POST", "/api/v1/auth/password/change",
                                 {"old_password": "salah", "new_password": "password2"}, org=org)
    check("password lama salah → 401", code == 401, f"status={code}")

    third = Client("AgentDeckProbe/three")
    login(third, email)
    code, _, _ = third.call("POST", "/api/v1/auth/password/change",
                            {"old_password": "password1", "new_password": "password2"}, org=org)
    check("ganti password → 204", code == 204, f"status={code}")
    code, _, _ = third.call("GET", "/api/v1/auth/sessions", org=org)
    check("sesi pemanggil tetap hidup", code == 200, f"status={code}")
    code, _, _ = device_two.call("GET", "/api/v1/auth/sessions", org=org)
    check("sesi lain dicabut (401)", code == 401, f"status={code}")

    fresh = Client()
    login(fresh, email, "password2")
    check("password baru bisa dipakai", fresh.token != "")
    try:
        login(Client(), email, "password1")
        check("password lama ditolak", False, "login berhasil")
    except SystemExit:
        check("password lama ditolak", True)

    print("== tutup akun (US-AD98) ==")
    code, _, _ = fresh.call("DELETE", "/api/v1/auth/me", {"confirm_email": "bukan-email@x.test"}, org=org)
    check("konfirmasi email salah → 400", code == 400, f"status={code}")
    code, _, _ = fresh.call("GET", "/api/v1/auth/me")
    check("akun belum tertutup setelah 400", code == 200, f"status={code}")

    code, _, _ = fresh.call("DELETE", "/api/v1/auth/me", {"confirm_email": email}, org=org)
    check("tutup akun → 202", code == 202, f"status={code}")
    code, _, _ = fresh.call("GET", "/api/v1/auth/sessions", org=org)
    check("semua sesi dicabut (401)", code == 401, f"status={code}")
    try:
        login(Client(), email, "password2")
        check("akun tertutup tidak bisa login", False, "login berhasil")
    except SystemExit:
        check("akun tertutup tidak bisa login", True)

    print()
    print(f"HASIL: {len(PASSED)} hijau, {len(FAILED)} gagal")
    if FAILED:
        for name in FAILED:
            print(f"  GAGAL: {name}")
        sys.exit(1)


if __name__ == "__main__":
    main()
