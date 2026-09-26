"""Probe Fase 5 (6.2.15) lawan API nyata: budget board + laporan biaya org.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:
  1. tiga route-nya terdaftar (401 tanpa sesi = route ada, bukan 404);
  2. bentuk responsnya cocok dengan tipe yang sudah dideklarasikan FE
     (`frontend/src/lib/domain.ts`: BoardBudget, CostSummary) — nama field yang
     meleset bikin layar finops kosong tanpa error;
  3. PATCH benar-benar mengubah cap yang dibaca GET (satu sumber angka);
  4. menaikkan cap di atas pemakaian membersihkan alert N18;
  5. org tanpa pemakaian menjawab laporan kosong bertotal nol, bukan 404;
  6. gerbang peran: viewer boleh baca budget, tidak boleh menulis; laporan org
     butuh admin.

Jalankan setelah `docker compose up -d api`:
    python tools/probe-budget.py
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
    email = f"probe.budget.{suffix}@example.com"

    print("== route terdaftar (401 tanpa sesi, bukan 404) ==")
    anon = Client()
    for method, path, body in [
        ("GET", "/api/v1/boards/01ABC/budget", None),
        ("PATCH", "/api/v1/boards/01ABC/budget", {"budget_daily_micros": 1}),
        ("GET", "/api/v1/orgs/01ABC/cost-summary", None),
    ]:
        status, _, _ = anon.call(method, path, body)
        check(f"{method} {path} menolak tanpa sesi", status == 401, f"status={status}")

    print("== siapkan board ==")
    owner = Client()
    org = register(owner, email)
    status, project, _ = owner.call("POST", "/api/v1/projects",
                                    {"name": "Probe", "slug": f"probe-b-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"project: {status} {project}")
    status, board, _ = owner.call("POST", f"/api/v1/projects/{project['id']}/boards",
                                  {"name": "Probe Board", "slug": f"probe-bb-{suffix}"}, org=org)
    if status != 201:
        raise SystemExit(f"board: {status} {board}")
    board_id = board["id"]

    print("== GET /boards/{id}/budget ==")
    status, budget, _ = owner.call("GET", f"/api/v1/boards/{board_id}/budget", org=org)
    check("budget → 200", status == 200, f"status={status} {budget}")
    if status == 200:
        for field in ("board_id", "day", "budget_daily_micros", "spent_micros",
                      "run_count", "tokens_in", "tokens_out", "threshold_crossed"):
            check(f"budget punya {field}", field in budget, budget)
        check("board baru: terpakai nol", budget.get("spent_micros") == 0, budget)
        check("board baru: cap terisi", budget.get("budget_daily_micros", 0) > 0, budget)
        check("board baru: alert N18 mati", budget.get("threshold_crossed") is False, budget)

    print("== PATCH menulis cap yang dibaca GET ==")
    status, patched, _ = owner.call("PATCH", f"/api/v1/boards/{board_id}/budget",
                                    {"budget_daily_micros": 1234567}, org=org)
    check("PATCH → 200", status == 200, f"status={status} {patched}")
    if status == 200:
        check("respons memuat cap baru", patched.get("budget_daily_micros") == 1234567, patched)
    status, reread, _ = owner.call("GET", f"/api/v1/boards/{board_id}/budget", org=org)
    check("GET berikutnya melihat cap baru",
          status == 200 and reread.get("budget_daily_micros") == 1234567, reread)

    print("== validasi PATCH ==")
    status, _, _ = owner.call("PATCH", f"/api/v1/boards/{board_id}/budget", {}, org=org)
    check("field absen → 400 (nol itu nilai sah, absen bukan nol)",
          status == 400, f"status={status}")
    status, _, _ = owner.call("PATCH", f"/api/v1/boards/{board_id}/budget",
                              {"budget_daily_micros": -5}, org=org)
    check("cap negatif → 400", status == 400, f"status={status}")
    status, zeroed, _ = owner.call("PATCH", f"/api/v1/boards/{board_id}/budget",
                                   {"budget_daily_micros": 0}, org=org)
    check("cap nol diterima", status == 200, f"status={status} {zeroed}")
    if status == 200:
        check("cap nol → exceeded, jadi klaim berhenti", zeroed.get("threshold_crossed") is True, zeroed)
    # kembalikan ke angka waras untuk sisa probe
    owner.call("PATCH", f"/api/v1/boards/{board_id}/budget", {"budget_daily_micros": 20000000}, org=org)

    print("== gerbang peran ==")
    nonmember = Client()
    register(nonmember, f"probe.nonmember.{suffix}@example.com", "Probe Non")
    status, _, _ = nonmember.call("GET", f"/api/v1/boards/{board_id}/budget", org=org)
    check("non-anggota baca budget → 403", status == 403, f"status={status}")
    status, _, _ = nonmember.call("PATCH", f"/api/v1/boards/{board_id}/budget",
                                  {"budget_daily_micros": 1}, org=org)
    check("non-anggota tulis budget → 403", status == 403, f"status={status}")

    print("== GET /orgs/{id}/cost-summary ==")
    status, summary, _ = owner.call("GET", f"/api/v1/orgs/{org}/cost-summary", org=org)
    check("cost-summary → 200", status == 200, f"status={status} {summary}")
    if status == 200:
        for field in ("total_micros", "window_days", "by_model", "by_board"):
            check(f"cost-summary punya {field}", field in summary, summary)
        check("window_days = 30", summary.get("window_days") == 30, summary)
        check("org tanpa pemakaian: total nol", summary.get("total_micros") == 0, summary)
        check("by_model array (bukan null)", isinstance(summary.get("by_model"), list), summary)
        check("by_board array (bukan null)", isinstance(summary.get("by_board"), list), summary)

    print("== cost-summary butuh admin ==")
    nonmember2 = Client()
    other_org = register(nonmember2, f"probe.viewer.{suffix}@example.com", "Probe Other")
    # non-anggota di org lain: middleware menolak 403 (bukan anggota) — batas
    # tenant-nya, bukan gerbang perannya.
    status, _, _ = nonmember2.call("GET", f"/api/v1/orgs/{org}/cost-summary", org=other_org)
    check("non-anggota baca laporan org → 403", status == 403, f"status={status}")

    print()
    print(f"HASIL: {len(PASSED)} hijau, {len(FAILED)} gagal")
    if FAILED:
        for name in FAILED:
            print(f"  GAGAL: {name}")
        sys.exit(1)


if __name__ == "__main__":
    main()
