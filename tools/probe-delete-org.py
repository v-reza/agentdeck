"""Probe Fase 6 (6.2.4) lawan API nyata: DELETE /orgs/{id}.

Yang cuma bisa dibuktikan lawan server yang benar-benar jalan:
  1. route-nya terdaftar (401 tanpa sesi = route ada, bukan 404);
  2. owner boleh (204), admin/member/viewer tidak (403);
  3. idempoten: request kedua 204, bukan 404. Ini yang gagal kalau route-nya
     didaftarkan lewat orgContextMiddleware — middleware itu me-resolve tenant
     lewat GetOrgByID yang menyaring deleted_at IS NULL;
  4. soft: workspace berhenti resolve untuk semua anggotanya dan hilang dari
     GET /orgs, tapi barisnya masih ada di DB (diperiksa lewat psql terpisah);
  5. non-anggota 403, id tak dikenal 404.

Jalankan setelah `docker compose up -d api`:
    python tools/probe-delete-org.py
"""

import http.cookiejar
import json
import subprocess
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


def org_exists_in_db(org_id):
    """Baca langsung dari Postgres: soft delete harus menyisakan barisnya."""
    sql = f"SELECT deleted_at IS NOT NULL FROM orgs WHERE id = '{org_id}';"
    r = subprocess.run(["docker", "exec", "agentdeck-db", "psql", "-U", "agentdeck",
                        "-d", "agentdeck", "-tAc", sql],
                       capture_output=True, text=True)
    return r.stdout.strip()


def main():
    suffix = str(int(time.time()))

    print("== route terdaftar (401 tanpa sesi, bukan 404) ==")
    anon = Client()
    status, _, _ = anon.call("DELETE", "/api/v1/orgs/01ABC")
    check("DELETE /orgs/{id} menolak tanpa sesi", status == 401, f"status={status}")

    print("== siapkan org + anggota ==")
    owner = Client()
    owner_personal = register(owner, f"probe.del.owner.{suffix}@example.com", "Probe Owner")
    status, org, _ = owner.call("POST", "/api/v1/orgs",
                                {"name": "Probe Del", "slug": f"probe-del-{suffix}"})
    if status != 201:
        raise SystemExit(f"create org: {status} {org}")
    org_id = org["id"]

    admin = Client()
    admin_personal = register(admin, f"probe.del.admin.{suffix}@example.com", "Probe Admin")
    status, _, _ = owner.call("POST", f"/api/v1/orgs/{org_id}/members",
                              {"email": f"probe.del.admin.{suffix}@example.com", "role": "admin"})
    if status not in (200, 201):
        raise SystemExit(f"invite admin: {status}")
    # admin menyelesaikan undangannya dengan melihat daftar org-nya sendiri
    admin.call("GET", "/api/v1/orgs")

    member = Client()
    register(member, f"probe.del.member.{suffix}@example.com", "Probe Member")
    owner.call("POST", f"/api/v1/orgs/{org_id}/members",
               {"email": f"probe.del.member.{suffix}@example.com", "role": "member"})
    member.call("GET", "/api/v1/orgs")

    outsider = Client()
    outsider_personal = register(outsider, f"probe.del.out.{suffix}@example.com", "Probe Out")

    print("== gerbang peran ==")
    for who, client in [("admin", admin), ("member", member)]:
        status, _, _ = client.call("DELETE", f"/api/v1/orgs/{org_id}", org=org_id)
        check(f"{who} menghapus org → 403", status == 403, f"status={status}")
    status, _, _ = outsider.call("DELETE", f"/api/v1/orgs/{org_id}", org=outsider_personal)
    check("non-anggota menghapus org → 403", status == 403, f"status={status}")

    # Org-nya harus masih hidup setelah semua penolakan.
    status, _, _ = owner.call("GET", f"/api/v1/orgs/{org_id}", org=org_id)
    check("org masih hidup setelah penolakan", status == 200, f"status={status}")

    print("== owner menutup ==")
    status, _, _ = owner.call("DELETE", f"/api/v1/orgs/{org_id}", org=org_id)
    check("owner menghapus org → 204", status == 204, f"status={status}")

    print("== idempoten ==")
    status, _, _ = owner.call("DELETE", f"/api/v1/orgs/{org_id}", org=org_id)
    check("hapus kedua → 204 (bukan 404)", status == 204, f"status={status}")

    print("== soft, bukan hard ==")
    marker = org_exists_in_db(org_id)
    check("baris org masih ada di DB", marker in ("t", "f"), f"psql={marker!r}")
    check("orgs.deleted_at terisi (soft, bukan hard)", marker == "t", f"psql={marker!r}")

    print("== efeknya terlihat oleh anggota lain ==")
    status, _, _ = member.call("GET", f"/api/v1/orgs/{org_id}", org=org_id)
    check("anggota lain tidak bisa resolve org tertutup", status in (403, 404), f"status={status}")

    status, orgs, _ = owner.call("GET", "/api/v1/orgs")
    check("GET /orgs → 200", status == 200, f"status={status}")
    if isinstance(orgs, list):
        check("org tertutup hilang dari switcher",
              all(o.get("id") != org_id for o in orgs), orgs)
    else:
        check("org tertutup hilang dari switcher", False, orgs)

    print("== id tak dikenal ==")
    status, _, _ = owner.call("DELETE", "/api/v1/orgs/01ZZZZZZZZZZZZZZZZZZZZZZZZZZ",
                              org=owner_personal)
    check("id tak dikenal → 404", status == 404, f"status={status}")

    print("== sesi akun tetap hidup ==")
    status, _, _ = owner.call("GET", "/api/v1/auth/me")
    check("menutup workspace tidak mencabut sesi akunnya", status == 200, f"status={status}")

    print()
    print(f"HASIL: {len(PASSED)} hijau, {len(FAILED)} gagal")
    if FAILED:
        for name in FAILED:
            print(f"  GAGAL: {name}")
        sys.exit(1)


if __name__ == "__main__":
    main()
