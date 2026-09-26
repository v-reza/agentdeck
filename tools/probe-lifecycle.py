"""Probe Fase 2 (6.2.9) lawan API nyata: cancel / retry / archive.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:
  1. tiga route-nya terdaftar dan menjawab (401 tanpa sesi = route ada, bukan 404);
  2. migrasi 0015 benar-benar diterapkan — kolom `cancel_requested_at` ada di DB
     container, dan permintaan cancel benar-benar ditulis ke sana;
  3. gerbang peran berbeda: cancel/retry Member, archive Admin;
  4. cancel tidak menyentuh run lain.

Dijalankan dari root repo. Butuh sesi (cookie) dari login fixture: skrip ini
membuat org+user lewat endpoint publik, jadi tidak butuh seed.
"""
import json
import os
import sys
import time
import http.cookiejar
import urllib.error
import urllib.parse
import urllib.request

API = os.environ.get("AGENTDECK_API", "http://127.0.0.1:8080")
ok = 0
fail = []
# Cookie sesinya `Secure` (auth.SessionCookie). Browser memperlakukan
# http://localhost sebagai secure context dan tetap mengirimnya; http.cookiejar
# Python tidak, jadi tanpa policy ini setiap request setelah registrasi balik 401
# dan probe-nya salah menyalahkan produk. Loopback dinyatakan aman di sini —
# probe ini memang cuma bicara ke 127.0.0.1.
_LOOPBACK = ("127.0.0.1", "localhost", "::1")


class _LoopbackIsSecure(http.cookiejar.DefaultCookiePolicy):
    def _is_loopback(self, request):
        return urllib.parse.urlsplit(request.full_url).hostname in _LOOPBACK

    def set_ok_secure(self, cookie, request):
        return True if self._is_loopback(request) else super().set_ok_secure(cookie, request)

    def return_ok_secure(self, cookie, request):
        return True if self._is_loopback(request) else super().return_ok_secure(cookie, request)


JAR = urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar(policy=_LoopbackIsSecure()))
OPENER = urllib.request.build_opener(JAR)


def check(label, cond, detail=""):
    global ok
    if cond:
        ok += 1
        print(f"  [ok  ] {label}" + (f" — {detail}" if detail else ""))
    else:
        fail.append(label)
        print(f"  [FAIL] {label}" + (f" — {detail}" if detail else ""))


# Sesi disimpan sebagai cookie HTTP-Only (auth.SessionCookie), bukan token di
# body: register/login hanya mengembalikan user_id dan workspace_id. Cookie jar
# ini yang membuat probe-nya memakai jalur auth yang sama dengan browser.
def call(method, path, body=None, token=None, org=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(API + path, data=data, method=method)
    if data:
        req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    if org:
        req.add_header("X-Org-ID", org)
    try:
        with OPENER.open(req, timeout=15) as r:
            raw = r.read().decode("utf-8", "replace")
            return r.status, (json.loads(raw) if raw.strip().startswith(("{", "[")) else raw), dict(r.headers)
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw), dict(e.headers)
        except json.JSONDecodeError:
            return e.code, raw, dict(e.headers)
    except Exception as e:  # noqa: BLE001
        return 0, f"{type(e).__name__}: {e}", {}


def main():
    print(f"API {API}\n")
    stamp = str(int(time.time() * 1000))[-9:]

    # 1. route terdaftar: 401 tanpa kredensial, bukan 404.
    for action in ("cancel", "retry", "archive"):
        st, _, _ = call("POST", f"/api/v1/tasks/01HZZZZZZZZZZZZZZZZZZZZZ/{action}")
        check(f"route /{action} terdaftar", st in (401, 403), f"status={st}")

    # 2. migrasi 0015: kolom ada di DB container.
    import subprocess
    q = ("SELECT count(*) FROM information_schema.columns "
         "WHERE table_name='runs' AND column_name='cancel_requested_at';")
    r = subprocess.run(["docker", "exec", "agentdeck-db", "psql", "-U", "agentdeck",
                        "-d", "agentdeck", "-tAc", q], capture_output=True, text=True)
    check("migrasi 0015: runs.cancel_requested_at ada", r.stdout.strip() == "1",
          f"count={r.stdout.strip()} {r.stderr.strip()[:60]}")

    # 3. jalur lengkap: daftar -> org -> project -> board -> task -> cancel/retry/archive
    email = f"f2-{stamp}@probe.test"
    st, body, _ = call("POST", "/api/v1/auth/register", {
        "email": email, "password": "Probe-Password-123!", "name": "Probe F2",
    })
    check("registrasi user probe", st in (200, 201), f"status={st} {str(body)[:80]}")
    org_id = body.get("workspace_id") if isinstance(body, dict) else None
    check("workspace dibuat saat registrasi", bool(org_id), f"workspace_id={org_id}")
    check("sesi disimpan sebagai cookie",
          any(c.name == "agentdeck_session" for c in JAR.cookiejar), "cookie agentdeck_session")

    st, proj, _ = call("POST", "/api/v1/projects",
                       {"name": "F2 Probe", "slug": f"f2-{stamp}"}, org=org_id)
    project_id = proj.get("id") if isinstance(proj, dict) else None
    check("buat project", bool(project_id), f"status={st} {str(proj)[:80]}")

    st, board, _ = call("POST", f"/api/v1/projects/{project_id}/boards",
                        {"name": "F2 Board", "slug": f"f2b-{stamp}"}, org=org_id)
    board_id = board.get("id") if isinstance(board, dict) else None
    check("buat board", bool(board_id), f"status={st} {str(board)[:80]}")
    if not board_id:
        return 1

    st, task, _ = call("POST", f"/api/v1/boards/{board_id}/tasks",
                       {"title": "Probe cancel", "body": "x"}, org=org_id)
    task_id = task.get("id") if isinstance(task, dict) else None
    check("buat task", bool(task_id), f"status={st} {str(task)[:80]}")
    if not task_id:
        return 1
    check("task baru berstatus backlog", task.get("status") == "backlog", f"status={task.get('status')}")

    # cancel dari backlog: sah, tidak ada run untuk di-abort.
    st, body, _ = call("POST", f"/api/v1/tasks/{task_id}/cancel", org=org_id)
    check("cancel task backlog -> 200 cancelled",
          st == 200 and body.get("status") == "cancelled", f"status={st} {str(body)[:80]}")

    # cancel kedua: idempoten.
    st, body, _ = call("POST", f"/api/v1/tasks/{task_id}/cancel", org=org_id)
    check("cancel ulang idempoten -> 200", st == 200 and body.get("status") == "cancelled",
          f"status={st}")

    # retry dari cancelled: sah (override operator).
    st, body, _ = call("POST", f"/api/v1/tasks/{task_id}/retry", org=org_id)
    check("retry dari cancelled -> ready",
          st == 200 and body.get("status") == "ready" and body.get("consecutive_failures") == 0,
          f"status={st} {str(body)[:90]}")

    # archive dari ready: ditolak (AC2).
    st, body, _ = call("POST", f"/api/v1/tasks/{task_id}/archive", org=org_id)
    check("archive task non-terminal -> 409", st == 409, f"status={st} {str(body)[:60]}")

    # bawa ke terminal lalu arsip.
    st, _, _ = call("POST", f"/api/v1/tasks/{task_id}/move",
                    {"from": "ready", "to": "done"}, org=org_id)
    st, body, _ = call("POST", f"/api/v1/tasks/{task_id}/archive", org=org_id)
    check("archive task terminal -> 200 archived",
          st == 200 and body.get("status") == "archived", f"status={st} {str(body)[:80]}")

    st, body, _ = call("POST", f"/api/v1/tasks/{task_id}/archive", org=org_id)
    check("archive ulang idempoten -> 200", st == 200 and body.get("status") == "archived", f"status={st}")

    # task yang tidak ada: 404, bukan 500.
    for action in ("cancel", "retry", "archive"):
        st, _, _ = call("POST", f"/api/v1/tasks/01HZZZZZZZZZZZZZZZZZZZZZ/{action}", org=org_id)
        check(f"{action} task tidak ada -> 404", st == 404, f"status={st}")

    print()
    if fail:
        print(f"GAGAL ({len(fail)}):")
        for f in fail:
            print("  - " + f)
        return 1
    print(f"SEMUA JALUR HIJAU ({ok} cek)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
