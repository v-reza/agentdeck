"""Probe Fase 10 (6.2.3) lawan API nyata: API keys.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:

  1. kelima route-nya terdaftar (401 tanpa sesi = route ada, bukan 404);
  2. gerbang peran: Viewer ditolak (Role Min = Member) — penting karena key
     mewarisi peran pemiliknya;
  3. plaintext `adk_...` muncul di 201 dan **tidak pernah lagi** — tidak di
     daftar, tidak di detail. Ini diperiksa dengan mencari string-nya, bukan
     dengan memeriksa field;
  4. **bearer `adk_...` benar-benar mengautentikasi** tanpa X-Org-ID, dan
     mewarisi peran pemiliknya (2291);
  5. revoke langsung mematikan key, dan idempoten;
  6. key milik anggota lain 404, bukan 403.

Jalankan setelah `docker compose up -d api`:
    python tools/probe-f10.py
"""

import http.cookiejar
import json
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"
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

    def call(self, method, path, body=None, org=None, bearer=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(BASE + path, data=data, method=method)
        if data is not None:
            req.add_header("Content-Type", "application/json")
        token = bearer if bearer is not None else self.token
        if token:
            req.add_header("Authorization", "Bearer " + token)
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


print("== system/info sebagai penanda build ==")
status, info, _ = Client().call("GET", "/api/v1/system/info")
check("system/info = 200", status == 200, status)

owner, viewer, stranger = Client(), Client(), Client()
owner_email = f"f10owner{STAMP}@x.test"
viewer_email = f"f10viewer{STAMP}@x.test"
stranger_email = f"f10stranger{STAMP}@x.test"
org = register(owner, owner_email)
register(viewer, viewer_email)
stranger_org = register(stranger, stranger_email)
login(owner, owner_email)
login(viewer, viewer_email)
login(stranger, stranger_email)

# Viewer harus benar-benar anggota org yang sama supaya 403-nya dari gerbang
# peran, bukan dari "bukan anggota".
status, _, text = owner.call("POST", f"/api/v1/orgs/{org}/members",
                             {"email": viewer_email, "role": "viewer"}, org=org)
check("owner menambah viewer", status in (200, 201), f"{status} {text}")

print()
print("== route terdaftar (401 tanpa sesi) ==")
for method, path in [("GET", "/api/v1/api-keys"),
                     ("POST", "/api/v1/api-keys"),
                     ("GET", "/api/v1/api-keys/01ABC"),
                     ("DELETE", "/api/v1/api-keys/01ABC"),
                     ("POST", "/api/v1/api-keys/01ABC/revoke")]:
    status, _, _ = Client().call(method, path, body={} if method == "POST" else None, org=org)
    check(f"{method} {path} = 401 (bukan 404)", status == 401, status)

print()
print("== gerbang peran ==")
status, _, _ = viewer.call("GET", "/api/v1/api-keys", org=org)
check("viewer ditolak daftar key = 403", status == 403, status)
status, _, _ = viewer.call("POST", "/api/v1/api-keys", {"name": "nope"}, org=org)
check("viewer ditolak membuat key = 403", status == 403, status)

print()
print("== membuat key ==")
status, created, text = owner.call("POST", "/api/v1/api-keys", {"name": "probe-ci"}, org=org)
check("POST /api-keys = 201", status == 201, f"{status} {text}")
if status != 201:
    print(f"\n=== HASIL: {len(PASSED)} ok / {len(FAILED)} gagal ===")
    sys.exit(1)
key_id = created["id"]
plaintext = created["key"]
check("plaintext berawalan adk_", plaintext.startswith("adk_"), plaintext[:12])
check("prefix = 8 karakter pertama", created["prefix"] == plaintext[:8], created["prefix"])
check("revoked_at null saat dibuat", created["revoked_at"] is None, created["revoked_at"])
check("last_used_at null saat dibuat", created["last_used_at"] is None, created["last_used_at"])

status, _, text = owner.call("POST", "/api/v1/api-keys", {"name": "  "}, org=org)
check("nama kosong = 400", status == 400, status)

print()
print("== plaintext tidak pernah muncul lagi ==")
status, _, detail_text = owner.call("GET", f"/api/v1/api-keys/{key_id}", org=org)
check("GET /api-keys/{id} = 200", status == 200, status)
check("detail tidak memuat plaintext", plaintext not in detail_text, "bocor!")
status, _, list_text = owner.call("GET", "/api/v1/api-keys", org=org)
check("daftar = 200", status == 200, status)
check("daftar tidak memuat plaintext", plaintext not in list_text, "bocor!")
check("daftar tidak memuat token_hash", "token_hash" not in list_text, "bocor!")
check("daftar memuat key kita", key_id in list_text, list_text[:120])

print()
print("== bearer adk_ mengautentikasi (tanpa X-Org-ID) ==")
status, _, text = Client().call("GET", "/api/v1/api-keys", bearer=plaintext)
check("bearer tanpa X-Org-ID = 200", status == 200, f"{status} {text}")
# Peran diwarisi: owner bisa membaca audit log (butuh Admin).
status, _, _ = Client().call("GET", "/api/v1/audit-log", bearer=plaintext)
check("key owner lolos audit-log = 200 (peran diwarisi)", status == 200, status)

print()
print("== isolasi: key milik anggota lain ==")
status, _, _ = stranger.call("GET", f"/api/v1/api-keys/{key_id}", org=org)
check("bukan anggota = 403", status in (403, 404), status)
status, _, _ = stranger.call("GET", f"/api/v1/api-keys/{key_id}", org=stranger_org)
check("anggota org lain = 404", status == 404, status)

print()
print("== last_used_at terisi setelah dipakai ==")
time.sleep(0.2)
status, detail, _ = owner.call("GET", f"/api/v1/api-keys/{key_id}", org=org)
check("last_used_at terisi", status == 200 and detail.get("last_used_at"), detail)

print()
print("== revoke ==")
status, _, text = owner.call("POST", f"/api/v1/api-keys/{key_id}/revoke", org=org)
check("revoke pertama = 200", status == 200, f"{status} {text}")
status, _, _ = Client().call("GET", "/api/v1/api-keys", bearer=plaintext)
check("key dicabut ditolak = 401", status == 401, status)
status, _, _ = owner.call("POST", f"/api/v1/api-keys/{key_id}/revoke", org=org)
check("revoke kedua = 200 (idempoten)", status == 200, status)

print()
print("== hapus ==")
status, temp_key, _ = owner.call("POST", "/api/v1/api-keys", {"name": "probe-temp"}, org=org)
check("key kedua dibuat", status == 201, status)
status, _, _ = owner.call("DELETE", f"/api/v1/api-keys/{temp_key['id']}", org=org)
check("DELETE = 204", status == 204, status)
status, _, _ = Client().call("GET", "/api/v1/api-keys", bearer=temp_key["key"])
check("key terhapus ditolak = 401", status == 401, status)
status, _, _ = owner.call("GET", f"/api/v1/api-keys/{temp_key['id']}", org=org)
check("detail setelah hapus = 404", status == 404, status)

print()
print(f"=== HASIL: {len(PASSED)} ok / {len(FAILED)} gagal ===")
if FAILED:
    for name in FAILED:
        print(f"  GAGAL: {name}")
    sys.exit(1)
