"""Probe Fase 9 (6.2.19) lawan API nyata: audit log, notifikasi, search, system info.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:

  1. keenam route-nya terdaftar (401/404 yang benar, bukan 404 halaman kosong);
  2. `/system/info` publik dan tipis — tidak membocorkan apa pun soal
     infrastruktur, dan `commit`-nya bisa dipakai untuk membuktikan container
     yang jalan memang build terbaru;
  3. gerbang peran: viewer ditolak dari audit log (Admin), tapi boleh search dan
     boleh membaca inbox-nya sendiri (Viewer);
  4. `/audit-log` benar-benar menolak filter yang tidak bisa di-parse dengan 400 —
     bukan 200 dengan hasil kosong, yang akan terbaca sebagai "tidak ada
     aktivitas" dan bukan "permintaanmu salah";
  5. search menolak `q` kosong dengan 400, dan menemukan task lewat kata di
     tengah judul (yang membedakan ILIKE dari pencarian berbasis awalan);
  6. inbox hanya berisi notifikasi pemanggilnya, dan `POST /notifications/read`
     tanpa `ids`/`all` = 400 (bukan "tandai semua").

Jalankan setelah `docker compose up -d api`:
    python tools/probe-comments.py   # pola helper yang sama
    python tools/probe-f9.py
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


def register(client, email, org_name="Probe F9"):
    status, body, _ = client.call("POST", "/api/v1/auth/register",
                                  {"email": email, "password": "password1",
                                   "name": "Probe", "org_name": org_name})
    if status != 201:
        raise SystemExit(f"register {email}: {status} {body}")
    client.token = parse_cookie(client)
    return body["workspace_id"]


def login(client, email):
    status, _, _ = client.call("POST", "/api/v1/auth/login",
                               {"email": email, "password": "password1"})
    if status != 200:
        raise SystemExit(f"login {email}: {status}")
    client.token = parse_cookie(client)


def main():
    print("== system/info (publik, tipis) ==")
    anon = Client()
    status, body, text = anon.call("GET", "/api/v1/system/info")
    check("GET /system/info tanpa sesi = 200", status == 200, f"{status} {text[:120]}")
    if isinstance(body, dict):
        check("punya version", "version" in body, body)
        check("punya commit", "commit" in body, body)
        check("punya go_version", "go_version" in body, body)
        check("punya started_at", "started_at" in body, body)
        # Kebocoran yang harus dicegah: apa pun yang menamai infrastruktur.
        low = text.lower()
        leaks = [k for k in ("postgres", "postgresql://", "agentdeck_", "password",
                             "secret", "dsn", "database") if k in low]
        check("tidak membocorkan infrastruktur", not leaks, leaks)
    else:
        check("body berupa objek JSON", False, text[:120])

    print("\n== gerbang peran ==")
    owner = Client()
    org = register(owner, f"f9-owner-{STAMP}@x.test")
    check("owner bisa baca audit log = 200",
          owner.call("GET", "/api/v1/audit-log", org=org)[0] == 200)

    # Viewer: anggota baru di org yang sama.
    viewer_email = f"f9-viewer-{STAMP}@x.test"
    viewer = Client()
    register(viewer, viewer_email, "Probe F9 Viewer")
    add = owner.call("POST", f"/api/v1/orgs/{org}/members",
                     {"email": viewer_email, "role": "viewer"}, org=org)
    check("owner menambah viewer", add[0] in (200, 201), f"{add[0]} {add[2][:120]}")
    login(viewer, viewer_email)
    viewer_org = org  # viewer diundang ke org owner

    st, _, _ = viewer.call("GET", "/api/v1/audit-log", org=viewer_org)
    check("viewer ditolak dari audit log = 403", st == 403, st)
    st, _, _ = viewer.call("GET", "/api/v1/notifications", org=viewer_org)
    check("viewer boleh baca inbox sendiri = 200", st == 200, st)
    st, _, _ = viewer.call("GET", f"/api/v1/search/tasks?q=apa", org=viewer_org)
    check("viewer boleh search = 200", st == 200, st)

    print("\n== audit-log: filter tidak sah = 400 ==")
    for query in ("?cursor=abc", "?cursor=-1", "?from=2026-09", "?to=bukan-tanggal", "?limit=abc"):
        st, _, _ = owner.call("GET", "/api/v1/audit-log" + query, org=org)
        check(f"audit-log{query} = 400", st == 400, st)
    st, _, _ = owner.call("GET", "/api/v1/audit-log?limit=5&from=2020-01-01T00:00:00Z", org=org)
    check("filter sah = 200", st == 200, st)

    print("\n== audit-log: isi ==")
    st, body, text = owner.call("GET", "/api/v1/audit-log?limit=50", org=org)
    if st == 200 and isinstance(body, dict):
        entries = body.get("entries") or []
        check("entries berupa list", isinstance(entries, list), type(entries).__name__)
        check("setiap entri punya id (dipakai sebagai cursor)",
              all("id" in e for e in entries), entries[:1])
        check("setiap entri punya created_at",
              all("created_at" in e for e in entries), entries[:1])
        # Paginasi: halaman kedua pakai cursor dari halaman pertama.
        if entries:
            cursor = entries[-1]["id"]
            st2, body2, _ = owner.call("GET", f"/api/v1/audit-log?cursor={cursor}&limit=5", org=org)
            check("cursor dari halaman 1 diterima", st2 == 200, st2)
            if st2 == 200 and isinstance(body2, dict):
                ids1 = {e["id"] for e in entries}
                ids2 = {e["id"] for e in (body2.get("entries") or [])}
                check("cursor eksklusif (tidak ada id di dua halaman)",
                      not (ids1 & ids2), ids1 & ids2)
    else:
        check("audit-log mengembalikan objek", False, f"{st} {text[:120]}")

    print("\n== notifications ==")
    st, body, text = owner.call("GET", "/api/v1/notifications", org=org)
    check("GET /notifications = 200", st == 200, f"{st} {text[:120]}")
    if isinstance(body, dict):
        check("punya notifications", "notifications" in body, list(body))
        check("punya unread_count", "unread_count" in body, list(body))
    st, _, _ = owner.call("POST", "/api/v1/notifications/read", {}, org=org)
    check("read tanpa ids/all = 400", st == 400, st)
    st, _, _ = owner.call("POST", "/api/v1/notifications/read", {"all": True}, org=org)
    check("read {all:true} = 200", st == 200, st)
    st, _, _ = owner.call("POST", "/api/v1/notifications/read", {"ids": []}, org=org)
    check("read {ids:[]} = 400 (bukan 'tandai semua')", st == 400, st)

    print("\n== search ==")
    st, _, _ = owner.call("GET", "/api/v1/search/tasks", org=org)
    check("search tanpa q = 400", st == 400, st)
    st, _, _ = owner.call("GET", "/api/v1/search/tasks?q=%20%20", org=org)
    check("search q hanya spasi = 400", st == 400, st)
    st, _, _ = owner.call("GET", "/api/v1/search/runs", org=org)
    check("search runs tanpa filter = 400", st == 400, st)

    # Buat task dengan kata yang khas, lalu cari kata di TENGAH judulnya.
    marker = "zebra" + STAMP
    # Board tinggal di bawah project (US-AD09 AC4 membuat satu project saat
    # workspace dibuka), jadi id project dibaca dulu — bukan `/boards` langsung.
    st, projects, _ = owner.call("GET", "/api/v1/projects", org=org)
    board_id = None
    project_id = None
    # Perhatikan: `/projects` mengembalikan array telanjang, sementara
    # `/projects/{id}/boards` mengembalikan objek ber-`boards`. Keduanya dibaca
    # apa adanya, bukan diasumsikan seragam.
    items = projects if isinstance(projects, list) else (projects or {}).get("projects") or []
    if st == 200 and items:
        project_id = items[0]["id"]
    if project_id:
        st, board_body, _ = owner.call("GET", f"/api/v1/projects/{project_id}/boards", org=org)
        boards = board_body.get("boards") if isinstance(board_body, dict) else board_body
        if st == 200 and boards:
            board_id = boards[0]["id"]
        else:
            # US-AD09 AC4 membuat PROJECT saat workspace dibuka, bukan board.
            # Jadi board-nya dibuat di sini — kalau tidak, uji search di bawah
            # tidak pernah punya apa pun untuk dicari dan akan lulus palsu.
            st, created, _ = owner.call("POST", f"/api/v1/projects/{project_id}/boards",
                                        {"slug": f"f9-{STAMP}", "name": "Probe F9"}, org=org)
            if st in (200, 201) and isinstance(created, dict):
                board_id = created.get("id")
    if board_id:
        st, task, _ = owner.call("POST", f"/api/v1/boards/{board_id}/tasks",
                                 {"title": f"Perbaiki pipeline {marker} staging", "status": "backlog"},
                                 org=org)
        check("task dibuat", st == 201, f"{st} {str(task)[:120]}")
        if st == 201:
            st, found, text = owner.call("GET", f"/api/v1/search/tasks?q={marker}", org=org)
            check("search = 200", st == 200, f"{st} {text[:120]}")
            if st == 200 and isinstance(found, dict):
                titles = [t.get("title", "") for t in (found.get("tasks") or [])]
                check("menemukan task lewat kata di tengah judul",
                      any(marker in t for t in titles), titles[:3])
            # Sebagian kata juga harus cocok.
            st, found, _ = owner.call("GET", f"/api/v1/search/tasks?q={marker[2:]}", org=org)
            partial = found.get("tasks") if isinstance(found, dict) else None
            check("cocok juga untuk sebagian kata",
                  bool(partial), partial if partial is not None else "bukan list")
    else:
        check("org punya project + board (untuk uji search)", False, f"projects={st}")

    print("\n== isolasi tenant ==")
    stranger = Client()
    stranger_org = register(stranger, f"f9-stranger-{STAMP}@x.test", "Probe F9 Stranger")
    st, body, _ = stranger.call("GET", f"/api/v1/search/tasks?q={marker}", org=stranger_org)
    if st == 200 and isinstance(body, dict):
        tasks = body.get("tasks") or []
        check("org lain tidak melihat task org pertama", len(tasks) == 0, tasks[:2])
    else:
        check("search org lain = 200", st == 200, st)
    st, body, _ = stranger.call("GET", "/api/v1/notifications", org=stranger_org)
    if st == 200 and isinstance(body, dict):
        check("inbox org lain kosong", body.get("unread_count", -1) == 0, body)

    print(f"\n=== HASIL: {len(PASSED)} ok / {len(FAILED)} gagal ===")
    for name in FAILED:
        print(f"  GAGAL: {name}")
    return 1 if FAILED else 0


if __name__ == "__main__":
    sys.exit(main())
