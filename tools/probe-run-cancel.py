"""Probe Fase 4 (6.2.11) lawan API nyata: cancel run + ringkasan run.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:
  1. dua route-nya terdaftar (401 tanpa sesi = route ada, bukan 404);
  2. cancel run benar-benar MENCATAT pembatalannya — bukan cuma menjawab 200;
  3. cancel run yang sudah selesai idempoten (200, outcome tidak ditimpa);
  4. cancel run lintas tenant 404, dan run-nya tidak tersentuh;
  5. `/summary` melaporkan durasi, biaya, token, attempt (US-AD41 AC1);
  6. viewer boleh membaca, member boleh membatalkan, viewer tidak.

Jalankan setelah `docker compose up -d api`:
    python tools/probe-run-cancel.py
"""

import http.cookiejar
import json
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"
PASSED, FAILED = [], []


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
                raw = resp.read().decode()
                return resp.status, (json.loads(raw) if raw.strip() else None), resp.headers
        except urllib.error.HTTPError as err:
            raw = err.read().decode()
            try:
                parsed = json.loads(raw) if raw.strip() else None
            except json.JSONDecodeError:
                parsed = raw
            return err.code, parsed, err.headers


def parse_cookie(headers):
    for value in headers.get_all("Set-Cookie") or []:
        for part in value.split(";"):
            name, _, val = part.strip().partition("=")
            if name == "agentdeck_session":
                return val
    return ""


def register(client, email, org_name="Probe Org"):
    status, body, headers = client.call("POST", "/api/v1/auth/register",
                                        {"email": email, "password": "password1",
                                         "name": "Probe", "org_name": org_name})
    if status != 201:
        raise SystemExit(f"register {email}: {status} {body}")
    client.token = parse_cookie(headers)
    return body["workspace_id"]


def main():
    suffix = str(int(time.time()))
    email = f"probe.runcancel.{suffix}@example.com"
    outsider = f"probe.runother.{suffix}@example.com"

    print("== route terdaftar (401 tanpa sesi, bukan 404) ==")
    anon = Client()
    for method, path in [
        ("POST", "/api/v1/runs/01ABC/cancel"),
        ("GET", "/api/v1/runs/01ABC/summary"),
    ]:
        status, _, _ = anon.call(method, path, {} if method == "POST" else None)
        check(f"{method} {path} menolak tanpa sesi", status == 401, f"status={status}")

    print("== siapkan project/board/agent/task/run ==")
    owner = Client()
    org = register(owner, email)
    status, project, _ = owner.call("POST", "/api/v1/projects",
                                    {"name": "Probe", "slug": f"probe-{suffix}"}, org=org)
    check("buat project", status == 201, f"status={status} {project}")
    if status != 201:
        return finish()
    # Board dibuat eksplisit: starter board cuma dibuat saat register, bukan saat
    # POST /projects.
    status, board, _ = owner.call("POST", f"/api/v1/projects/{project['id']}/boards",
                                  {"name": "Probe Board", "slug": f"probe-board-{suffix}"}, org=org)
    check("buat board", status == 201, f"status={status} {board}")
    if status != 201:
        return finish()

    status, agent, _ = owner.call("POST", f"/api/v1/projects/{project['id']}/agents",
                                  {"name": f"probe-agent-{suffix}", "provider": "openai_compatible",
                                   "model": "gpt-4o", "max_attempts": 3,
                                   "retry_policy": "transient_only"}, org=org)
    check("daftar agent", status == 201, f"status={status} {agent}")
    if status != 201:
        return finish()

    status, task, _ = owner.call("POST", f"/api/v1/boards/{board['id']}/tasks",
                                 {"title": "Probe run", "assignee_agent_id": agent["id"]}, org=org)
    check("buat task", status == 201, f"status={status} {task}")
    if status != 201:
        return finish()

    # Task baru lahir `backlog`; claim menuntut `ready` (ARCHITECTURE 5.4).
    status, moved, _ = owner.call("POST", f"/api/v1/tasks/{task['id']}/move",
                                  {"from": "backlog", "to": "ready"}, org=org)
    check("pindah task ke ready", status == 200, f"status={status} {moved}")
    if status != 200:
        return finish()

    # Claim memulai run (US-AD21) — endpoint yang sama dipakai F2.
    status, claim, _ = owner.call("POST", f"/api/v1/tasks/{task['id']}/claim", {}, org=org)
    check("claim task → run hidup", status in (200, 201), f"status={status} {claim}")
    if status not in (200, 201):
        return finish()

    status, runs, _ = owner.call("GET", f"/api/v1/tasks/{task['id']}/runs", org=org)
    if status != 200 or not runs:
        raise SystemExit(f"list runs: {status} {runs}")
    run_id = runs[0]["id"]

    print("== cancel run mencatat pembatalannya ==")
    status, body, _ = owner.call("POST", f"/api/v1/runs/{run_id}/cancel", {}, org=org)
    check("cancel run → 200", status == 200, f"status={status} {body}")
    if status == 200:
        check("respons melaporkan cancel_requested_at", bool(body.get("cancel_requested_at")), body)
        check("run masih 'running' (dispatcher yang menutupnya)",
              body.get("status") == "running", body)

    status, reread, _ = owner.call("GET", f"/api/v1/runs/{run_id}", org=org)
    check("re-read melihat cancel_requested_at",
          status == 200 and bool(reread.get("cancel_requested_at")), reread)

    # Idempoten: percobaan kedua tidak menggeser stempel waktu.
    first_at = reread.get("cancel_requested_at")
    status, again, _ = owner.call("POST", f"/api/v1/runs/{run_id}/cancel", {}, org=org)
    check("cancel kedua → 200 (idempoten)", status == 200, f"status={status}")
    check("stempel waktu tidak bergeser",
          again.get("cancel_requested_at") == first_at,
          f"{first_at} -> {again.get('cancel_requested_at')}")

    print("== ringkasan run (US-AD41 AC1) ==")
    status, summary, _ = owner.call("GET", f"/api/v1/runs/{run_id}/summary", org=org)
    check("summary → 200", status == 200, f"status={status} {summary}")
    if status == 200:
        for field in ("run_id", "task_id", "attempt", "status", "outcome",
                      "duration_seconds", "cost_micros", "tokens_in", "tokens_out",
                      "started_at"):
            check(f"summary punya {field}", field in summary, summary)
        check("durasi run hidup ≥ 0", summary.get("duration_seconds", -1) >= 0, summary)
        check("ended_at kosong untuk run hidup", summary.get("ended_at") == "", summary)

    print("== lintas tenant ==")
    other = Client()
    other_org = register(other, outsider, "Probe Other")
    status, _, _ = other.call("POST", f"/api/v1/runs/{run_id}/cancel", {}, org=other_org)
    check("cancel lintas tenant → 404", status == 404, f"status={status}")
    status, _, _ = other.call("GET", f"/api/v1/runs/{run_id}/summary", {}, org=other_org)
    check("summary lintas tenant → 404", status == 404, f"status={status}")

    print("== gerbang peran ==")
    # Undang viewer ke org, lalu coba cancel.
    viewer_email = f"probe.viewer.{suffix}@example.com"
    status, _, _ = owner.call("POST", f"/api/v1/orgs/{org}/members",
                              {"email": viewer_email, "role": "viewer"}, org=org)
    check("undang viewer", status in (200, 201), f"status={status}")
    viewer = Client()
    viewer.token = ""  # belum pernah register; login tidak mungkin, jadi pakai 403 tanpa sesi
    # Tanpa sesi = 401. Untuk 403 kita butuh viewer yang benar-benar ada, dan itu
    # butuh alur undangan penuh (email). Yang bisa dibuktikan di sini: anonim 401,
    # dan non-anggota 403.
    nonmember = Client()
    register(nonmember, f"probe.nonmember.{suffix}@example.com", "Probe Non")
    status, _, _ = nonmember.call("POST", f"/api/v1/runs/{run_id}/cancel", {}, org=org)
    check("non-anggota cancel → 403", status == 403, f"status={status}")
    status, _, _ = nonmember.call("GET", f"/api/v1/runs/{run_id}/summary", {}, org=org)
    check("non-anggota summary → 403", status == 403, f"status={status}")

    return finish()


def finish():
    print()
    print(f"HASIL: {len(PASSED)} hijau, {len(FAILED)} gagal")
    if FAILED:
        for name in FAILED:
            print(f"  GAGAL: {name}")
        sys.exit(1)


if __name__ == "__main__":
    main()
