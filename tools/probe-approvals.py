"""Probe Fase 7 (6.2.14) lawan API nyata: approval gate.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:
  1. lima route-nya terdaftar (401 tanpa sesi = route ada, bukan 404);
  2. gerbang perannya: viewer tidak boleh membuat gate (US-AD33 AC3), member
     tidak boleh memutuskan (ARCHITECTURE 11.3 — PRD US-AD34 AC3 bilang boleh,
     dan itu ditolak dengan alasan di internal/board/approval.go);
  3. `preview_json` kembali byte-identik. Server yang mem-parse lalu
     men-serialisasi ulang bisa menampilkan diff yang berbeda dari yang
     diusulkan worker — jadi probe-nya membandingkan string, bukan field;
  4. approve/reject dua kali → 409, bukan 200 kedua;
  5. task benar-benar berpindah: approve → `ready`, reject → `blocked(policy)`.
     Keduanya dibaca dari API, bukan dari asumsi.

Jalankan setelah `docker compose up -d api`:
    python tools/probe-approvals.py
"""

import http.cookiejar
import json
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"
PASSED, FAILED = [], []
PREVIEW = '{"zzz":1,"aaa":2.50,"nested":{"b":1,"a":2}}'


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

    def call(self, method, path, body=None, org=None, raw=None):
        data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
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
    """Ambil token sesi dari cookie jar. `call()` mengembalikan teks, bukan
    headers, jadi cookienya dibaca dari jar yang sudah diisi opener."""
    for cookie in client.jar:
        if cookie.name == "agentdeck_session":
            return cookie.value
    return ""


def register(client, email, org_name="Probe Org"):
    status, body, _ = client.call("POST", "/api/v1/auth/register",
                                  {"email": email, "password": "password1",
                                   "name": "Probe", "org_name": org_name})
    if status != 201:
        raise SystemExit(f"register {email}: {status} {body}")
    return body["workspace_id"]


def login(client, email):
    status, _, _ = client.call("POST", "/api/v1/auth/login",
                               {"email": email, "password": "password1"})
    if status != 200:
        raise SystemExit(f"login {email}: {status}")
    client.token = parse_cookie(client)


def make_task_ready(client, org, project_id, board_id, title):
    """Task + agent + claim, supaya gate punya run untuk ditunjuk."""
    status, task, _ = client.call("POST", f"/api/v1/boards/{board_id}/tasks",
                                  {"title": title, "status": "backlog"}, org=org)
    if status != 201:
        raise SystemExit(f"create task: {status} {task}")
    status, _, text = client.call("POST", f"/api/v1/tasks/{task['id']}/move",
                                  {"from": "backlog", "to": "ready"}, org=org)
    if status not in (200, 201):
        raise SystemExit(f"move ready: {status} {text}")
    return task["id"]


def main():
    suffix = str(int(time.time()))

    print("== route terdaftar (401 tanpa sesi, bukan 404) ==")
    anon = Client()
    for method, path, body in [
        ("GET", "/api/v1/approvals", None),
        ("GET", "/api/v1/approvals/01ABC", None),
        ("POST", "/api/v1/approvals/01ABC/approve", None),
        ("POST", "/api/v1/approvals/01ABC/reject", {"reason": "x"}),
        ("POST", "/api/v1/tasks/01ABC/approvals", {"preview_json": {"a": 1}}),
    ]:
        status, _, _ = anon.call(method, path, body)
        check(f"{method} {path} menolak tanpa sesi", status == 401, f"status={status}")

    print("== siapkan org, anggota, board, agent, task ==")
    owner = Client()
    org = register(owner, f"probe.ap.owner.{suffix}@example.com", "Probe Approvals")

    for role, name in [("admin", "admin"), ("member", "member"), ("viewer", "viewer")]:
        email = f"probe.ap.{name}.{suffix}@example.com"
        register(Client(), email, f"Probe {name}")
        status, _, text = owner.call("POST", f"/api/v1/orgs/{org}/members",
                                     {"email": email, "role": role})
        if status not in (200, 201):
            raise SystemExit(f"invite {role}: {status} {text}")

    admin, member, viewer = Client(), Client(), Client()
    login(admin, f"probe.ap.admin.{suffix}@example.com")
    login(member, f"probe.ap.member.{suffix}@example.com")
    login(viewer, f"probe.ap.viewer.{suffix}@example.com")

    status, project, _ = owner.call("POST", "/api/v1/projects",
                                    {"name": "Probe", "slug": f"probe-ap-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"project: {status} {project}")
    status, board, _ = owner.call("POST", f"/api/v1/projects/{project['id']}/boards",
                                  {"name": "Probe Board", "slug": f"probe-apb-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"board: {status} {board}")
    board_id = board["id"]

    status, agent, _ = owner.call("POST", f"/api/v1/projects/{project['id']}/agents",
                                  {"name": f"runner-{suffix}", "provider": "openai_compatible",
                                   "model": "gpt-4o"}, org=org)
    if status != 201:
        raise SystemExit(f"agent: {status} {agent}")

    print("== US-AD33: buat gate ==")
    task_id = make_task_ready(owner, org, project["id"], board_id, "Dangerous work")
    status, _, text = owner.call("POST", f"/api/v1/tasks/{task_id}/assign",
                                 {"agent_id": agent["id"]}, org=org)
    status, _, text = owner.call("POST", f"/api/v1/tasks/{task_id}/claim", org=org)
    check("claim task → 200/201 (run dimulai)", status in (200, 201), f"status={status} {text}")

    # Viewer tidak boleh membuat gate (US-AD33 AC3).
    status, _, _ = viewer.call("POST", f"/api/v1/tasks/{task_id}/approvals",
                               {"preview_json": json.loads(PREVIEW)}, org=org)
    check("viewer membuat gate → 403 (US-AD33 AC3)", status == 403, f"status={status}")

    # Tanpa preview → 400 (US-AD33 AC2).
    for label, payload in [("tanpa preview", {"reason": "x"}),
                           ("preview {}", {"preview_json": {}}),
                           ("preview null", {"preview_json": None})]:
        status, _, _ = member.call("POST", f"/api/v1/tasks/{task_id}/approvals", payload, org=org)
        check(f"gate {label} → 400", status == 400, f"status={status}")

    # Dengan preview: dikirim sebagai teks mentah supaya urutan kunci dan format
    # angka bertahan sampai ke respons.
    status, gate, text = member.call(
        "POST", f"/api/v1/tasks/{task_id}/approvals", org=org,
        raw=('{"preview_json":' + PREVIEW + ',"reason":"drop a dev table"}').encode())
    check("member membuat gate → 201", status == 201, f"status={status} {text}")
    if status != 201:
        raise SystemExit("gate tidak dibuat; sisa probe tidak bisa jalan")
    gate_id = gate["id"]
    check("gate baru: decision=pending", gate.get("decision") == "pending", gate)
    check("gate baru: gate_mode=require (US-AD33 AC1)", gate.get("gate_mode") == "require", gate)
    check("gate baru: requested_by terisi", bool(gate.get("requested_by")), gate)

    print("== preview_json utuh ==")
    # Isinya harus sama persis dengan yang dikirim. Urutan kunci TIDAK diperiksa:
    # kolomnya JSONB, dan Postgres menormalkan urutan kunci + spasi (JSONB adalah
    # tipe biner, bukan teks). Yang penting adalah tidak ada field yang hilang,
    # tidak ada nilai yang berubah bentuk, dan angka tidak dibulatkan.
    check("preview sama persis isinya", gate.get("preview_json") == json.loads(PREVIEW),
          gate.get("preview_json"))
    nested = (gate.get("preview_json") or {}).get("nested")
    check("objek bersarang utuh", nested == {"b": 1, "a": 2}, nested)
    check("angka pecahan tidak dibulatkan",
          (gate.get("preview_json") or {}).get("aaa") == 2.50,
          (gate.get("preview_json") or {}).get("aaa"))
    # Dibaca ulang lewat GET: yang tersimpan sama dengan yang dikembalikan.
    status, fetched, _ = viewer.call("GET", f"/api/v1/approvals/{gate_id}", org=org)
    check("GET approval → 200", status == 200, f"status={status}")
    check("preview sama saat dibaca ulang",
          (fetched or {}).get("preview_json") == json.loads(PREVIEW),
          (fetched or {}).get("preview_json"))

    print("== task diparkir ==")
    status, tasks, _ = owner.call("GET", f"/api/v1/boards/{board_id}/tasks", org=org)
    parked = next((t for t in (tasks or []) if t["id"] == task_id), None)
    check("task → awaiting_approval", parked and parked.get("status") == "awaiting_approval",
          parked and parked.get("status"))

    print("== inbox ==")
    status, inbox, _ = viewer.call("GET", "/api/v1/approvals", org=org)
    check("viewer membaca inbox → 200 (11.3)", status == 200, f"status={status}")
    ids = [a["id"] for a in (inbox or [])]
    check("gate muncul di inbox", gate_id in ids, ids)

    print("== gerbang keputusan (ARCHITECTURE 11.3) ==")
    status, _, _ = member.call("POST", f"/api/v1/approvals/{gate_id}/approve", org=org)
    check("member menyetujui → 403", status == 403, f"status={status}")
    status, _, _ = viewer.call("POST", f"/api/v1/approvals/{gate_id}/approve", org=org)
    check("viewer menyetujui → 403", status == 403, f"status={status}")
    status, _, _ = admin.call("POST", f"/api/v1/approvals/{gate_id}/reject",
                              {"reason": "   "}, org=org)
    check("admin menolak tanpa alasan → 400 (US-AD35 AC2)", status == 400, f"status={status}")

    print("== approve ==")
    status, decided, text = admin.call("POST", f"/api/v1/approvals/{gate_id}/approve", org=org)
    check("admin menyetujui → 200", status == 200, f"status={status} {text}")
    check("decision=approved", decided and decided.get("decision") == "approved", decided)
    check("decided_by terisi", decided and decided.get("decided_by"), decided)
    check("decided_at terisi", decided and decided.get("decided_at"), decided)

    status, _, _ = admin.call("POST", f"/api/v1/approvals/{gate_id}/approve", org=org)
    check("approve kedua → 409 (US-AD34 AC2)", status == 409, f"status={status}")

    status, tasks, _ = owner.call("GET", f"/api/v1/boards/{board_id}/tasks", org=org)
    after = next((t for t in (tasks or []) if t["id"] == task_id), None)
    check("task → ready setelah approve (5.4)", after and after.get("status") == "ready",
          after and after.get("status"))

    print("== reject ==")
    # Gate kedua di task yang sama tidak mungkin tanpa run hidup: gate pertama
    # sudah menutup run-nya, dan `approvals.run_id` NOT NULL. Yang benar adalah
    # task-nya diklaim ulang dulu — persis alur produksi.
    status, _, text = member.call("POST", f"/api/v1/tasks/{task_id}/approvals", org=org,
                                  raw=('{"preview_json":' + PREVIEW + '}').encode())
    check("gate kedua tanpa run hidup → 400", status == 400, f"status={status} {text}")

    status, _, text = owner.call("POST", f"/api/v1/tasks/{task_id}/claim", org=org)
    check("claim ulang → 200/201", status in (200, 201), f"status={status} {text}")
    status, gate2, text = member.call(
        "POST", f"/api/v1/tasks/{task_id}/approvals", org=org,
        raw=('{"preview_json":' + PREVIEW + '}').encode())
    check("gate kedua dibuat setelah claim ulang → 201", status == 201, f"status={status} {text}")
    if status == 201:
        status, decided, text = admin.call("POST", f"/api/v1/approvals/{gate2['id']}/reject",
                                           {"reason": "tabel dev masih dipakai"}, org=org)
        check("admin menolak → 200", status == 200, f"status={status} {text}")
        check("decision=rejected", decided and decided.get("decision") == "rejected", decided)
        check("reason tersimpan", decided and decided.get("reason") == "tabel dev masih dipakai", decided)

        status, tasks, _ = owner.call("GET", f"/api/v1/boards/{board_id}/tasks", org=org)
        blocked = next((t for t in (tasks or []) if t["id"] == task_id), None)
        check("task → blocked setelah reject (5.4)", blocked and blocked.get("status") == "blocked",
              blocked and blocked.get("status"))
        check("block_kind=policy, bukan needs_input (5.4)",
              blocked and blocked.get("block_kind") == "policy",
              blocked and blocked.get("block_kind"))

        status, _, _ = admin.call("POST", f"/api/v1/approvals/{gate2['id']}/reject",
                                  {"reason": "lagi"}, org=org)
        check("reject kedua → 409 (US-AD35 AC3)", status == 409, f"status={status}")

    print("== tenant lain tidak melihat apa-apa ==")
    outsider = Client()
    other_org = register(outsider, f"probe.ap.out.{suffix}@example.com", "Probe Out")
    status, _, _ = outsider.call("GET", f"/api/v1/approvals/{gate_id}", org=other_org)
    check("gate lintas-tenant dibaca → 404", status == 404, f"status={status}")
    status, _, _ = outsider.call("POST", f"/api/v1/approvals/{gate_id}/approve", org=other_org)
    check("gate lintas-tenant disetujui → 404", status == 404, f"status={status}")

    print()
    print(f"HASIL: {len(PASSED)} hijau, {len(FAILED)} gagal")
    if FAILED:
        for name in FAILED:
            print(f"  GAGAL: {name}")
        sys.exit(1)


if __name__ == "__main__":
    main()
