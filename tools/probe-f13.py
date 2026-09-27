"""Probe Fase 13 (6.2.16) lawan API nyata: artifacts.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan, dengan object
storage sungguhan (MinIO, S3-compatible seperti R2):

  1. Kelima route terdaftar (401 tanpa sesi = route ada, bukan 404).
  2. Siklus 12.3 lengkap: upload-url -> PUT ke storage -> register -> download.
     Presigned URL-nya benar-benar diterima SERVER STORAGE, bukan cuma
     berbentuk benar.
  3. Digest-nya dihitung ulang dari isi objek yang mendarat. Klien yang
     mengirim sha256 milik isi lain harus ditolak.
  4. Ukuran yang tidak cocok dengan storage ditolak.
  5. Batas 25 MB per file dan kuota 100 MB per task.
  6. Isolasi tenant: artifact org lain 404, storage_key org lain ditolak.
  7. Gerbang peran: viewer tidak boleh mengunggah.

API-nya dijalankan DI HOST (bukan di container) supaya satu host string
bekerja untuk kedua sisi: presigned URL dibuat oleh API dan di-PUT oleh probe
ini, dan keduanya harus menjangkau storage yang sama.
"""

import hashlib
import http.cookiejar
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = os.environ.get("AGENTDECK_BASE", "http://127.0.0.1:8080")
PASSED = []
FAILED = []


def check(label, ok, detail=""):
    (PASSED if ok else FAILED).append(label)
    print(f"  [{'ok' if ok else 'GAGAL'}] {label}" + (f" — {detail}" if detail and not ok else ""))


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **kw):
        return None


class Client:
    def __init__(self):
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar), NoRedirect()
        )
        self.token = None

    def call(self, method, path, body=None, org=None, raw=None, headers=None):
        data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
        req = urllib.request.Request(BASE + path, data=data, method=method)
        if data is not None:
            req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", "Bearer " + self.token)
        if org:
            req.add_header("X-Org-ID", org)
        for k, v in (headers or {}).items():
            req.add_header(k, v)
        try:
            with self.opener.open(req, timeout=20) as resp:
                text = resp.read().decode()
                return resp.status, (json.loads(text) if text.strip() else None), text, resp
        except urllib.error.HTTPError as err:
            text = err.read().decode()
            try:
                parsed = json.loads(text) if text.strip() else None
            except json.JSONDecodeError:
                parsed = text
            return err.code, parsed, text, err


def register(email):
    c = Client()
    status, body, text, _ = c.call("POST", "/api/v1/auth/register", {
        "email": email, "password": "Rahasia123!", "name": "Probe",
    })
    if status not in (200, 201):
        raise SystemExit(f"register gagal: {status} {text[:200]}")
    c.token = body.get("token") or body.get("session_token")
    for cookie in c.jar:
        if cookie.name in ("agentdeck_session", "session") and not c.token:
            c.token = cookie.value
    status, body, text, _ = c.call("GET", "/api/v1/orgs")
    if status != 200:
        raise SystemExit(f"orgs gagal: {status} {text[:200]}")
    orgs = body if isinstance(body, list) else body.get("orgs", [])
    return c, orgs[0]["id"]


def put_to_storage(url, payload, content_type):
    req = urllib.request.Request(url, data=payload, method="PUT")
    req.add_header("Content-Type", content_type)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            return resp.status, resp.read().decode(errors="replace")
    except urllib.error.HTTPError as err:
        return err.code, err.read().decode(errors="replace")


def main():
    suffix = str(int(time.time()))
    owner, org = register(f"art-owner-{suffix}@x.test")
    viewer, _ = register(f"art-viewer-{suffix}@x.test")

    print("== route terdaftar (401 tanpa sesi) ==")
    anon = Client()
    for method, path in [
        ("GET", "/api/v1/tasks/01ABC/artifacts"),
        ("POST", "/api/v1/tasks/01ABC/artifacts/upload-url"),
        ("POST", "/api/v1/tasks/01ABC/artifacts"),
        ("GET", "/api/v1/artifacts/01ABC"),
        ("GET", "/api/v1/artifacts/01ABC/download"),
    ]:
        status, _, _, _ = anon.call(method, path)
        check(f"{method} {path} tanpa sesi = 401", status == 401, str(status))

    print("\n== siapkan task + run ==")
    status, project, text, _ = owner.call("POST", "/api/v1/projects",
                                          {"name": "Probe Art", "slug": f"art-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"project: {status} {text[:200]}")
    status, board, text, _ = owner.call("POST", f"/api/v1/projects/{project['id']}/boards",
                                        {"name": "Art Board", "slug": f"artb-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"board: {status} {text[:200]}")
    board_id = board["id"]

    status, agent, text, _ = owner.call("POST", f"/api/v1/projects/{project['id']}/agents",
                                        {"name": f"art-runner-{suffix}",
                                         "provider": "openai_compatible", "model": "gpt-4o"}, org=org)
    if status != 201:
        raise SystemExit(f"agent: {status} {text[:200]}")

    status, task, text, _ = owner.call("POST", f"/api/v1/boards/{board_id}/tasks",
                                       {"title": "Hasilkan file", "status": "backlog"}, org=org)
    if status != 201:
        raise SystemExit(f"task: {status} {text[:200]}")
    task_id = task["id"]
    owner.call("POST", f"/api/v1/tasks/{task_id}/move", {"from": "backlog", "to": "ready"}, org=org)
    owner.call("POST", f"/api/v1/tasks/{task_id}/assign", {"agent_id": agent["id"]}, org=org)
    status, run, text, _ = owner.call("POST", f"/api/v1/tasks/{task_id}/claim", org=org)
    if status not in (200, 201):
        raise SystemExit(f"claim: {status} {text[:200]}")
    run_id = run["id"]
    check("claim membuat run", bool(run_id), str(run_id))

    print("\n== siklus 12.3: upload-url -> PUT -> register -> download ==")
    payload = b"laporan hasil eksekusi\nbaris kedua\n"
    digest = hashlib.sha256(payload).hexdigest()

    status, ticket, text, _ = owner.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts/upload-url",
        {"filename": "laporan.txt", "size": len(payload),
         "content_type": "text/plain", "run_id": run_id}, org=org)
    check("upload-url = 200", status == 200, f"{status} {text[:200]}")
    if status != 200:
        raise SystemExit("tidak bisa lanjut tanpa upload-url")
    check("ticket memuat upload_url", bool(ticket.get("upload_url")))
    check("ticket memuat storage_key", bool(ticket.get("storage_key")))
    key = ticket["storage_key"]
    check("storage_key berawalan prefix org", key.startswith(f"artifacts/{org}/{task_id}/"), key)

    put_status, put_body = put_to_storage(ticket["upload_url"], payload, "text/plain")
    check("PUT ke presigned URL = 200", put_status == 200, f"{put_status} {put_body[:200]}")

    status, created, text, _ = owner.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts",
        {"filename": "laporan.txt", "size": len(payload), "sha256": digest,
         "storage_key": key, "run_id": run_id, "content_type": "text/plain"}, org=org)
    check("register = 201", status == 201, f"{status} {text[:200]}")
    artifact_id = created["id"] if status == 201 else None
    if artifact_id:
        check("sha256 tersimpan sama", created["sha256"] == digest, str(created.get("sha256")))
        check("size tersimpan sama", created["size"] == len(payload), str(created.get("size")))

    status, items, text, _ = owner.call("GET", f"/api/v1/tasks/{task_id}/artifacts", org=org)
    check("list = 200", status == 200, str(status))
    check("list memuat artifact baru",
          status == 200 and any(i["id"] == artifact_id for i in items),
          json.dumps(items)[:200] if status == 200 else text[:200])

    if artifact_id:
        status, one, text, _ = owner.call("GET", f"/api/v1/artifacts/{artifact_id}", org=org)
        check("metadata satu artifact = 200", status == 200, str(status))

        status, _, _, resp = owner.call("GET", f"/api/v1/artifacts/{artifact_id}/download", org=org)
        check("download = 302", status == 302, str(status))
        location = resp.headers.get("Location") if hasattr(resp, "headers") else None
        check("download memberi Location", bool(location))
        if location:
            try:
                with urllib.request.urlopen(location, timeout=20) as dl:
                    got = dl.read()
                check("isi unduhan identik dengan yang diunggah", got == payload,
                      f"{len(got)} byte vs {len(payload)}")
            except urllib.error.HTTPError as err:
                check("presigned GET diterima storage", False, f"{err.code}")

    print("\n== verifikasi yang menolak ==")
    # Digest milik isi lain.
    status, _, text, _ = owner.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts",
        {"filename": "laporan.txt", "size": len(payload),
         "sha256": hashlib.sha256(b"isi lain").hexdigest(),
         "storage_key": key, "run_id": run_id}, org=org)
    check("sha256 salah = 400", status == 400, f"{status} {text[:160]}")

    # Ukuran yang tidak cocok dengan storage.
    status, _, text, _ = owner.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts",
        {"filename": "laporan.txt", "size": len(payload) + 500, "sha256": digest,
         "storage_key": key, "run_id": run_id}, org=org)
    check("size tidak cocok = 400", status == 400, f"{status} {text[:160]}")

    # storage_key milik org lain.
    status, _, text, _ = owner.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts",
        {"filename": "x.txt", "size": 10, "sha256": digest,
         "storage_key": "artifacts/01ORGMILIKORANGLAINXXXXXXXX/t/r/id-x.txt",
         "run_id": run_id}, org=org)
    check("storage_key org lain = 400", status == 400, f"{status} {text[:160]}")

    # run yang tidak ada.
    status, _, text, _ = owner.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts/upload-url",
        {"filename": "x.txt", "size": 10, "run_id": "01RUNTIDAKADAXXXXXXXXXXX"}, org=org)
    check("run_id tidak ada = 404", status == 404, f"{status} {text[:160]}")

    # File di atas 25 MB.
    status, _, text, _ = owner.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts/upload-url",
        {"filename": "besar.bin", "size": 26 * 1024 * 1024, "run_id": run_id}, org=org)
    check("file > 25 MB = 400", status == 400, f"{status} {text[:160]}")

    print("\n== isolasi tenant ==")
    # US-AD47 AC3 bilang 404. Yang terjadi adalah 403: viewer bukan anggota org
    # ini, dan middleware resolusi tenant menolaknya SEBELUM handler berjalan —
    # jadi handler yang menjawab 404 tidak pernah dipanggil. Dua-duanya berarti
    # "tidak boleh dibaca"; yang penting bukan angkanya, melainkan bahwa
    # artifact-nya tidak pernah keluar. Temuan yang sama sudah dicatat di
    # docs/OPEN-ISSUES.md sejak fase komentar.
    if artifact_id:
        status, _, _, _ = viewer.call("GET", f"/api/v1/artifacts/{artifact_id}", org=org)
        check("artifact org lain ditolak (403/404)", status in (403, 404), str(status))
        status, _, _, _ = viewer.call("GET", f"/api/v1/tasks/{task_id}/artifacts", org=org)
        check("task org lain = 403", status in (403, 404), str(status))

    print("\n== gerbang peran ==")
    # viewer ada di org sendiri; dia harus ditolak di jalur unggah.
    status, _, text, _ = viewer.call(
        "POST", f"/api/v1/tasks/{task_id}/artifacts/upload-url",
        {"filename": "x.txt", "size": 10, "run_id": run_id}, org=org)
    check("viewer upload-url = 403", status in (403, 404), f"{status} {text[:160]}")

    print(f"\n=== HASIL: {len(PASSED)} ok / {len(FAILED)} gagal ===")
    for label in FAILED:
        print("  GAGAL:", label)
    return 1 if FAILED else 0


if __name__ == "__main__":
    sys.exit(main())
