"""Probe Fase 8 (6.2.17) lawan API nyata: komentar task.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:
  1. empat route-nya terdaftar (401 tanpa sesi = route ada, bukan 404);
  2. gerbang peran: viewer membaca, tidak menulis (US-AD42 AC2);
  3. batas panjang diukur dalam rune, bukan byte — body 4096 karakter non-ASCII
     (8192 byte) diterima, 4097 karakter ditolak (AC3);
  4. kepemilikan komentar ditegakkan lawan DB nyata, bukan cuma di unit test;
  5. isolasi tenant: task dan komentar org lain sama-sama 404.

Catatan cakupan: event `comment.created` TIDAK diperiksa di sini karena belum
ada route HTTP untuk membaca timeline task (itu 6.2.13, masih ⬜). Event-nya
dibuktikan lawan Postgres di internal/board/comment_test.go.

Jalankan: python tools/probe-comments.py
"""

import http.cookiejar
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = os.environ.get("AGENTDECK_BASE", "http://127.0.0.1:8080")
PASSWORD = "password1"
PASSED, FAILED = [], []


def check(label, ok, detail=""):
    if ok:
        PASSED.append(label)
        print(f"  [ok] {label}")
    else:
        FAILED.append(label)
        print(f"  [GAGAL] {label}{' — ' + detail if detail else ''}")


class RelaxedPolicy(http.cookiejar.DefaultCookiePolicy):
    """Cookie sesi tidak boleh ditolak karena flag Secure: bridge-nya HTTP."""

    def return_ok_secure(self, cookie, request):
        return True


class Client:
    def __init__(self):
        self.jar = http.cookiejar.CookieJar()
        self.jar.set_policy(RelaxedPolicy())
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar))
        self.token = ""

    def call(self, method, path, body=None, org=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(BASE + path, data=data, method=method)
        req.add_header("Accept", "application/json")
        if data is not None:
            req.add_header("Content-Type", "application/json")
        # Sesi dikirim sebagai Bearer, bukan hanya cookie: cookie jar tidak
        # selalu diteruskan, dan tanpa header ini setiap panggilan jadi 401.
        if self.token:
            req.add_header("Authorization", "Bearer " + self.token)
        if org:
            req.add_header("X-Org-ID", org)
        try:
            with self.opener.open(req, timeout=20) as resp:
                text = resp.read().decode("utf-8", "replace")
                return resp.status, (json.loads(text) if text.strip() else None), text
        except urllib.error.HTTPError as err:
            text = err.read().decode("utf-8", "replace")
            try:
                parsed = json.loads(text) if text.strip() else None
            except json.JSONDecodeError:
                parsed = text
            return err.code, parsed, text


def register(client, email, org_name="Probe Comments"):
    status, body, text = client.call("POST", "/api/v1/auth/register",
                                     {"email": email, "password": PASSWORD,
                                      "name": "Probe", "org_name": org_name})
    if status != 201:
        raise SystemExit(f"register {email}: {status} {text}")
    client.token = session_token(client)
    return body["workspace_id"]


def session_token(client):
    for cookie in client.jar:
        if cookie.name == "agentdeck_session":
            return cookie.value
    return ""


def login(client, email):
    status, _, text = client.call("POST", "/api/v1/auth/login",
                                  {"email": email, "password": PASSWORD})
    if status != 200:
        raise SystemExit(f"login {email}: {status} {text}")
    client.token = session_token(client)
    if not client.token:
        raise SystemExit(f"login {email}: cookie sesi tidak ada")


def main():
    suffix = str(int(time.time()))

    print("== route terdaftar (401 tanpa sesi, bukan 404) ==")
    anon = Client()
    for method, path, body in [
        ("GET", "/api/v1/tasks/01ABC/comments", None),
        ("POST", "/api/v1/tasks/01ABC/comments", {"body": "x"}),
        ("PATCH", "/api/v1/comments/1", {"body": "x"}),
        ("DELETE", "/api/v1/comments/1", None),
    ]:
        status, _, _ = anon.call(method, path, body)
        check(f"{method} {path} menolak tanpa sesi", status == 401, f"status={status}")

    print("== siapkan org, anggota, board, task ==")
    owner = Client()
    org = register(owner, f"probe.cm.owner.{suffix}@example.com")

    for role in ("member", "viewer"):
        email = f"probe.cm.{role}.{suffix}@example.com"
        register(Client(), email, f"Probe {role}")
        status, _, text = owner.call("POST", f"/api/v1/orgs/{org}/members",
                                     {"email": email, "role": role})
        if status not in (200, 201):
            raise SystemExit(f"invite {role}: {status} {text}")

    member, viewer, outsider = Client(), Client(), Client()
    login(member, f"probe.cm.member.{suffix}@example.com")
    login(viewer, f"probe.cm.viewer.{suffix}@example.com")
    # Workspace sendiri, terpisah dari `org`.
    out_org = register(outsider, f"probe.cm.out.{suffix}@example.com")

    status, project, text = owner.call("POST", "/api/v1/projects",
                                       {"name": "Probe Comments",
                                        "slug": f"probe-cm-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"project: {status} {text}")
    status, board, text = owner.call("POST", f"/api/v1/projects/{project['id']}/boards",
                                     {"name": "Probe Board",
                                      "slug": f"probe-cmb-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"board: {status} {text}")
    status, task, text = owner.call("POST", f"/api/v1/boards/{board['id']}/tasks",
                                    {"title": "Task untuk komentar",
                                     "status": "backlog"}, org=org)
    if status != 201:
        raise SystemExit(f"task: {status} {text}")
    task_id = task["id"]

    print("== US-AD42 AC2: viewer membaca, tidak menulis ==")
    status, _, _ = viewer.call("POST", f"/api/v1/tasks/{task_id}/comments",
                               {"body": "dari viewer"}, org=org)
    check("viewer menulis komentar → 403", status == 403, f"status={status}")

    status, first, text = member.call("POST", f"/api/v1/tasks/{task_id}/comments",
                                      {"body": "dari member"}, org=org)
    check("member menulis komentar → 201", status == 201, f"status={status} {text[:120]}")
    if status != 201:
        print("HASIL: tidak bisa lanjut tanpa komentar pertama")
        sys.exit(1)
    check("author_user_id terisi", bool(first.get("author_user_id")), str(first)[:140])
    check("author_agent_id kosong", not first.get("author_agent_id"), str(first)[:140])

    status, body, text = viewer.call("GET", f"/api/v1/tasks/{task_id}/comments", org=org)
    check("viewer membaca daftar komentar → 200", status == 200, f"status={status}")
    if status == 200:
        listed = body["comments"]
        check("komentar pertama ada di daftar",
              any(c["id"] == first["id"] for c in listed), str(listed)[:140])

    print("== US-AD42 AC3: batas panjang ==")
    for label, payload in [("kosong", ""), ("spasi saja", "   "),
                           ("4097 karakter", "x" * 4097)]:
        status, _, _ = member.call("POST", f"/api/v1/tasks/{task_id}/comments",
                                   {"body": payload}, org=org)
        check(f"body {label} → 400", status == 400, f"status={status}")
    status, _, text = member.call("POST", f"/api/v1/tasks/{task_id}/comments",
                                  {"body": "x" * 4096}, org=org)
    check("body 4096 karakter → 201", status == 201, f"status={status} {text[:100]}")
    # Rune, bukan byte: 'é' itu 2 byte, jadi 4096 rune = 8192 byte. Kalau batasnya
    # dihitung byte, ini ditolak dan pesan errornya tidak menjelaskan kenapa.
    status, _, text = member.call("POST", f"/api/v1/tasks/{task_id}/comments",
                                  {"body": "é" * 4096}, org=org)
    check("body 4096 rune non-ASCII (8192 byte) → 201", status == 201,
          f"status={status} {text[:100]}")

    print("== kepemilikan: hanya penulis yang boleh mengubah ==")
    status, _, _ = owner.call("PATCH", f"/api/v1/comments/{first['id']}",
                              {"body": "dibajak owner"}, org=org)
    check("bukan penulis mengedit → 404", status == 404, f"status={status}")
    status, _, _ = owner.call("DELETE", f"/api/v1/comments/{first['id']}", org=org)
    check("bukan penulis menghapus → 404", status == 404, f"status={status}")
    status, edited, text = member.call("PATCH", f"/api/v1/comments/{first['id']}",
                                       {"body": "diubah penulis"}, org=org)
    check("penulis mengedit → 200", status == 200, f"status={status} {text[:100]}")
    if status == 200:
        check("body benar-benar berubah", edited.get("body") == "diubah penulis")

    print("== id non-numerik → 400, bukan 500 ==")
    status, _, _ = member.call("DELETE", "/api/v1/comments/abc", org=org)
    check("id non-numerik → 400", status == 400, f"status={status}")

    print("== isolasi tenant ==")
    # Bukan anggota workspace ini sama sekali: middleware menolak lebih dulu, dan
    # 403 lebih ketat daripada 404 — id-nya tidak pernah sampai ke handler.
    status, _, _ = outsider.call("GET", f"/api/v1/tasks/{task_id}/comments", org=org)
    check("bukan anggota → 403 (ditolak sebelum handler)", status == 403, f"status={status}")

    # Anggota workspace LAIN, mengakses id milik workspace ini lewat org-nya
    # sendiri. Ini yang mengukur scoping org di SQL: 404, bukan 403 dan bukan
    # daftar kosong — daftar kosong akan mengonfirmasi id-nya ada di suatu tempat.
    status, _, _ = outsider.call("GET", f"/api/v1/tasks/{task_id}/comments", org=out_org)
    check("task tenant lain → 404", status == 404, f"status={status}")
    status, _, _ = outsider.call("PATCH", f"/api/v1/comments/{first['id']}",
                                 {"body": "x"}, org=out_org)
    check("komentar tenant lain → 404", status == 404, f"status={status}")

    print("== hapus oleh penulis ==")
    status, _, _ = member.call("DELETE", f"/api/v1/comments/{first['id']}", org=org)
    check("penulis menghapus → 204", status == 204, f"status={status}")
    status, _, _ = member.call("DELETE", f"/api/v1/comments/{first['id']}", org=org)
    check("hapus dua kali → 404", status == 404, f"status={status}")

    print()
    print(f"HASIL: {len(PASSED)} hijau, {len(FAILED)} gagal")
    for label in FAILED:
        print(f"  GAGAL: {label}")
    sys.exit(1 if FAILED else 0)


if __name__ == "__main__":
    main()
